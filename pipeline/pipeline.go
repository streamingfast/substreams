package pipeline

import (
	"fmt"
	"math"
	"strings"

	"github.com/streamingfast/substreams/pipeline/execout"
	"github.com/streamingfast/substreams/tracing"

	"github.com/streamingfast/bstream"
	"github.com/streamingfast/dstore"
	"github.com/streamingfast/substreams"
	"github.com/streamingfast/substreams/block"
	"github.com/streamingfast/substreams/manifest"
	"github.com/streamingfast/substreams/orchestrator"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/store"
	"github.com/streamingfast/substreams/wasm"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	ttrace "go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

type Pipeline struct {
	vmType    string // wasm/rust-v1, native
	blockType string

	maxStoreSyncRangeSize uint64

	preBlockHooks  []substreams.BlockHook
	postBlockHooks []substreams.BlockHook
	postJobHooks   []substreams.PostJobHook

	wasmRuntime    *wasm.Runtime
	wasmExtensions []wasm.WASMExtensioner

	reqCtx *RequestContext

	graph        *manifest.ModuleGraph
	moduleHashes *manifest.ModuleHashes
	respFunc     func(resp *pbsubstreams.Response) error

	outputModuleMap      map[string]bool
	backprocessingStores []store.Store

	moduleExecutors []ModuleExecutor

	cachingEngine execout.CacheEngine

	baseStateStore dstore.Store

	moduleOutputs []*pbsubstreams.ModuleOutput
	forkHandler   *ForkHandler

	partialsWritten block.Ranges // when backprocessing, to report back to orchestrator

	subrequestSplitSize int

	storeMap *store.Map
	tracer   ttrace.Tracer
	// rootSpan represents the top-level span of the Pipeline object, initiated when `Init` is called
	rootSpan     ttrace.Span
	storeFactory *StoreFactory
	bounder      *StoreBoundary
}

func New(reqCtx *RequestContext, graph *manifest.ModuleGraph, blockType string, wasmExtensions []wasm.WASMExtensioner, subRequestSplitSize int, engine execout.CacheEngine, storeMap *store.Map, storeGenerator *StoreFactory, bounder *StoreBoundary, respFunc func(resp *pbsubstreams.Response) error, opts ...Option) *Pipeline {
	pipe := &Pipeline{
		reqCtx:        reqCtx,
		cachingEngine: engine,
		tracer:        otel.GetTracerProvider().Tracer("pipeline"),
		// WARN: we don't support < 0 StartBlock for now
		//requestedStartBlockNum: uint64(request.StartBlockNum),
		storeMap:              storeMap,
		storeFactory:          storeGenerator,
		graph:                 graph,
		outputModuleMap:       map[string]bool{},
		blockType:             blockType,
		wasmExtensions:        wasmExtensions,
		subrequestSplitSize:   subRequestSplitSize,
		maxStoreSyncRangeSize: math.MaxUint64,
		respFunc:              respFunc,
		bounder:               bounder,
		forkHandler:           NewForkHandle(),
	}

	for _, name := range reqCtx.Request().OutputModules {
		pipe.outputModuleMap[name] = true
	}

	for _, opt := range opts {
		opt(pipe)
	}

	return pipe
}

func (p *Pipeline) Init(workerPool *orchestrator.WorkerPool) (err error) {
	_, p.rootSpan = p.tracer.Start(p.reqCtx.Context, "pipeline_init")
	defer tracing.EndSpan(p.rootSpan, tracing.WithEndErr(err))

	p.reqCtx.logger.Info("initializing handler",
		zap.Uint64("requested_start_block", p.reqCtx.StartBlockNum()),
		zap.Uint64("requested_stop_block", p.reqCtx.StopBlockNum()),
		zap.String("requested_start_cursor", p.reqCtx.Request().StartCursor),
		zap.Bool("is_back_processing", p.reqCtx.isSubRequest),
		zap.Strings("outputs", p.reqCtx.Request().OutputModules),
	)

	if err := p.validateBinaries(); err != nil {
		return fmt.Errorf("binary validation failed: %w", err)
	}

	modules, storeModules, err := p.getModules()
	if err != nil {
		return fmt.Errorf("failed to retrieve modules: %w", err)
	}

	if err := p.validateAndHashModules(modules); err != nil {
		return fmt.Errorf("module failed validation: %w", err)
	}

	p.reqCtx.logger.Info("priming caching engine")
	if err := p.cachingEngine.Init(p.moduleHashes); err != nil {
		return fmt.Errorf("failed to prime caching engine: %w", err)
	}

	p.reqCtx.logger.Info("initializing and adding stores")
	if err := p.addStores(storeModules); err != nil {
		return fmt.Errorf("failed to add stores: %w", err)
	}

	p.reqCtx.logger.Info("stores loaded", zap.Object("stores", p.storeMap))

	if p.reqCtx.isSubRequest {
		if err := p.setupSubrequestStores(storeModules); err != nil {
			return fmt.Errorf("faile to setup backprocessings: %w", err)
		}
	} else {
		if err := p.runBackProcessAndSetupStores(workerPool, storeModules); err != nil {
			return fmt.Errorf("faile setup request: %w", err)
		}
	}

	if err = p.buildWASM(modules); err != nil {
		return fmt.Errorf("initiating module output caches: %w", err)
	}

	p.bounder.InitBoundary(p.reqCtx.StartBlockNum())
	p.reqCtx.logger.Info("initialized store boundary block",
		zap.Uint64("request_start_block", p.reqCtx.StartBlockNum()),
		zap.Uint64("next_boundary_block", p.bounder.Boundary()),
	)

	return nil
}

func (p *Pipeline) setupSubrequestStores(storeModules []*pbsubstreams.Module) error {
	outputStoreCount := len(p.reqCtx.Request().OutputModules)
	if outputStoreCount > 1 {
		// currently only support 1 lead stores
		return fmt.Errorf("invalid number of backprocess leaf store: %d", outputStoreCount)
	}
	outputStoreModule := storeModules[0]

	p.reqCtx.logger.Info("marking leaf store for partial processing", zap.String("module", outputStoreModule.Name))

	var partialStore store.Store
	var err error
	// if a subtrequest's StartBlock is equal to the module StartBlock, we will create a full store regardless
	isPartialStore := p.reqCtx.StartBlockNum() != outputStoreModule.InitialBlock
	//if isPartialStore {
	partialStore, err = p.storeFactory.NewPartialKV(
		p.moduleHashes.Get(outputStoreModule.Name),
		outputStoreModule,
		p.reqCtx.StartBlockNum(),
		p.reqCtx.logger,
	)
	//} else {
	//	partialStore, err = p.storeFactory.NewFullKV(
	//		p.moduleHashes.Get(outputStoreModule.Name),
	//		outputStoreModule,
	//		p.reqCtx.logger,
	//	)
	//}
	if err != nil {
		return fmt.Errorf("creating store (partial: %t): %w", isPartialStore, err)
	}

	// update the BaseStore to a partial store for a backprocessing output
	p.storeMap.Set(outputStoreModule.Name, partialStore)

	for _, store := range p.storeMap.All() {
		// we want to load the store's start block if
		// the module initial block is not the request start block.. why
		// I'm not sure yet
		if store.InitialBlock() == p.reqCtx.StartBlockNum() {
			continue
		}

		if err := store.Load(p.reqCtx, p.reqCtx.StartBlockNum()); err != nil {
			return fmt.Errorf("failed to initialize store: %w", err)
		}
	}
	p.backprocessingStores = append(p.backprocessingStores, partialStore)
	return nil
}

func (p *Pipeline) runBackProcessAndSetupStores(workerPool *orchestrator.WorkerPool, storeModules []*pbsubstreams.Module) error {
	// this is a long run process, it will run the whole back process logic
	backProcessedStores, err := p.backProcessStores(workerPool, storeModules)
	if err != nil {
		return fmt.Errorf("synchronizing stores: %w", err)
	}

	for modName, store := range backProcessedStores {
		p.storeMap.Set(modName, store)
	}

	p.backprocessingStores = nil

	if err := p.sendSnapshots(); err != nil {
		return fmt.Errorf("send initial snapshots: %w", err)
	}
	return nil
}

func (p *Pipeline) isOutputModule(name string) bool {
	_, found := p.outputModuleMap[name]
	return found
}

func (p *Pipeline) validateAndHashModules(modules []*pbsubstreams.Module) error {
	p.moduleHashes = manifest.NewModuleHashes()
	for _, module := range modules {
		isOutput := p.outputModuleMap[module.Name]
		if isOutput && p.reqCtx.StartBlockNum() < module.InitialBlock {
			return fmt.Errorf("start block %d smaller than request outputs for module %q with start block %d", p.reqCtx.StartBlockNum(), module.Name, module.InitialBlock)
		}
		p.moduleHashes.HashModule(p.reqCtx.Request().Modules, module, p.graph)
	}
	return nil
}

func (p *Pipeline) flushStores(blockNum uint64, span trace.Span) error {
	subrequestStopBlock := p.reqCtx.isSubRequest && (p.reqCtx.StopBlockNum() == blockNum)
	for p.bounder.PassedBoundary(blockNum) || subrequestStopBlock {
		span.AddEvent("store_save_boundary_reach")

		boundaryBlock := p.bounder.Boundary()
		if subrequestStopBlock {
			boundaryBlock = p.reqCtx.StopBlockNum()
		}

		if err := p.saveStoresSnapshots(boundaryBlock); err != nil {
			return fmt.Errorf("error saving stores snashotps: %w", err)
		}

		p.bounder.BumpBoundary()
		if isStopBlockReached(blockNum, p.reqCtx.StopBlockNum()) {
			break
		}
	}
	return nil
}

func (p *Pipeline) saveStoresSnapshots(boundaryBlock uint64) (err error) {
	for name, s := range p.storeMap.All() {
		// optimatinz because we know that in a subrequest we are only running throught the last store (output)
		// all parent stores should have come from moduleOutput cache
		if p.reqCtx.isSubRequest && !p.isOutputModule(name) {
			// skip saving snapshot for non-output stores in sub request
			continue
		}

		_, span := p.tracer.Start(p.reqCtx.Context, "save_store_snapshot", ttrace.WithAttributes(attribute.String("store", name)))
		defer tracing.EndSpan(span, tracing.WithEndErr(err))

		blockRange, err := s.Save(p.reqCtx, boundaryBlock)
		if err != nil {
			return fmt.Errorf("sacing store %q at boundary %d: %w", name, boundaryBlock, err)
		}

		if p.reqCtx.isSubRequest && p.isOutputModule(name) {
			p.partialsWritten = append(p.partialsWritten, blockRange)
			p.reqCtx.logger.Debug("adding partials written", zap.Object("range", blockRange), zap.Stringer("ranges", p.partialsWritten), zap.Uint64("boundary_block", boundaryBlock))

			if v, ok := s.(store.PartialStore); ok {
				span.AddEvent("store_roll_trigger")
				v.Roll(boundaryBlock)
			}
		}
	}
	return nil
}

func (p *Pipeline) runPostJobHooks(clock *pbsubstreams.Clock) {
	for _, hook := range p.postJobHooks {
		if err := hook(p.reqCtx, clock); err != nil {
			p.reqCtx.logger.Warn("post job hook failed", zap.Error(err))
		}
	}

}

func (p *Pipeline) runPreBlockHooks(clock *pbsubstreams.Clock, span trace.Span) error {
	for _, hook := range p.preBlockHooks {
		span.AddEvent("running_pre_block_hook", ttrace.WithAttributes(attribute.String("hook", fmt.Sprintf("%T", hook))))
		if err := hook(p.reqCtx, clock); err != nil {
			return fmt.Errorf("pre block hook: %w", err)
		}
	}
	return nil
}

func (p *Pipeline) runExecutor(executor ModuleExecutor, execOutput execout.ExecutionOutput) error {
	//FIXME(abourget): should we ever skip that work?
	// if executor.ModuleInitialBlock < block.Number {
	// 	continue ??
	// }

	executorName := executor.Name()
	p.reqCtx.logger.Debug("executing", zap.String("module_name", executorName))

	output, cached, err := execOutput.Get(executor.Name())
	if err != nil && err != execout.NotFound {
		return fmt.Errorf("error getting module %q output: %w", executor.Name(), err)
	}
	if cached {
		if err := executor.applyCachedOutput(output); err != nil {
			return fmt.Errorf("failed to apply cache output for module %q: %w", executorName, err)
		}
		return nil
	}

	outputData, moduleOutputData, err := executor.run(p.reqCtx, execOutput)
	if err != nil {
		logs, truncated := executor.moduleLogs()
		if len(logs) != 0 || moduleOutputData != nil {
			p.moduleOutputs = append(p.moduleOutputs, &pbsubstreams.ModuleOutput{
				Name:          executorName,
				Data:          moduleOutputData,
				Logs:          logs,
				LogsTruncated: truncated,
			})
		}
		return fmt.Errorf("running module: %w", err)
	}

	if err := execOutput.Set(executor.Name(), outputData); err != nil {
		return fmt.Errorf("failed to set output %w", err)
	}

	if p.isOutputModule(executorName) {
		logs, truncated := executor.moduleLogs()
		if len(logs) != 0 || moduleOutputData != nil {
			moduleOutput := &pbsubstreams.ModuleOutput{
				Name:          executorName,
				Data:          moduleOutputData,
				Logs:          logs,
				LogsTruncated: truncated,
			}
			p.moduleOutputs = append(p.moduleOutputs, moduleOutput)
			p.forkHandler.addReversibleOutput(moduleOutput, execOutput.Clock().Number)
		}
	}

	executor.Reset()
	return nil
}

func shouldReturn(blockNum, requestedStartBlockNum uint64) bool {
	return blockNum >= requestedStartBlockNum
}

func shouldReturnProgress(isSubRequest bool) bool {
	return isSubRequest
}

func shouldReturnDataOutputs(blockNum, requestedStartBlockNum uint64, isSubRequest bool) bool {
	return shouldReturn(blockNum, requestedStartBlockNum) && !isSubRequest
}

func isStopBlockReached(currentBlock uint64, stopBlock uint64) bool {
	return stopBlock != 0 && currentBlock >= stopBlock
}

func (p *Pipeline) returnModuleProgressOutputs(clock *pbsubstreams.Clock) error {
	var progress []*pbsubstreams.ModuleProgress
	for _, store := range p.backprocessingStores {
		progress = append(progress, &pbsubstreams.ModuleProgress{
			Name: store.Name(),
			Type: &pbsubstreams.ModuleProgress_ProcessedRanges{
				// TODO charles: add p.hostname
				ProcessedRanges: &pbsubstreams.ModuleProgress_ProcessedRange{
					ProcessedRanges: []*pbsubstreams.BlockRange{
						{
							StartBlock: store.InitialBlock(),
							EndBlock:   clock.Number,
						},
					},
				},
			},
		})
	}

	if err := p.respFunc(substreams.NewModulesProgressResponse(progress)); err != nil {
		return fmt.Errorf("calling return func: %w", err)
	}
	return nil
}

func (p *Pipeline) returnFailureProgress(err error, failedExecutor ModuleExecutor) error {
	var out []*pbsubstreams.ModuleProgress

	failedProgress := &pbsubstreams.ModuleProgress{
		Name: failedExecutor.Name(),
		Type: &pbsubstreams.ModuleProgress_Failed_{
			Failed: &pbsubstreams.ModuleProgress_Failed{
				Reason: err.Error(),
				Logs:   failedExecutor.currentExecutionStack(),
			},
		},
	}
	out = append(out, failedProgress)

	for _, moduleOutput := range p.moduleOutputs {
		if moduleOutput.Name == failedExecutor.Name() {
			failedProgress.GetFailed().LogsTruncated = moduleOutput.GetLogsTruncated()
		}

		if len(moduleOutput.Logs) != 0 {
			out = append(out, &pbsubstreams.ModuleProgress{
				Name: moduleOutput.Name,
				Type: &pbsubstreams.ModuleProgress_Failed_{
					Failed: &pbsubstreams.ModuleProgress_Failed{
						Reason:        fmt.Sprintf("Failed to execute %s: %s", failedExecutor.Name(), err.Error()),
						Logs:          failedExecutor.currentExecutionStack(),
						LogsTruncated: moduleOutput.LogsTruncated,
					},
				},
			})
		}
	}

	p.reqCtx.logger.Debug("return failed progress", zap.Int("progress_len", len(out)))
	return p.respFunc(substreams.NewModulesProgressResponse(out))
}

func (p *Pipeline) validateBinaries() error {
	for _, binary := range p.reqCtx.Request().Modules.Binaries {
		if binary.Type != "wasm/rust-v1" {
			return fmt.Errorf("unsupported binary type: %q, supported: %q", binary.Type, p.vmType)
		}
		p.vmType = binary.Type
	}
	return nil
}

func (p *Pipeline) getModules() (modules []*pbsubstreams.Module, storeModules []*pbsubstreams.Module, err error) {
	if modules, err = p.graph.ModulesDownTo(p.reqCtx.Request().OutputModules); err != nil {
		return nil, nil, fmt.Errorf("building execution moduleGraph: %w", err)
	}
	if storeModules, err = p.graph.StoresDownTo(p.reqCtx.Request().OutputModules); err != nil {
		return nil, nil, err
	}
	return modules, storeModules, nil
}

func (p *Pipeline) buildWASM(modules []*pbsubstreams.Module) error {
	p.wasmRuntime = wasm.NewRuntime(p.wasmExtensions)
	tracer := otel.GetTracerProvider().Tracer("executor")

	for _, module := range modules {
		inputs, err := renderWasmInputs(module, p.storeMap)
		if err != nil {
			return fmt.Errorf("module %q: get wasm inputs: %w", module.Name, err)
		}

		modName := module.Name // to ensure it's enclosed
		entrypoint := module.BinaryEntrypoint
		code := p.reqCtx.Request().Modules.Binaries[module.BinaryIndex]
		wasmModule, err := p.wasmRuntime.NewModule(p.reqCtx, p.reqCtx.Request(), code.Content, module.Name, entrypoint)
		if err != nil {
			return fmt.Errorf("new wasm module: %w", err)
		}

		switch kind := module.Kind.(type) {
		case *pbsubstreams.Module_KindMap_:
			outType := strings.TrimPrefix(module.Output.Type, "proto:")

			baseExecutor := BaseExecutor{
				moduleName:    module.Name,
				wasmModule:    wasmModule,
				entrypoint:    entrypoint,
				wasmArguments: inputs,
				tracer:        tracer,
			}

			executor := &MapperModuleExecutor{
				BaseExecutor: baseExecutor,
				outputType:   outType,
			}

			p.moduleExecutors = append(p.moduleExecutors, executor)
			continue
		case *pbsubstreams.Module_KindStore_:
			updatePolicy := kind.KindStore.UpdatePolicy
			valueType := kind.KindStore.ValueType

			outputStore, found := p.storeMap.Get(modName)
			if !found {
				return fmt.Errorf(" store %q not found", modName)
			}
			inputs = append(inputs, wasm.NewStoreWriterOutput(modName, outputStore, updatePolicy, valueType))

			baseExecutor := BaseExecutor{
				moduleName:    modName,
				wasmModule:    wasmModule,
				entrypoint:    entrypoint,
				wasmArguments: inputs,
				tracer:        tracer,
			}

			s := &StoreModuleExecutor{
				BaseExecutor: baseExecutor,
				outputStore:  outputStore,
			}

			p.moduleExecutors = append(p.moduleExecutors, s)
			continue
		default:
			return fmt.Errorf("invalid kind %q input module %q", module.Kind, module.Name)
		}
	}

	return nil
}

func (p *Pipeline) addStores(storeModules []*pbsubstreams.Module) error {
	for _, storeModule := range storeModules {
		store, err := p.storeFactory.NewFullKV(p.moduleHashes.Get(storeModule.Name), storeModule, p.reqCtx.logger)
		if err != nil {
			return fmt.Errorf("failed to load store: %w", err)
		}
		p.storeMap.Set(storeModule.Name, store)
	}
	return nil
}

func returnModuleDataOutputs(clock *pbsubstreams.Clock, step bstream.StepType, cursor *bstream.Cursor, moduleOutputs []*pbsubstreams.ModuleOutput, respFunc func(resp *pbsubstreams.Response) error) error {
	protoStep, _ := pbsubstreams.StepToProto(step, false)
	out := &pbsubstreams.BlockScopedData{
		Outputs: moduleOutputs,
		Clock:   clock,
		Step:    protoStep,
		Cursor:  cursor.ToOpaque(),
	}

	if err := respFunc(substreams.NewBlockScopedDataResponse(out)); err != nil {
		return fmt.Errorf("calling return func: %w", err)
	}

	return nil
}

func renderWasmInputs(module *pbsubstreams.Module, storeAccessor *store.Map) (out []wasm.Argument, err error) {
	for _, input := range module.Inputs {
		switch in := input.Input.(type) {
		case *pbsubstreams.Module_Input_Map_:
			out = append(out, wasm.NewMapInput(in.Map.ModuleName))
		case *pbsubstreams.Module_Input_Store_:
			inputName := input.GetStore().ModuleName
			if input.GetStore().Mode == pbsubstreams.Module_Input_Store_DELTAS {
				out = append(out, wasm.NewMapInput(inputName))
			} else {
				store, found := storeAccessor.Get(inputName)
				if !found {
					return nil, fmt.Errorf("store %q npt found", inputName)
				}
				out = append(out, wasm.NewStoreReaderInput(inputName, store))
			}
		case *pbsubstreams.Module_Input_Source_:
			out = append(out, wasm.NewBlockInput(in.Source.Type))
		default:
			return nil, fmt.Errorf("invalid input struct for module %q", module.Name)
		}
	}
	return out, nil
}

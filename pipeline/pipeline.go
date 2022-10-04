package pipeline

import (
	"context"
	"fmt"
	"io"
	"math"
	"runtime/debug"
	"strings"

	"github.com/streamingfast/bstream"
	"github.com/streamingfast/dstore"
	"github.com/streamingfast/logging"
	"github.com/streamingfast/substreams"
	"github.com/streamingfast/substreams/block"
	"github.com/streamingfast/substreams/manifest"
	"github.com/streamingfast/substreams/orchestrator"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/pipeline/outputs"
	"github.com/streamingfast/substreams/state"
	"github.com/streamingfast/substreams/wasm"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	ttrace "go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type Pipeline struct {
	vmType    string // wasm/rust-v1, native
	blockType string

	requestedStartBlockNum uint64 // rename to: requestStartBlock, SET UPON receipt of the request
	maxStoreSyncRangeSize  uint64
	isSubrequest           bool

	preBlockHooks  []substreams.BlockHook
	postBlockHooks []substreams.BlockHook
	postJobHooks   []substreams.PostJobHook

	wasmRuntime    *wasm.Runtime
	wasmExtensions []wasm.WASMExtensioner

	context      context.Context
	request      *pbsubstreams.Request
	graph        *manifest.ModuleGraph
	moduleHashes *manifest.ModuleHashes
	respFunc     func(resp *pbsubstreams.Response) error

	modules              []*pbsubstreams.Module
	outputModuleMap      map[string]bool
	storeModules         []*pbsubstreams.Module
	storeMap             map[string]*state.Store
	backprocessingStores []*state.Store

	moduleExecutors       []ModuleExecutor
	wasmOutputs           map[string][]byte
	nextStoreSaveBoundary uint64 // The next expected block at which we should flush stores (at save interval)

	baseStateStore    dstore.Store
	storeSaveInterval uint64

	clock         *pbsubstreams.Clock
	moduleOutputs []*pbsubstreams.ModuleOutput
	logs          []string
	forkHandler   *ForkHandler

	moduleOutputCache *outputs.ModulesOutputCache

	partialsWritten block.Ranges // when backprocessing, to report back to orchestrator

	currentBlockRef bstream.BlockRef

	outputCacheSaveBlockInterval uint64
	subrequestSplitSize          int

	logger *zap.Logger
	tracer ttrace.Tracer
}

var _zlog, _ = logging.PackageLogger("pipe", "github.com/streamingfast/substreams/pipeline")

func New(
	ctx context.Context,
	tracer ttrace.Tracer,
	request *pbsubstreams.Request,
	graph *manifest.ModuleGraph,
	blockType string,
	baseStateStore dstore.Store,
	outputCacheSaveBlockInterval uint64,
	wasmExtensions []wasm.WASMExtensioner,
	subrequestSplitSize int,
	respFunc func(resp *pbsubstreams.Response) error,
	opts ...Option) *Pipeline {

	pipe := &Pipeline{
		context: ctx,
		tracer:  tracer,
		request: request,
		// WARN: we don't support < 0 StartBlock for now
		requestedStartBlockNum:       uint64(request.StartBlockNum),
		storeMap:                     map[string]*state.Store{},
		graph:                        graph,
		baseStateStore:               baseStateStore,
		outputModuleMap:              map[string]bool{},
		blockType:                    blockType,
		wasmExtensions:               wasmExtensions,
		outputCacheSaveBlockInterval: outputCacheSaveBlockInterval,
		subrequestSplitSize:          subrequestSplitSize,
		maxStoreSyncRangeSize:        math.MaxUint64,
		respFunc:                     respFunc,
		forkHandler:                  NewForkHandle(),
		logger:                       _zlog,
	}

	for _, name := range request.OutputModules {
		pipe.outputModuleMap[name] = true
	}

	for _, opt := range opts {
		opt(pipe)
	}

	return pipe
}

func (p *Pipeline) isOutputModule(name string) bool {
	_, found := p.outputModuleMap[name]
	return found
}

func GetTraceID(ctx context.Context) (out ttrace.TraceID) {
	span := ttrace.SpanFromContext(ctx)
	return span.SpanContext().TraceID()
}

func (p *Pipeline) computeModuleHashes() {
	p.moduleHashes = manifest.NewModuleHashes()

	for _, module := range p.modules {
		if p.outputModuleMap[module.Name] {
			p.moduleHashes.HashModule(p.request.Modules, module, p.graph)
		}
	}
}

func (p *Pipeline) Init(workerPool *orchestrator.WorkerPool) (err error) {
	ctx := p.context
	traceID := GetTraceID(ctx)
	p.logger = p.logger.With(zap.Strings("outputs", p.request.OutputModules), zap.Bool("sub_request", p.isSubrequest), zap.String("trace_id", traceID.String()))

	ctx, span := p.tracer.Start(ctx, "pipeline_init")
	defer span.End()
	defer func() {
		if err != nil {
			span.SetStatus(codes.Error, err.Error())
		} else {
			span.SetStatus(codes.Ok, "")
		}
	}()

	p.logger.Info("initializing handler", zap.Uint64("requested_start_block", p.requestedStartBlockNum), zap.Uint64("requested_stop_block", p.request.StopBlockNum), zap.Bool("is_backprocessing", p.isSubrequest), zap.Strings("outputs", p.request.OutputModules))

	p.moduleOutputCache = outputs.NewModuleOutputCache(p.outputCacheSaveBlockInterval, p.logger)

	if err := p.build(); err != nil {
		span.SetStatus(codes.Error, err.Error())
		return fmt.Errorf("building pipeline: %w", err)
	}

	p.computeModuleHashes()

	for _, module := range p.modules {
		isOutput := p.outputModuleMap[module.Name]

		if isOutput && p.requestedStartBlockNum < module.InitialBlock {
			err := fmt.Errorf("invalid request: start block %d smaller that request outputs for module: %q start block %d", p.requestedStartBlockNum, module.Name, module.InitialBlock)
			return err
		}

		hash := p.moduleHashes.Get(module.Name)
		_, err := p.moduleOutputCache.RegisterModule(module, hash, p.baseStateStore)
		if err != nil {
			return fmt.Errorf("registering output cache for module %q: %w", module.Name, err)
		}
	}

	p.logger.Info("initializing and loading stores")
	initialStoreMap, err := p.buildStoreMap()
	p.logger.Info("stores load", zap.Int("number_of_stores", len(initialStoreMap)))
	if err != nil {
		return fmt.Errorf("building store map: %w", err)
	}

	// Fetch the stores
	if p.isSubrequest { // this is actually never gone in because of the wrong `if` statement in service.go
		// truncateLeaf()
		totalOutputModules := len(p.request.OutputModules)
		outputName := p.request.OutputModules[0]
		backProcessingStore := initialStoreMap[outputName]
		lastStoreName := p.storeModules[len(p.storeModules)-1].Name
		isLastStore := lastStoreName == backProcessingStore.Name

		if totalOutputModules != 1 || backProcessingStore == nil || !isLastStore {
			// totalOutputModels is a temporary restrictions, for when the orchestrator
			// will be able to run two leaf stores from the same job
			p.logger.Warn("conditions for leaf store not met",
				zap.String("module", outputName),
				zap.Bool("is_last_store", isLastStore),
				zap.Int("output_module_count", totalOutputModules))
			err := fmt.Errorf("invalid conditions to backprocess leaf store %q", outputName)
			return err
		}

		p.logger.Info("marking leaf store for partial processing", zap.String("module", outputName))

		backProcessingStore.Roll(p.requestedStartBlockNum)

		if err = loadCompleteStores(ctx, initialStoreMap, p.requestedStartBlockNum); err != nil {
			return fmt.Errorf("loading stores: %w", err)
		}

		p.storeMap = initialStoreMap
		p.backprocessingStores = append(p.backprocessingStores, backProcessingStore)
	} else {
		backProcessedStores, err := p.backProcessStores(ctx, workerPool, initialStoreMap)
		if err != nil {
			return fmt.Errorf("synchronizing stores: %w", err)
		}

		for modName, store := range backProcessedStores {
			initialStoreMap[modName] = store
		}

		p.storeMap = initialStoreMap
		p.backprocessingStores = nil

		if len(p.request.InitialStoreSnapshotForModules) != 0 {
			p.logger.Info("sending snapshot", zap.Strings("modules", p.request.InitialStoreSnapshotForModules))
			if err := p.sendSnapshots(p.request.InitialStoreSnapshotForModules); err != nil {
				return fmt.Errorf("send initial snapshots: %w", err)
			}
		}
	}

	p.initStoreSaveBoundary()

	err = p.buildWASM(ctx, p.request, p.modules)
	if err != nil {
		return fmt.Errorf("initiating module output caches: %w", err)
	}

	for _, cache := range p.moduleOutputCache.OutputCaches {
		atBlock := outputs.ComputeStartBlock(p.requestedStartBlockNum, p.outputCacheSaveBlockInterval)
		if _, err := cache.LoadAtBlock(ctx, atBlock); err != nil {
			return fmt.Errorf("loading outputs caches")
		}
	}

	return nil
}

func (p *Pipeline) initStoreSaveBoundary() {
	p.nextStoreSaveBoundary = p.computeNextStoreSaveBoundary(p.requestedStartBlockNum)
}

func (p *Pipeline) HandleStoreSaveBoundaries(ctx context.Context, span ttrace.Span, blockNum uint64) error {
	for p.nextStoreSaveBoundary <= blockNum {
		p.logger.Debug("saving stores on boundary", zap.Uint64("block_num", p.nextStoreSaveBoundary))
		span.AddEvent("store_save_boundary_reach")
		if err := p.saveStoresSnapshots(ctx, p.nextStoreSaveBoundary); err != nil {
			if span != nil {
				span.SetStatus(codes.Error, err.Error())
			}
			return fmt.Errorf("saving stores: %w", err)
		}
		p.bumpStoreSaveBoundary()
		if isStopBlockReached(blockNum, p.request.StopBlockNum) {
			break
		}
	}
	return nil
}

func (p *Pipeline) bumpStoreSaveBoundary() bool {
	p.nextStoreSaveBoundary = p.computeNextStoreSaveBoundary(p.nextStoreSaveBoundary)
	return p.request.StopBlockNum == p.nextStoreSaveBoundary
}
func (p *Pipeline) computeNextStoreSaveBoundary(fromBlock uint64) uint64 {
	nextBoundary := fromBlock - fromBlock%p.storeSaveInterval + p.storeSaveInterval
	if p.isSubrequest && p.request.StopBlockNum != 0 && p.request.StopBlockNum < nextBoundary {
		return p.request.StopBlockNum
	}
	return nextBoundary
}

func (p *Pipeline) ProcessBlock(block *bstream.Block, obj interface{}) (err error) {
	ctx, span := p.tracer.Start(p.context, "process_block")
	span.SetAttributes(attribute.Int64("block_num", int64(block.Num())))
	defer span.End()

	p.logger.Debug("processing block", zap.Uint64("block_num", block.Number))
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic at block %d: %s", block.Num(), r)
			p.logger.Error("panic while process block", zap.Uint64("block_num", block.Num()), zap.Error(err))
			p.logger.Error(string(debug.Stack()))
			span.SetStatus(codes.Error, err.Error())
		}
		if err != nil {
			for _, hook := range p.postJobHooks {
				if err := hook(ctx, p.clock); err != nil {
					p.logger.Warn("post job hook failed", zap.Error(err))
				}
			}
		}
	}()

	blockNum := block.Num()
	p.clock = &pbsubstreams.Clock{
		Number:    blockNum,
		Id:        block.Id,
		Timestamp: timestamppb.New(block.Time()),
	}
	p.currentBlockRef = block.AsRef()

	cursor := obj.(bstream.Cursorable).Cursor()
	step := obj.(bstream.Stepable).Step()

	if step.Matches(bstream.StepUndo) {
		span.AddEvent("handling_step_undo")
		if err = p.forkHandler.handleUndo(p.clock, cursor, p.moduleOutputCache, p.storeMap, p.respFunc); err != nil {
			span.SetStatus(codes.Error, err.Error())
			return fmt.Errorf("reverting outputs: %w", err)
		}
		return nil
	}

	if step.Matches(bstream.StepIrreversible) {
		// FIXME: what about bstream.StepNewIrreversible ??
		if err = p.moduleOutputCache.Update(ctx, p.currentBlockRef); err != nil {
			span.SetStatus(codes.Error, err.Error())
			return fmt.Errorf("updating module output cache: %w", err)
		}
	}

	if step.Matches(bstream.StepIrreversible) {
		// todo: should we send the output??
		span.AddEvent("handling_step_irreversible")
		p.forkHandler.handleIrreversible(block.Number)
	}

	if step.Matches(bstream.StepStalled) {
		span.AddEvent("handling_step_stalled")
		p.forkHandler.handleIrreversible(block.Number)
		return nil
	}

	for _, hook := range p.preBlockHooks {
		span.AddEvent("running_pre_block_hook", ttrace.WithAttributes(attribute.String("hook", fmt.Sprintf("%T", hook))))
		if err := hook(ctx, p.clock); err != nil {
			span.SetStatus(codes.Error, err.Error())
			return fmt.Errorf("pre block hook: %w", err)
		}
	}

	if err := p.HandleStoreSaveBoundaries(ctx, span, blockNum); err != nil {
		return err
	}
	if isStopBlockReached(blockNum, p.request.StopBlockNum) {
		p.logger.Debug("about to save cache output", zap.Uint64("clock", blockNum), zap.Uint64("stop_block", p.request.StopBlockNum))
		if err := p.moduleOutputCache.Flush(ctx); err != nil {
			span.SetStatus(codes.Error, err.Error())
			return fmt.Errorf("saving partial caches")
		}
		return io.EOF
	}

	if err = p.assignSource(block); err != nil {
		span.SetStatus(codes.Error, err.Error())
		return fmt.Errorf("setting up sources: %w", err)
	}

	ctx, execSpan := p.tracer.Start(ctx, "modules_executions")
	for _, executor := range p.moduleExecutors {
		err = p.runExecutor(ctx, executor, cursor.ToOpaque())
		if err != nil {
			//if returnErr := p.returnFailureProgress(err, executor); returnErr != nil {
			//	return fmt.Errorf("progress error: %w", returnErr)
			//}
			return err
		}
	}
	execSpan.End()

	// Snapshot all outputs, in case we undo
	// map[block_id]outputs

	if shouldReturnProgress(p.isSubrequest) {
		if err := p.returnModuleProgressOutputs(); err != nil {
			span.SetStatus(codes.Error, err.Error())
			return err
		}
	}

	if shouldReturnDataOutputs(blockNum, p.requestedStartBlockNum, p.isSubrequest) {
		p.logger.Debug("will return module outputs")
		if err := returnModuleDataOutputs(p.clock, step, cursor, p.moduleOutputs, p.respFunc); err != nil {
			span.SetStatus(codes.Error, err.Error())
			return err
		}
	}

	for _, s := range p.storeMap {
		s.Flush()
	}

	p.moduleOutputs = nil
	p.wasmOutputs = map[string][]byte{}

	p.logger.Debug("block processed", zap.Uint64("block_num", block.Number))
	span.SetStatus(codes.Ok, "")
	return nil
}

func (p *Pipeline) PartialsWritten() block.Ranges {
	return p.partialsWritten
}

func (p *Pipeline) runExecutor(ctx context.Context, executor ModuleExecutor, cursor string) error {
	//FIXME(abourget): should we ever skip that work?
	// if executor.ModuleInitialBlock < block.Number {
	// 	continue ??
	// }
	executorName := executor.Name()
	p.logger.Debug("executing", zap.String("module_name", executorName))

	err := executor.run(ctx, p.wasmOutputs, p.clock, cursor)
	if err != nil {
		logs, truncated := executor.moduleLogs()
		outputData := executor.moduleOutputData()
		if len(logs) != 0 || outputData != nil {
			p.moduleOutputs = append(p.moduleOutputs, &pbsubstreams.ModuleOutput{
				Name:          executorName,
				Data:          outputData,
				Logs:          logs,
				LogsTruncated: truncated,
			})
		}
		return fmt.Errorf("running module: %w", err)
	}

	if p.isOutputModule(executorName) {
		logs, truncated := executor.moduleLogs()
		outputData := executor.moduleOutputData()
		if len(logs) != 0 || outputData != nil {
			moduleOutput := &pbsubstreams.ModuleOutput{
				Name:          executorName,
				Data:          outputData,
				Logs:          logs,
				LogsTruncated: truncated,
			}
			p.moduleOutputs = append(p.moduleOutputs, moduleOutput)
			p.forkHandler.addModuleOutput(moduleOutput, p.clock.Number)
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
	if stopBlock == 0 {
		return false
	}

	return currentBlock >= stopBlock
}

func (p *Pipeline) returnModuleProgressOutputs() error {
	var progress []*pbsubstreams.ModuleProgress
	for _, store := range p.backprocessingStores {
		progress = append(progress, &pbsubstreams.ModuleProgress{
			Name: store.Name,
			Type: &pbsubstreams.ModuleProgress_ProcessedRanges{
				// FIXME charles: add p.hostname
				ProcessedRanges: &pbsubstreams.ModuleProgress_ProcessedRange{
					ProcessedRanges: []*pbsubstreams.BlockRange{
						{
							StartBlock: store.StoreInitialBlock(),
							EndBlock:   p.clock.Number,
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
				Logs:   failedExecutor.getCurrentExecutionStack(),
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
						Logs:          failedExecutor.getCurrentExecutionStack(),
						LogsTruncated: moduleOutput.LogsTruncated,
					},
				},
			})
		}
	}

	p.logger.Debug("return failed progress", zap.Int("progress_len", len(out)))
	return p.respFunc(substreams.NewModulesProgressResponse(out))
}

func (p *Pipeline) assignSource(block *bstream.Block) error {
	switch p.vmType {
	case "wasm/rust-v1":
		blkBytes, err := block.Payload.Get()
		if err != nil {
			return fmt.Errorf("getting block %d %q: %w", block.Number, block.Id, err)
		}

		clockBytes, err := proto.Marshal(p.clock)

		p.wasmOutputs[p.blockType] = blkBytes
		p.wasmOutputs["sf.substreams.v1.Clock"] = clockBytes
	default:
		panic("unsupported vmType " + p.vmType)
	}
	return nil
}

func (p *Pipeline) build() error {
	if err := p.validate(); err != nil {
		return fmt.Errorf("validate: %w", err)
	}
	if err := p.buildModules(); err != nil {
		return fmt.Errorf("build modules graph: %w", err)
	}
	return nil
}

func (p *Pipeline) validate() error {
	for _, binary := range p.request.Modules.Binaries {
		if binary.Type != "wasm/rust-v1" {
			return fmt.Errorf("unsupported binary type: %q, supported: %q", binary.Type, p.vmType)
		}
		p.vmType = binary.Type
	}
	return nil
}

func (p *Pipeline) buildModules() error {
	modules, err := p.graph.ModulesDownTo(p.request.OutputModules)
	if err != nil {
		return fmt.Errorf("building execution graph: %w", err)
	}
	p.modules = modules

	storeModules, err := p.graph.StoresDownTo(p.request.OutputModules)
	if err != nil {
		return err
	}
	p.storeModules = storeModules

	return nil
}

func (p *Pipeline) buildWASM(ctx context.Context, request *pbsubstreams.Request, modules []*pbsubstreams.Module) error {
	p.wasmOutputs = map[string][]byte{}
	p.wasmRuntime = wasm.NewRuntime(p.wasmExtensions)
	tracer := otel.GetTracerProvider().Tracer("executor")

	for _, module := range modules {
		isOutput := p.outputModuleMap[module.Name]
		var inputs []*wasm.Input

		for _, input := range module.Inputs {
			switch in := input.Input.(type) {
			case *pbsubstreams.Module_Input_Map_:
				inputs = append(inputs, &wasm.Input{
					Type: wasm.InputSource,
					Name: in.Map.ModuleName,
				})
			case *pbsubstreams.Module_Input_Store_:
				inputName := input.GetStore().ModuleName
				if input.GetStore().Mode == pbsubstreams.Module_Input_Store_DELTAS {
					inputs = append(inputs, &wasm.Input{
						Type:   wasm.InputStore,
						Name:   inputName,
						Store:  p.storeMap[inputName],
						Deltas: true,
					})
				} else {
					inputs = append(inputs, &wasm.Input{
						Type:  wasm.InputStore,
						Name:  inputName,
						Store: p.storeMap[inputName],
					})
					if p.storeMap[inputName] == nil {
						return fmt.Errorf("no store with name %q", inputName)
					}
				}

			case *pbsubstreams.Module_Input_Source_:
				inputs = append(inputs, &wasm.Input{
					Type: wasm.InputSource,
					Name: in.Source.Type,
				})
			default:
				return fmt.Errorf("invalid input struct for module %q", module.Name)
			}
		}

		modName := module.Name // to ensure it's enclosed
		entrypoint := module.BinaryEntrypoint
		code := p.request.Modules.Binaries[module.BinaryIndex]
		wasmModule, err := p.wasmRuntime.NewModule(ctx, request, code.Content, module.Name, entrypoint)
		if err != nil {
			return fmt.Errorf("new wasm module: %w", err)
		}

		switch kind := module.Kind.(type) {
		case *pbsubstreams.Module_KindMap_:
			outType := strings.TrimPrefix(module.Output.Type, "proto:")

			baseExecutor := BaseExecutor{
				moduleName: module.Name,
				wasmModule: wasmModule,
				entrypoint: entrypoint,
				wasmInputs: inputs,
				isOutput:   isOutput,
				tracer:     tracer,
			}

			baseExecutor.cache = p.moduleOutputCache.OutputCaches[module.Name]

			executor := &MapperModuleExecutor{
				BaseExecutor: baseExecutor,
				outputType:   outType,
			}

			p.moduleExecutors = append(p.moduleExecutors, executor)
			continue
		case *pbsubstreams.Module_KindStore_:
			updatePolicy := kind.KindStore.UpdatePolicy
			valueType := kind.KindStore.ValueType

			outputStore, found := p.storeMap[modName]
			if !found {
				return fmt.Errorf("store %q not found", modName)
			}
			inputs = append(inputs, &wasm.Input{
				Type:         wasm.OutputStore,
				Name:         modName,
				Store:        outputStore,
				UpdatePolicy: updatePolicy,
				ValueType:    valueType,
			})

			baseExecutor := BaseExecutor{
				moduleName: modName,
				isOutput:   isOutput,
				wasmModule: wasmModule,
				entrypoint: entrypoint,
				wasmInputs: inputs,
				tracer:     tracer,
			}

			baseExecutor.cache = p.moduleOutputCache.OutputCaches[module.Name]

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

func (p *Pipeline) saveStoresSnapshots(ctx context.Context, boundaryBlock uint64) error {
	for _, store := range p.storeMap {
		if p.isSubrequest && !p.isOutputModule(store.Name) {
			// skip saving snapshot for non-output stores in sub request
			continue
		}

		_, span := p.tracer.Start(ctx, "save_store_snapshot", ttrace.WithAttributes(attribute.String("store", store.Name)))
		stateWriter, err := store.WriteState(ctx, boundaryBlock)
		if err != nil {
			span.SetStatus(codes.Error, err.Error())
			span.End()
			return fmt.Errorf("store writer %q: %w", store.Name, err)
		}
		if err := stateWriter.Write(); err != nil {
			span.SetStatus(codes.Error, err.Error())
			span.End()
			return fmt.Errorf("writing store '%s' state: %w", store.Name, err)
		}
		span.SetStatus(codes.Ok, "")
		p.logger.Info("store written", zap.String("store_name", store.Name), zap.Object("store", store))

		if p.isSubrequest && p.isOutputModule(store.Name) {
			r := block.NewRange(store.StoreInitialBlock(), boundaryBlock)
			p.partialsWritten = append(p.partialsWritten, r)
			p.logger.Debug("adding partials written", zap.Object("range", r), zap.Stringer("ranges", p.partialsWritten), zap.Uint64("boundary_block", boundaryBlock))
			span.AddEvent("store_roll_trigger")
			store.Roll(boundaryBlock)
		}
		span.End()

	}
	return nil
}

func (p *Pipeline) buildStoreMap() (storeMap map[string]*state.Store, err error) {
	storeMap = map[string]*state.Store{}
	for _, storeModule := range p.storeModules {
		newStore, err := state.NewStore(
			storeModule.Name,
			p.storeSaveInterval,
			storeModule.InitialBlock,
			p.moduleHashes.Get(storeModule.Name),
			storeModule.GetKindStore().UpdatePolicy,
			storeModule.GetKindStore().ValueType,
			p.baseStateStore,
			p.logger,
		)
		if err != nil {
			return nil, fmt.Errorf("creating builder %s: %w", storeModule.Name, err)
		}
		storeMap[newStore.Name] = newStore
	}
	return storeMap, nil
}

func loadCompleteStores(ctx context.Context, storeMap map[string]*state.Store, requestedStartBlock uint64) error {
	for _, store := range storeMap {
		if store.StoreInitialBlock() == requestedStartBlock {
			continue
		}

		err := store.Fetch(ctx, requestedStartBlock)
		if err != nil {
			return fmt.Errorf("reading state for builder %q: %w", store.Name, err)
		}
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

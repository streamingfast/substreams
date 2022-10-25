package pipeline

import (
	"context"
	"fmt"
	"math"
	"strings"

	"github.com/streamingfast/bstream"
	"github.com/streamingfast/substreams"
	"github.com/streamingfast/substreams/block"
	"github.com/streamingfast/substreams/manifest"
	"github.com/streamingfast/substreams/orchestrator"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/pipeline/exec"
	"github.com/streamingfast/substreams/pipeline/execout"
	"github.com/streamingfast/substreams/reqctx"
	"github.com/streamingfast/substreams/service/config"
	"github.com/streamingfast/substreams/store"
	"github.com/streamingfast/substreams/wasm"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	ttrace "go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

type backprocessingStore struct {
	name            string
	initialBlockNum uint64
}

type Pipeline struct {
	// storing context since the processBlock interface
	// does not have context. For consistency, it should only be read in the processBlock function
	ctx context.Context

	vmType    string // wasm/rust-v1, native
	blockType string

	maxStoreSyncRangeSize uint64

	preBlockHooks  []substreams.BlockHook
	postBlockHooks []substreams.BlockHook
	postJobHooks   []substreams.PostJobHook

	wasmRuntime    *wasm.Runtime
	wasmExtensions []wasm.WASMExtensioner

	graph        *manifest.ModuleGraph
	moduleHashes *manifest.ModuleHashes
	respFunc     func(resp *pbsubstreams.Response) error

	outputModuleMap      map[string]bool
	backprocessingStores []*backprocessingStore

	moduleExecutors []exec.ModuleExecutor

	execOutputCache execout.CacheEngine

	moduleOutputs []*pbsubstreams.ModuleOutput
	forkHandler   *ForkHandler

	partialsWritten block.Ranges // when backprocessing, to report back to orchestrator

	runtimeConfig config.RuntimeConfig

	bounder  *StoreBoundary
	StoreMap store.Map
}

func New(
	ctx context.Context,
	graph *manifest.ModuleGraph,
	blockType string,
	wasmExtensions []wasm.WASMExtensioner,
	execOutputCache execout.CacheEngine,
	runtimeConfig config.RuntimeConfig,
	bounder *StoreBoundary,
	respFunc func(resp *pbsubstreams.Response) error,
	opts ...Option,
) *Pipeline {
	pipe := &Pipeline{
		ctx:                   ctx,
		execOutputCache:       execOutputCache,
		runtimeConfig:         runtimeConfig,
		graph:                 graph,
		outputModuleMap:       map[string]bool{},
		blockType:             blockType,
		wasmExtensions:        wasmExtensions,
		maxStoreSyncRangeSize: math.MaxUint64,
		respFunc:              respFunc,
		bounder:               bounder,
		forkHandler:           NewForkHandler(),
	}

	for _, opt := range opts {
		opt(pipe)
	}

	return pipe
}

func (p *Pipeline) Init(ctx context.Context) (err error) {
	reqDetails := reqctx.Details(ctx)
	logger := reqctx.Logger(ctx)
	ctx, span := reqctx.WithSpan(ctx, "pipeline_init")
	defer span.EndWithErr(&err)

	for _, name := range reqDetails.Request.OutputModules {
		p.outputModuleMap[name] = true
	}

	p.forkHandler.registerHandler(func(clock *pbsubstreams.Clock, moduleOutput *pbsubstreams.ModuleOutput) {
		p.execOutputCache.HandleUndo(clock, moduleOutput.Name)
	})

	p.forkHandler.registerHandler(func(clock *pbsubstreams.Clock, moduleOutput *pbsubstreams.ModuleOutput) {
		p.storesHandleUndo(moduleOutput)
	})

	logger.Info("initializing pipeline",
		zap.Int64("requested_start_block", reqDetails.Request.StartBlockNum),
		zap.Uint64("effective_start_block", reqDetails.EffectiveStartBlockNum),
		zap.Uint64("requested_stop_block", reqDetails.Request.StopBlockNum),
		zap.String("requested_start_cursor", reqDetails.Request.StartCursor),
		zap.Bool("is_back_processing", reqDetails.IsSubRequest),
		zap.Strings("outputs", reqDetails.Request.OutputModules),
	)

	if err := p.validateBinaries(ctx); err != nil {
		return fmt.Errorf("binary validation failed: %w", err)
	}

	modules, storeModules, err := p.getModules(ctx)
	if err != nil {
		return fmt.Errorf("failed to retrieve modules: %w", err)
	}

	if err := p.validateAndHashModules(ctx, modules); err != nil {
		return fmt.Errorf("module failed validation: %w", err)
	}

	logger.Info("priming caching engine")
	if err := p.execOutputCache.Init(p.moduleHashes); err != nil {
		return fmt.Errorf("failed to prime caching engine: %w", err)
	}

	logger.Info("initializing store configurations", zap.Int("store_count", len(storeModules)))
	storeConfigs, err := p.initializeStoreConfigs(storeModules)
	if err != nil {
		return fmt.Errorf("initialize store config map: %w", err)
	}

	var storeMap store.Map
	if reqDetails.IsSubRequest {
		logger.Info("stores loaded", zap.Object("stores", p.StoreMap))
		if storeMap, err = p.setupSubrequestStores(ctx, storeConfigs); err != nil {
			return fmt.Errorf("faile to setup backprocessings: %w", err)
		}
	} else {
		if storeMap, err = p.runBackProcessAndSetupStores(ctx, storeConfigs); err != nil {
			return fmt.Errorf("failed setup request: %w", err)
		}
	}
	p.StoreMap = storeMap

	if err = p.buildWASM(ctx, modules); err != nil {
		return fmt.Errorf("initiating module output caches: %w", err)
	}

	p.bounder.InitBoundary(reqDetails.EffectiveStartBlockNum)

	logger.Info("initialized store boundary block",
		zap.Uint64("effective_start_block", reqDetails.EffectiveStartBlockNum),
		zap.Uint64("next_boundary_block", p.bounder.nextBoundary),
	)

	return nil
}

func (p *Pipeline) setupSubrequestStores(ctx context.Context, storeConfigs store.ConfigMap) (store.Map, error) {
	reqDetails := reqctx.Details(ctx)
	logger := reqctx.Logger(ctx)
	outputStoreCount := len(reqDetails.Request.OutputModules)
	if outputStoreCount > 1 {
		// currently only support 1 lead stores
		return nil, fmt.Errorf("invalid number of backprocess leaf store: %d", outputStoreCount)
	}

	outputModuleName := reqDetails.Request.OutputModules[0]

	// there is an assumption that in backgprocess mode the outputModule is a store
	if _, found := storeConfigs[outputModuleName]; !found {
		return nil, fmt.Errorf("requested output module %q is not found in store configurations", outputModuleName)
	}

	ttrace.SpanContextFromContext(context.Background())
	storeMap := store.NewMap()

	for name, storeConfig := range storeConfigs {
		if name == outputModuleName {
			partialStore := storeConfig.NewPartialKV(reqDetails.EffectiveStartBlockNum, logger)
			storeMap.Set(partialStore)

			p.backprocessingStores = append(p.backprocessingStores, &backprocessingStore{
				name:            partialStore.Name(),
				initialBlockNum: partialStore.InitialBlock(),
			})
		} else {
			fullStore := storeConfig.NewFullKV(logger)

			if fullStore.InitialBlock() != reqDetails.EffectiveStartBlockNum {
				if err := fullStore.Load(ctx, reqDetails.EffectiveStartBlockNum); err != nil {
					return nil, fmt.Errorf("load partial store: %w", err)
				}
			}

			storeMap.Set(fullStore)
		}
	}

	return storeMap, nil
}

func (p *Pipeline) runBackProcessAndSetupStores(ctx context.Context, storeConfigs store.ConfigMap) (storeMap store.Map, err error) {
	ctx, span := reqctx.WithSpan(ctx, "backprocess")
	defer span.EndWithErr(&err)
	reqDetails := reqctx.Details(ctx)
	o := orchestrator.New(
		p.runtimeConfig,
		reqDetails.EffectiveStartBlockNum,
		p.graph,
		p.respFunc,
		storeConfigs,
		reqDetails.Request.Modules,
	)

	storeMap, err = o.Run(ctx)
	if err != nil {
		return nil, fmt.Errorf("backrprocess run: %w", err)
	}

	p.backprocessingStores = nil

	if err := p.sendSnapshots(ctx); err != nil {
		return nil, fmt.Errorf("send initial snapshots: %w", err)
	}

	return storeMap, nil
}

func (p *Pipeline) isOutputModule(name string) bool {
	_, found := p.outputModuleMap[name]
	return found
}

func (p *Pipeline) validateAndHashModules(ctx context.Context, modules []*pbsubstreams.Module) error {
	reqDetails := reqctx.Details(ctx)

	p.moduleHashes = manifest.NewModuleHashes()

	for _, module := range modules {
		isOutput := p.outputModuleMap[module.Name]
		if isOutput && reqDetails.EffectiveStartBlockNum < module.InitialBlock {
			return fmt.Errorf("start block %d smaller than request outputs for module %q with start block %d", reqDetails.EffectiveStartBlockNum, module.Name, module.InitialBlock)
		}
		p.moduleHashes.HashModule(reqDetails.Request.Modules, module, p.graph)
	}

	return nil
}

func (p *Pipeline) runPostJobHooks(ctx context.Context, clock *pbsubstreams.Clock) {
	for _, hook := range p.postJobHooks {
		if err := hook(ctx, clock); err != nil {
			reqctx.Logger(ctx).Warn("post job hook failed", zap.Error(err))
		}
	}
}

func (p *Pipeline) runPreBlockHooks(ctx context.Context, clock *pbsubstreams.Clock) (err error) {
	_, span := reqctx.WithSpan(ctx, "pre_block_hooks")
	defer span.EndWithErr(&err)

	for _, hook := range p.preBlockHooks {
		span.AddEvent("running_pre_block_hook", ttrace.WithAttributes(attribute.String("hook", fmt.Sprintf("%T", hook))))
		if err := hook(ctx, clock); err != nil {
			return fmt.Errorf("pre block hook: %w", err)
		}
	}
	return nil
}

func (p *Pipeline) execute(ctx context.Context, executor exec.ModuleExecutor, execOutput execout.ExecutionOutput) (err error) {
	logger := reqctx.Logger(ctx)

	executor.Reset()

	executorName := executor.Name()
	logger.Debug("executing", zap.Uint64("block", execOutput.Clock().Number), zap.String("module_name", executorName))

	output, runError := exec.RunModule(ctx, executor, execOutput)
	returnOutput := func() {
		if output != nil {
			p.moduleOutputs = append(p.moduleOutputs, output)
		}
	}
	if runError != nil {
		returnOutput()
		return fmt.Errorf("execute module: %w", runError)
	}

	if p.isOutputModule(executor.Name()) {
		returnOutput()
	}

	p.forkHandler.addReversibleOutput(output, execOutput.Clock().Number)

	return nil
}

func shouldReturn(blockNum, effectiveStartBlockNum uint64) bool {
	return blockNum >= effectiveStartBlockNum
}

func shouldReturnProgress(isSubRequest bool) bool {
	return isSubRequest
}

func shouldReturnDataOutputs(blockNum, effectiveStartBlockNum uint64, isSubRequest bool) bool {
	return shouldReturn(blockNum, effectiveStartBlockNum) && !isSubRequest
}

func (p *Pipeline) returnModuleProgressOutputs(clock *pbsubstreams.Clock) error {
	var progress []*pbsubstreams.ModuleProgress
	for _, backprocessStore := range p.backprocessingStores {
		progress = append(progress, &pbsubstreams.ModuleProgress{
			Name: backprocessStore.name,
			Type: &pbsubstreams.ModuleProgress_ProcessedRanges{
				// TODO charles: add p.hostname
				ProcessedRanges: &pbsubstreams.ModuleProgress_ProcessedRange{
					ProcessedRanges: []*pbsubstreams.BlockRange{
						{
							StartBlock: backprocessStore.initialBlockNum,
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

func (p *Pipeline) validateBinaries(ctx context.Context) error {
	reqDetails := reqctx.Details(ctx)
	for _, binary := range reqDetails.Request.Modules.Binaries {
		if binary.Type != "wasm/rust-v1" {
			return fmt.Errorf("unsupported binary type: %q, supported: %q", binary.Type, p.vmType)
		}
		p.vmType = binary.Type
	}
	return nil
}

func (p *Pipeline) getModules(ctx context.Context) (modules []*pbsubstreams.Module, storeModules []*pbsubstreams.Module, err error) {
	reqDetails := reqctx.Details(ctx)
	if modules, err = p.graph.ModulesDownTo(reqDetails.Request.OutputModules); err != nil {
		return nil, nil, fmt.Errorf("building execution moduleGraph: %w", err)
	}
	if storeModules, err = p.graph.StoresDownTo(reqDetails.Request.OutputModules); err != nil {
		return nil, nil, err
	}
	return modules, storeModules, nil
}

func (p *Pipeline) buildWASM(ctx context.Context, modules []*pbsubstreams.Module) error {
	request := reqctx.Details(ctx).Request
	p.wasmRuntime = wasm.NewRuntime(p.wasmExtensions)
	tracer := otel.GetTracerProvider().Tracer("executor")

	for _, module := range modules {
		inputs, err := p.renderWasmInputs(module)
		if err != nil {
			return fmt.Errorf("module %q: get wasm inputs: %w", module.Name, err)
		}

		modName := module.Name // to ensure it's enclosed
		entrypoint := module.BinaryEntrypoint
		code := request.Modules.Binaries[module.BinaryIndex]
		wasmModule, err := p.wasmRuntime.NewModule(ctx, request, code.Content, module.Name, entrypoint)
		if err != nil {
			return fmt.Errorf("new wasm module: %w", err)
		}

		switch kind := module.Kind.(type) {
		case *pbsubstreams.Module_KindMap_:
			outType := strings.TrimPrefix(module.Output.Type, "proto:")

			baseExecutor := exec.NewBaseExecutor(
				module.Name,
				wasmModule,
				inputs,
				entrypoint,
				tracer,
			)

			executor := exec.NewMapperModuleExecutor(baseExecutor, outType)

			p.moduleExecutors = append(p.moduleExecutors, executor)
			continue
		case *pbsubstreams.Module_KindStore_:
			updatePolicy := kind.KindStore.UpdatePolicy
			valueType := kind.KindStore.ValueType

			outputStore, found := p.StoreMap.Get(modName)
			if !found {
				return fmt.Errorf(" store %q not found", modName)
			}
			inputs = append(inputs, wasm.NewStoreWriterOutput(modName, outputStore, updatePolicy, valueType))

			baseExecutor := exec.NewBaseExecutor(
				modName,
				wasmModule,
				inputs,
				entrypoint,
				tracer,
			)

			s := exec.NewStoreModuleExecutor(baseExecutor, outputStore)

			p.moduleExecutors = append(p.moduleExecutors, s)
			continue
		default:
			return fmt.Errorf("invalid kind %q input module %q", module.Kind, module.Name)
		}
	}

	return nil
}

func (p *Pipeline) initializeStoreConfigs(storeModules []*pbsubstreams.Module) (out store.ConfigMap, err error) {
	out = make(store.ConfigMap)
	for _, storeModule := range storeModules {

		c, err := store.NewConfig(
			storeModule.Name,
			storeModule.InitialBlock,
			p.moduleHashes.Get(storeModule.Name),
			storeModule.GetKindStore().UpdatePolicy,
			storeModule.GetKindStore().ValueType,
			p.runtimeConfig.BaseObjectStore,
		)
		if err != nil {
			return nil, fmt.Errorf("new config for store %q: %w", storeModule.Name, err)
		}
		out[storeModule.Name] = c
	}
	return out, nil
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

func (p *Pipeline) renderWasmInputs(module *pbsubstreams.Module) (out []wasm.Argument, err error) {
	storeAccessor := p.StoreMap
	for _, input := range module.Inputs {
		switch in := input.Input.(type) {
		case *pbsubstreams.Module_Input_Map_:
			out = append(out, wasm.NewMapInput(in.Map.ModuleName))
		case *pbsubstreams.Module_Input_Store_:
			inputName := input.GetStore().ModuleName
			if input.GetStore().Mode == pbsubstreams.Module_Input_Store_DELTAS {
				out = append(out, wasm.NewMapInput(inputName))
			} else {
				inputStore, found := storeAccessor.Get(inputName)
				if !found {
					return nil, fmt.Errorf("store %q npt found", inputName)
				}
				out = append(out, wasm.NewStoreReaderInput(inputName, inputStore))
			}
		case *pbsubstreams.Module_Input_Source_:
			if !p.isValidWasmSourceInputType(in.Source.Type) {
				return nil, fmt.Errorf("input source has an unknown type: %q", in.Source.Type)
			}
			out = append(out, wasm.NewSourceInput(in.Source.Type))
		default:
			return nil, fmt.Errorf("invalid input struct for module %q", module.Name)
		}
	}
	return out, nil
}

func (p *Pipeline) isValidWasmSourceInputType(inputType string) bool {
	return (p.blockType == inputType) || (wasm.CLOCK_TYPE == inputType)
}

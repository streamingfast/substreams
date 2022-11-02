package pipeline

import (
	"context"
	"fmt"
	"github.com/streamingfast/bstream"
	"github.com/streamingfast/substreams"
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
	"strings"
)

type backprocessingStore struct {
	name            string
	initialBlockNum uint64
}

type Pipeline struct {
	ctx           context.Context
	runtimeConfig config.RuntimeConfig

	preBlockHooks  []substreams.BlockHook
	postBlockHooks []substreams.BlockHook
	postJobHooks   []substreams.PostJobHook

	wasmRuntime     *wasm.Runtime
	moduleTree      *ModuleTree
	moduleExecutors []exec.ModuleExecutor
	moduleOutputs   []*pbsubstreams.ModuleOutput

	respFunc func(resp *pbsubstreams.Response) error

	backprocessingStores []*backprocessingStore

	execOutputCache execout.CacheEngine

	forkHandler *ForkHandler

	stores *Stores
}

func New(
	ctx context.Context,
	moduleTree *ModuleTree,
	stores *Stores,
	wasmRuntime *wasm.Runtime,
	execOutputCache execout.CacheEngine,
	runtimeConfig config.RuntimeConfig,
	respFunc func(resp *pbsubstreams.Response) error,
	opts ...Option,
) *Pipeline {
	pipe := &Pipeline{
		ctx:             ctx,
		execOutputCache: execOutputCache,
		runtimeConfig:   runtimeConfig,
		moduleTree:      moduleTree,
		wasmRuntime:     wasmRuntime,
		respFunc:        respFunc,
		stores:          stores,
		forkHandler:     NewForkHandler(),
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

	// Registers some ForkHandlers (to adjust exec output, and stores states)
	p.forkHandler.registerHandler(func(clock *pbsubstreams.Clock, moduleOutput *pbsubstreams.ModuleOutput) {
		p.execOutputCache.HandleUndo(clock, moduleOutput.Name)
	})
	p.forkHandler.registerHandler(func(clock *pbsubstreams.Clock, moduleOutput *pbsubstreams.ModuleOutput) {
		p.stores.storesHandleUndo(moduleOutput)
	})

	// Initialization of the Store Provider, ExecOut Cache Engine?

	logger.Info("initializing exec output cache")
	if err := p.execOutputCache.Init(p.moduleTree.moduleHashes); err != nil {
		return fmt.Errorf("failed to prime caching engine: %w", err)
	}

	// FIXME(abourget): Populate the StoreProvider by one of two means: on-disk snapshots, or parallel backprocessing.
	// This clearly doesn't belong in the Init() function.
	var storeMap store.Map
	if reqDetails.IsSubRequest {
		logger.Info("stores loaded", zap.Object("stores", p.stores.StoreMap))
		if storeMap, err = p.setupSubrequestStores(ctx); err != nil {
			return fmt.Errorf("faile to setup backprocessings: %w", err)
		}
	} else {
		if storeMap, err = p.runBackProcessAndSetupStores(ctx); err != nil {
			return fmt.Errorf("failed setup request: %w", err)
		}

		if err := p.sendSnapshots(ctx, storeMap); err != nil {
			return fmt.Errorf("send initial snapshots: %w", err)
		}
	}
	p.stores.SetStoreMap(storeMap)

	// Build the Module Executor list
	// TODO(abourget): this could be done lazily, but the ModuleTree,
	// and cache the latest if all block boundaries
	// are still clear.

	if err = p.buildWASM(ctx, p.moduleTree.processModules); err != nil {
		return fmt.Errorf("initiating module output caches: %w", err)
	}

	return nil
}

func (p *Pipeline) GetStoreMap() store.Map {
	return p.stores.StoreMap
}

func (p *Pipeline) setupSubrequestStores(ctx context.Context) (store.Map, error) {
	reqDetails := reqctx.Details(ctx)
	logger := reqctx.Logger(ctx)
	outputStoreCount := len(reqDetails.Request.OutputModules)
	if outputStoreCount > 1 {
		// currently only support 1 lead stores
		return nil, fmt.Errorf("invalid number of backprocess leaf store: %d", outputStoreCount)
	}

	outputModuleName := reqDetails.Request.OutputModules[0]

	// there is an assumption that in backgprocess mode the outputModule is a store
	if _, found := p.stores.configs[outputModuleName]; !found {
		return nil, fmt.Errorf("requested output module %q is not found in store configurations", outputModuleName)
	}

	ttrace.SpanContextFromContext(context.Background())
	storeMap := store.NewMap()

	for name, storeConfig := range p.stores.configs {
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
					return nil, fmt.Errorf("load full store: %w", err)
				}
			}

			storeMap.Set(fullStore)
		}
	}

	return storeMap, nil
}

func (p *Pipeline) runBackProcessAndSetupStores(ctx context.Context) (storeMap store.Map, err error) {
	ctx, span := reqctx.WithSpan(ctx, "backprocess")
	defer span.EndWithErr(&err)
	reqDetails := reqctx.Details(ctx)
	reqStats := reqctx.ReqStats(ctx)
	logger := reqctx.Logger(ctx)

	logger.Info("starting back processing")
	backproc, err := orchestrator.BuildBackprocessor(
		p.ctx,
		p.runtimeConfig,
		reqDetails.EffectiveStartBlockNum,
		p.moduleTree.graph,
		p.respFunc,
		p.stores.configs,
		reqDetails.Request.Modules,
	)
	if err != nil {
		return nil, fmt.Errorf("building backproc: %w", err)
	}

	reqStats.StartBackProcessing()
	storeMap, err = backproc.Run(ctx)
	if err != nil {
		return nil, fmt.Errorf("backrprocess run: %w", err)
	}
	reqStats.EndBackProcessing()

	p.backprocessingStores = nil

	return storeMap, nil
}

func (p *Pipeline) isOutputModule(name string) bool {
	return p.moduleTree.IsOutputModule(name)
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

	executor.ResetWASMInstance()

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

func shouldReturnProgress(isSubRequest bool) bool {
	return isSubRequest
}

func shouldReturnDataOutputs(blockNum, effectiveStartBlockNum uint64, isSubRequest bool) bool {
	return shouldReturn(blockNum, effectiveStartBlockNum) && !isSubRequest
}

func shouldReturn(blockNum, effectiveStartBlockNum uint64) bool {
	return blockNum >= effectiveStartBlockNum
}

func (p *Pipeline) returnModuleProgressOutputs(clock *pbsubstreams.Clock) error {
	var progress []*pbsubstreams.ModuleProgress
	for _, backprocessStore := range p.backprocessingStores {
		progress = append(progress, &pbsubstreams.ModuleProgress{
			Name: backprocessStore.name,
			Type: &pbsubstreams.ModuleProgress_ProcessedRanges{
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

func (p *Pipeline) buildWASM(ctx context.Context, modules []*pbsubstreams.Module) error {
	request := reqctx.Details(ctx).Request
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

		case *pbsubstreams.Module_KindStore_:
			updatePolicy := kind.KindStore.UpdatePolicy
			valueType := kind.KindStore.ValueType

			outputStore, found := p.stores.StoreMap.Get(modName)
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

		default:
			panic(fmt.Errorf("invalid kind %q input module %q", module.Kind, module.Name))
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

func (p *Pipeline) renderWasmInputs(module *pbsubstreams.Module) (out []wasm.Argument, err error) {
	storeAccessor := p.stores.StoreMap
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
			// in.Source.Type checking against `blockType` is already done
			// upfront in `validateGraph`.
			out = append(out, wasm.NewSourceInput(in.Source.Type))
		default:
			return nil, fmt.Errorf("invalid input struct for module %q", module.Name)
		}
	}
	return out, nil
}

package pipeline

import (
	"context"
	"fmt"
	"strings"

	"github.com/streamingfast/substreams/pipeline/outputmodules"

	"github.com/streamingfast/bstream"
	"github.com/streamingfast/substreams"
	"github.com/streamingfast/substreams/orchestrator"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/pipeline/cache"
	"github.com/streamingfast/substreams/pipeline/exec"
	"github.com/streamingfast/substreams/reqctx"
	"github.com/streamingfast/substreams/service/config"
	"github.com/streamingfast/substreams/storage/execout"
	"github.com/streamingfast/substreams/storage/store"
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
	ctx           context.Context
	runtimeConfig config.RuntimeConfig

	preBlockHooks  []substreams.BlockHook
	postBlockHooks []substreams.BlockHook
	postJobHooks   []substreams.PostJobHook

	wasmRuntime     *wasm.Runtime
	outputGraph     *outputmodules.Graph
	moduleExecutors []exec.ModuleExecutor
	moduleOutputs   []*pbsubstreams.ModuleOutput
	respFunc        func(resp *pbsubstreams.Response) error

	stores         *Stores
	execoutStorage *execout.Configs
	partialStores  []*backprocessingStore

	gate *gate

	forkHandler     *ForkHandler
	execOutputCache *cache.Engine

	// lastFinalClock should always be either THE `stopBlock` or a block beyond that point
	// (for chains with potential block skips)
	lastFinalClock *pbsubstreams.Clock
}

func New(
	ctx context.Context,
	outputGraph *outputmodules.Graph,
	stores *Stores,
	execoutStorage *execout.Configs,
	wasmRuntime *wasm.Runtime,
	execOutputCache *cache.Engine,
	runtimeConfig config.RuntimeConfig,
	respFunc func(resp *pbsubstreams.Response) error,
	opts ...Option,
) *Pipeline {
	pipe := &Pipeline{
		ctx:             ctx,
		gate:            newGate(ctx),
		execOutputCache: execOutputCache,
		runtimeConfig:   runtimeConfig,
		outputGraph:     outputGraph,
		wasmRuntime:     wasmRuntime,
		respFunc:        respFunc,
		stores:          stores,
		execoutStorage:  execoutStorage,
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

	p.forkHandler.registerUndoHandler(func(clock *pbsubstreams.Clock, moduleOutputs []*pbsubstreams.ModuleOutput) {
		for _, modOut := range moduleOutputs {
			p.stores.storesHandleUndo(modOut)
		}
	})

	// DESTROY Init
	// TODO(abourget):
	//  when in a SubRequest, the caller should call `setupSubrequestStores` and take the result, assign it
	//  to the `stores.SetStoreMap()` and also keep track of the `partialStores` (extracted from looping through
	//  that map)
	var storeMap store.Map
	if reqDetails.IsSubRequest {
		logger.Info("stores loaded", zap.Object("stores", p.stores.StoreMap))
		if storeMap, err = p.setupSubrequestStores(ctx); err != nil {
			return fmt.Errorf("failed to load stores: %w", err)
		}
	} else {
		if storeMap, err = p.runBackProcessAndSetupStores(ctx); err != nil {
			return fmt.Errorf("failed setup request: %w", err)
		}
	}
	p.stores.SetStoreMap(storeMap)

	// TODO(abourget): Build the Module Executor list: this could be done lazily, but the outputmodules.Graph,
	//  and cache the latest if all block boundaries
	//  are still clear.

	if err = p.buildWASM(ctx, p.outputGraph.AllModules()); err != nil {
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

	outputModuleName := reqDetails.Request.GetOutputModuleName()

	ttrace.SpanContextFromContext(context.Background())
	storeMap := store.NewMap()

	for name, storeConfig := range p.stores.configs {
		if name == outputModuleName {
			partialStore := storeConfig.NewPartialKV(reqDetails.RequestStartBlockNum, logger)
			storeMap.Set(partialStore)

			p.partialStores = append(p.partialStores, &backprocessingStore{
				name:            partialStore.Name(),
				initialBlockNum: partialStore.InitialBlock(),
			})
		} else {
			fullStore := storeConfig.NewFullKV(logger)

			//fixme: should we check if we don't have a boundary finished to not load ?
			if fullStore.InitialBlock() != reqDetails.RequestStartBlockNum {
				if err := fullStore.Load(ctx, reqDetails.RequestStartBlockNum); err != nil {
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

	backprocessor, err := orchestrator.BuildBackProcessor(
		p.ctx,
		reqDetails,
		p.runtimeConfig,
		p.outputGraph,
		p.execoutStorage,
		p.respFunc,
		p.stores.configs,
	)
	if err != nil {
		return nil, fmt.Errorf("building backprocessor: %w", err)
	}

	logger.Info("starting back processing")

	reqStats.StartBackProcessing()
	storeMap, err = backprocessor.Run(ctx)
	if err != nil {
		return nil, fmt.Errorf("backprocess run: %w", err)
	}
	reqStats.EndBackProcessing()

	p.partialStores = nil

	return storeMap, nil
}

func (p *Pipeline) isOutputModule(name string) bool {
	return p.outputGraph.IsOutputModule(name)
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
	hasValidOutput := executor.HasValidOutput()
	logger.Debug("executing", zap.Uint64("block", execOutput.Clock().Number), zap.String("module_name", executorName))

	moduleOutput, outputBytes, runError := exec.RunModule(ctx, executor, execOutput)
	if runError != nil {
		if hasValidOutput {
			p.appendModuleOutputs(moduleOutput)
		}
		return fmt.Errorf("execute module: %w", runError)
	}

	if !hasValidOutput {
		return nil
	}
	if p.isOutputModule(executor.Name()) {
		p.appendModuleOutputs(moduleOutput)
	}
	if err := execOutput.Set(executorName, outputBytes); err != nil {
		return fmt.Errorf("set output cache: %w", err)
	}
	if moduleOutput != nil {
		p.forkHandler.addReversibleOutput(moduleOutput, execOutput.Clock().Number)
	}
	return nil
}

func (p *Pipeline) appendModuleOutputs(moduleOutput *pbsubstreams.ModuleOutput) {
	if moduleOutput != nil {
		p.moduleOutputs = append(p.moduleOutputs, moduleOutput)
	}
}
func (p *Pipeline) returnModuleProgressOutputs(clock *pbsubstreams.Clock) error {
	var progress []*pbsubstreams.ModuleProgress
	for _, backprocessStore := range p.partialStores {
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

// TODO(abourget): have this being generated and the `buildWASM` by taking
// this Graph as input, and creating the ModuleExecutors, and caching
// them over there.
// moduleExecutorsInitialized bool
// moduleExecutors            []exec.ModuleExecutor
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
				return fmt.Errorf("store %q not found", modName)
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

func newGate(ctx context.Context) *gate {
	reqDetails := reqctx.Details(ctx)
	return &gate{
		disabled:             reqDetails.IsSubRequest,
		requestStartBlockNum: reqDetails.RequestStartBlockNum,
	}
}

type gate struct {
	requestStartBlockNum uint64
	disabled             bool
	passed               bool
	snapshotSent         bool
}

func (g *gate) processBlock(blockNum uint64, step bstream.StepType) {
	if g.disabled || g.passed {
		return
	}

	if blockTriggersGate(blockNum, g.requestStartBlockNum, step) {
		g.passed = true
	}

}

func (g *gate) shouldSendSnapshot() bool {
	if g.snapshotSent {
		return false
	}

	if g.passed {
		g.snapshotSent = true
		return true
	}
	return false
}

func (g *gate) shouldSendOutputs() bool {
	return g.passed
}

func blockTriggersGate(blockNum, requestStartBlockNum uint64, step bstream.StepType) bool {
	if step.Matches(bstream.StepNew) {
		return blockNum >= requestStartBlockNum
	}
	if step.Matches(bstream.StepUndo) {
		return blockNum+1 == requestStartBlockNum //  FIXME undo case will require additional previousBlock in cursor
	}
	return false
}

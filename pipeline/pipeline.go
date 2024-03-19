package pipeline

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/streamingfast/bstream"
	"github.com/streamingfast/dmetering"
	"go.opentelemetry.io/otel"
	"go.uber.org/zap"

	"github.com/streamingfast/substreams"
	"github.com/streamingfast/substreams/orchestrator"
	"github.com/streamingfast/substreams/orchestrator/plan"
	"github.com/streamingfast/substreams/orchestrator/response"
	pbssinternal "github.com/streamingfast/substreams/pb/sf/substreams/intern/v2"
	pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/pipeline/cache"
	"github.com/streamingfast/substreams/pipeline/exec"
	"github.com/streamingfast/substreams/pipeline/outputmodules"
	"github.com/streamingfast/substreams/reqctx"
	"github.com/streamingfast/substreams/service/config"
	"github.com/streamingfast/substreams/storage/execout"
	"github.com/streamingfast/substreams/storage/store"
	"github.com/streamingfast/substreams/wasm"
)

type processingModule struct {
	name            string
	initialBlockNum uint64
}

type Pipeline struct {
	ctx           context.Context
	runtimeConfig config.RuntimeConfig

	pendingUndoMessage *pbsubstreamsrpc.Response
	preBlockHooks      []substreams.BlockHook
	postBlockHooks     []substreams.BlockHook
	postJobHooks       []substreams.PostJobHook

	wasmRuntime     *wasm.Registry
	outputGraph     *outputmodules.Graph
	loadedModules   map[uint32]wasm.Module
	moduleExecutors [][]exec.ModuleExecutor // Staged module executors
	executionStages outputmodules.ExecutionStages

	mapModuleOutput         *pbsubstreamsrpc.MapModuleOutput
	extraMapModuleOutputs   []*pbsubstreamsrpc.MapModuleOutput
	extraStoreModuleOutputs []*pbsubstreamsrpc.StoreModuleOutput

	respFunc         substreams.ResponseFunc
	lastProgressSent time.Time

	startTime      time.Time
	modulesStats   map[string]*pbssinternal.ModuleStats
	stores         *Stores
	execoutStorage *execout.Configs

	processingModule *processingModule

	gate            *gate
	finalBlocksOnly bool
	highestStage    *int

	forkHandler     *ForkHandler
	insideReorgUpTo bstream.BlockRef

	execOutputCache *cache.Engine

	// lastFinalClock should always be either THE `stopBlock` or a block beyond that point
	// (for chains with potential block skips)
	lastFinalClock *pbsubstreams.Clock

	blockStepMap map[bstream.StepType]uint64
}

func New(
	ctx context.Context,
	outputGraph *outputmodules.Graph,
	stores *Stores,
	execoutStorage *execout.Configs,
	wasmRuntime *wasm.Registry,
	execOutputCache *cache.Engine,
	runtimeConfig config.RuntimeConfig,
	respFunc substreams.ResponseFunc,
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
		blockStepMap:    make(map[bstream.StepType]uint64),
		startTime:       time.Now(),
	}
	for _, opt := range opts {
		opt(pipe)
	}
	return pipe
}

func (p *Pipeline) Init(ctx context.Context) (err error) {
	reqDetails := reqctx.Details(ctx)

	p.forkHandler.registerUndoHandler(func(clock *pbsubstreams.Clock, moduleOutputs []*pbssinternal.ModuleOutput) {
		for _, modOut := range moduleOutputs {
			p.stores.storesHandleUndo(modOut)
		}
	})

	p.setupProcessingModule(reqDetails)

	stagedModules := p.outputGraph.StagedUsedModules()

	// truncate stages to highest scheduled stage
	if highest := p.highestStage; highest != nil {
		if len(stagedModules) < *highest+1 {
			return fmt.Errorf("invalid stage %d, there aren't that many", highest)
		}
		stagedModules = stagedModules[0 : *highest+1]
	}
	p.executionStages = stagedModules

	return nil
}

func (p *Pipeline) InitTier2Stores(ctx context.Context) (err error) {

	storeMap, err := p.setupSubrequestStores(ctx)
	if err != nil {
		return fmt.Errorf("subrequest stores setup failed: %w", err)
	}

	p.stores.SetStoreMap(storeMap)

	logger := reqctx.Logger(ctx)
	logger.Debug("stores loaded", zap.Object("stores", p.stores.StoreMap), zap.Int("stage", reqctx.Details(ctx).Tier2Stage))

	return nil
}

func (p *Pipeline) InitTier1StoresAndBackprocess(ctx context.Context, reqPlan *plan.RequestPlan) (err error) {

	if reqPlan.RequiresParallelProcessing() {
		storeMap, err := p.runParallelProcess(ctx, reqPlan)
		if err != nil {
			return fmt.Errorf("run_parallel_process failed: %w", err)
		}
		p.stores.SetStoreMap(storeMap) // this is valid even if we don't have stores in the parallelProcessing but only a mapper
		return nil
	}

	p.stores.SetStoreMap(p.setupEmptyStores(ctx))
	return nil
}

func (p *Pipeline) GetStoreMap() store.Map {
	return p.stores.StoreMap
}

func (p *Pipeline) setupProcessingModule(reqDetails *reqctx.RequestDetails) {
	for _, module := range reqDetails.Modules.Modules {
		if reqDetails.IsOutputModule(module.Name) {
			p.processingModule = &processingModule{
				name:            module.GetName(),
				initialBlockNum: reqDetails.ResolvedStartBlockNum,
			}
		}
	}
}

// setupSubrequestsStores will prepare stores for all required modules up to the current stage.
func (p *Pipeline) setupSubrequestStores(ctx context.Context) (storeMap store.Map, err error) {
	ctx, span := reqctx.WithSpan(ctx, "substreams/pipeline/tier2/store_setup")
	defer span.EndWithErr(&err)

	reqDetails := reqctx.Details(ctx)
	logger := reqctx.Logger(ctx)

	storeMap = store.NewMap()

	lastStage := len(p.executionStages) - 1
	for stageIdx, stage := range p.executionStages {
		if p.highestStage != nil && stageIdx > *p.highestStage {
			break // skip stores for stages that we're not running
		}
		isLastStage := stageIdx == lastStage
		layer := stage.LastLayer()
		if !layer.IsStoreLayer() {
			continue
		}
		for _, mod := range layer {
			storeConfig := p.stores.configs[mod.Name]

			if isLastStage {
				partialStore := storeConfig.NewPartialKV(reqDetails.ResolvedStartBlockNum, logger)
				storeMap.Set(partialStore)

			} else {
				fullStore := storeConfig.NewFullKV(logger)

				if fullStore.InitialBlock() != reqDetails.ResolvedStartBlockNum {
					file := store.NewCompleteFileInfo(fullStore.Name(), fullStore.InitialBlock(), reqDetails.ResolvedStartBlockNum)
					// FIXME: run debugging session with conditional breakpoint
					// `request.Stage == 1 && request.StartBlockNum == 20`
					// in tier2.go: on the call to InitTier2Stores.
					// Things stall in this LOAD command:
					if err := fullStore.Load(ctx, file); err != nil {
						return nil, fmt.Errorf("load full store %s (%s): %w", storeConfig.Name(), storeConfig.ModuleHash(), err)
					}
				}
				storeMap.Set(fullStore)
			}
		}
	}

	return storeMap, nil
}

func (p *Pipeline) setupEmptyStores(ctx context.Context) store.Map {
	logger := reqctx.Logger(ctx)
	storeMap := store.NewMap()
	for _, storeConfig := range p.stores.configs {
		fullStore := storeConfig.NewFullKV(logger)
		storeMap.Set(fullStore)
	}
	return storeMap
}

// runParallelProcess
func (p *Pipeline) runParallelProcess(ctx context.Context, reqPlan *plan.RequestPlan) (storeMap store.Map, err error) {
	ctx, span := reqctx.WithSpan(ctx, "substreams/pipeline/tier1/parallel_process")
	defer span.EndWithErr(&err)

	reqDetails := reqctx.Details(ctx)
	reqStats := reqctx.ReqStats(ctx)
	logger := reqctx.Logger(ctx)

	if reqDetails.ShouldStreamCachedOutputs() && p.pendingUndoMessage != nil {
		p.respFunc(p.pendingUndoMessage)
	}

	parallelProcessor, err := orchestrator.BuildParallelProcessor(
		ctx,
		reqPlan,
		p.runtimeConfig,
		int(reqDetails.MaxParallelJobs),
		p.outputGraph,
		p.execoutStorage,
		p.respFunc,
		p.stores.configs,
	)
	if err != nil {
		return nil, fmt.Errorf("building parallel processor: %w", err)
	}

	stats := reqctx.ReqStats(ctx)
	progressCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() {
		stream := response.New(p.respFunc)

		meter := dmetering.GetBytesMeter(ctx)
		for {
			select {
			case <-time.After(time.Millisecond * 500):
				stagesProgress := stats.Stages()
				jobs := stats.JobsStats()
				modStats := stats.AggregatedModulesStats()
				remoteBytesRead, remoteBytesWritten := stats.RemoteBytesConsumption()

				stream.SendModulesStats(modStats, stagesProgress, jobs, meter.BytesRead()+remoteBytesRead, meter.BytesWritten()+remoteBytesWritten)
			case <-progressCtx.Done():
				return
			}
		}
	}()

	logger.Debug("starting parallel processing")

	storeMap, err = parallelProcessor.Run(ctx)
	if err != nil {
		return nil, fmt.Errorf("parallel processing run: %w", err)
	}
	reqStats.RecordInitializationComplete()

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
	for _, hook := range p.preBlockHooks {
		if err := hook(ctx, clock); err != nil {
			return fmt.Errorf("pre block hook: %w", err)
		}
	}
	return nil
}

// TODO: move this to `responses`
func toRPCStoreModuleOutputs(in *pbssinternal.ModuleOutput) (out *pbsubstreamsrpc.StoreModuleOutput) {
	deltas := in.GetStoreDeltas()
	if deltas == nil {
		return nil
	}

	return &pbsubstreamsrpc.StoreModuleOutput{
		Name:             in.ModuleName,
		DebugStoreDeltas: toRPCDeltas(deltas),
		DebugInfo: &pbsubstreamsrpc.OutputDebugInfo{
			Logs:          in.Logs,
			LogsTruncated: in.DebugLogsTruncated,
			Cached:        in.Cached,
		},
	}
}

func toRPCDeltas(in *pbsubstreams.StoreDeltas) (out []*pbsubstreamsrpc.StoreDelta) {
	if len(in.StoreDeltas) == 0 {
		return nil
	}

	out = make([]*pbsubstreamsrpc.StoreDelta, len(in.StoreDeltas))
	for i, d := range in.StoreDeltas {
		out[i] = &pbsubstreamsrpc.StoreDelta{
			Operation: toRPCOperation(d.Operation),
			Ordinal:   d.Ordinal,
			Key:       d.Key,
			OldValue:  d.OldValue,
			NewValue:  d.NewValue,
		}
	}
	return
}

func toRPCOperation(in pbsubstreams.StoreDelta_Operation) (out pbsubstreamsrpc.StoreDelta_Operation) {
	switch in {
	case pbsubstreams.StoreDelta_UPDATE:
		return pbsubstreamsrpc.StoreDelta_UPDATE
	case pbsubstreams.StoreDelta_CREATE:
		return pbsubstreamsrpc.StoreDelta_CREATE
	case pbsubstreams.StoreDelta_DELETE:
		return pbsubstreamsrpc.StoreDelta_DELETE
	}
	return pbsubstreamsrpc.StoreDelta_UNSET
}

func toRPCMapModuleOutputs(in *pbssinternal.ModuleOutput) (out *pbsubstreamsrpc.MapModuleOutput) {
	data := in.GetMapOutput()
	if data == nil {
		return nil
	}
	return &pbsubstreamsrpc.MapModuleOutput{
		Name:      in.ModuleName,
		MapOutput: data,
		DebugInfo: &pbsubstreamsrpc.OutputDebugInfo{
			Logs:          in.Logs,
			LogsTruncated: in.DebugLogsTruncated,
			Cached:        in.Cached,
		},
	}
}

func (p *Pipeline) returnRPCModuleProgressOutputs(clock *pbsubstreams.Clock, forceOutput bool) error {
	if time.Since(p.lastProgressSent) < progressMessageInterval && !forceOutput {
		return nil
	}
	p.lastProgressSent = time.Now()

	stats := reqctx.ReqStats(p.ctx)
	stream := response.New(p.respFunc)
	stagesProgress := stats.Stages()
	jobs := stats.JobsStats()
	modStats := stats.AggregatedModulesStats()

	meter := dmetering.GetBytesMeter(p.ctx)
	remoteBytesRead, remoteBytesWritten := stats.RemoteBytesConsumption()
	return stream.SendModulesStats(modStats, stagesProgress, jobs, meter.BytesRead()+remoteBytesRead, meter.BytesWritten()+remoteBytesWritten)

}

func (p *Pipeline) toInternalUpdate(clock *pbsubstreams.Clock) *pbssinternal.Update {
	meter := dmetering.GetBytesMeter(p.ctx)

	return &pbssinternal.Update{
		ProcessedBlocks:   clock.Number - p.processingModule.initialBlockNum,
		DurationMs:        uint64(time.Since(p.startTime).Milliseconds()),
		TotalBytesRead:    meter.BytesRead(),
		TotalBytesWritten: meter.BytesWritten(),
		ModulesStats:      reqctx.ReqStats(p.ctx).LocalModulesStats(),
	}
}

func (p *Pipeline) returnInternalModuleProgressOutputs(clock *pbsubstreams.Clock, forceOutput bool) error {
	if time.Since(p.lastProgressSent) < progressMessageInterval && !forceOutput {
		return nil
	}
	p.lastProgressSent = time.Now()

	upd := p.toInternalUpdate(clock)

	out := &pbssinternal.ProcessRangeResponse{
		Type: &pbssinternal.ProcessRangeResponse_Update{
			Update: upd,
		},
	}

	if err := p.respFunc(out); err != nil {
		return fmt.Errorf("calling return func: %w", err)
	}
	return nil
}

// buildModuleExecutors builds the moduleExecutors, and the loadedModules.
func (p *Pipeline) buildModuleExecutors(ctx context.Context) ([][]exec.ModuleExecutor, error) {
	if p.moduleExecutors != nil {
		// Eventually, we can invalidate our catch to accomodate the PATCH
		// and rebuild all the modules, and tear down the previously loaded ones.
		return p.moduleExecutors, nil
	}

	reqModules := reqctx.Details(ctx).Modules
	tracer := otel.GetTracerProvider().Tracer("executor")

	loadedModules := make(map[uint32]wasm.Module)
	for _, stage := range p.executionStages {
		for _, layer := range stage {
			for _, module := range layer {
				if _, exists := loadedModules[module.BinaryIndex]; exists {
					continue
				}
				code := reqModules.Binaries[module.BinaryIndex]
				m, err := p.wasmRuntime.NewModule(ctx, code.Content)
				if err != nil {
					return nil, fmt.Errorf("new wasm module: %w", err)
				}
				loadedModules[module.BinaryIndex] = m
			}
		}
	}

	p.loadedModules = loadedModules

	var stagedModuleExecutors [][]exec.ModuleExecutor
	for _, stage := range p.executionStages {
		for _, layer := range stage {
			var moduleExecutors []exec.ModuleExecutor
			for _, module := range layer {
				inputs, err := p.renderWasmInputs(module)
				if err != nil {
					return nil, fmt.Errorf("module %q: get wasm inputs: %w", module.Name, err)
				}

				entrypoint := module.BinaryEntrypoint
				mod := loadedModules[module.BinaryIndex]

				switch kind := module.Kind.(type) {
				case *pbsubstreams.Module_KindMap_:
					outType := strings.TrimPrefix(module.Output.Type, "proto:")
					baseExecutor := exec.NewBaseExecutor(
						ctx,
						module.Name,
						mod,
						p.wasmRuntime.InstanceCacheEnabled(),
						inputs,
						entrypoint,
						tracer,
					)
					executor := exec.NewMapperModuleExecutor(baseExecutor, outType)
					moduleExecutors = append(moduleExecutors, executor)

				case *pbsubstreams.Module_KindStore_:
					updatePolicy := kind.KindStore.UpdatePolicy
					valueType := kind.KindStore.ValueType

					outputStore, found := p.stores.StoreMap.Get(module.Name)
					if !found {
						return nil, fmt.Errorf("store %q not found", module.Name)
					}
					inputs = append(inputs, wasm.NewStoreWriterOutput(module.Name, outputStore, updatePolicy, valueType))

					baseExecutor := exec.NewBaseExecutor(
						ctx,
						module.Name,
						mod,
						p.wasmRuntime.InstanceCacheEnabled(),
						inputs,
						entrypoint,
						tracer,
					)
					executor := exec.NewStoreModuleExecutor(baseExecutor, outputStore)
					moduleExecutors = append(moduleExecutors, executor)

				default:
					panic(fmt.Errorf("invalid kind %q input module %q", module.Kind, module.Name))
				}
			}
			stagedModuleExecutors = append(stagedModuleExecutors, moduleExecutors)
		}
	}

	p.moduleExecutors = stagedModuleExecutors
	return stagedModuleExecutors, nil
}

func (p *Pipeline) cleanUpModuleExecutors(ctx context.Context) error {
	for _, stage := range p.moduleExecutors {
		for _, executor := range stage {
			if err := executor.Close(ctx); err != nil {
				return fmt.Errorf("closing module executor %q: %w", executor.Name(), err)
			}
		}
	}
	for idx, mod := range p.loadedModules {
		if err := mod.Close(ctx); err != nil {
			return fmt.Errorf("closing wasm module %d: %w", idx, err)
		}
	}
	return nil
}

func returnModuleDataOutputs(
	clock *pbsubstreams.Clock,
	cursor *bstream.Cursor,
	mapModuleOutput *pbsubstreamsrpc.MapModuleOutput,
	extraMapModuleOutputs []*pbsubstreamsrpc.MapModuleOutput,
	extraStoreModuleOutputs []*pbsubstreamsrpc.StoreModuleOutput,
	respFunc substreams.ResponseFunc,
) error {
	out := &pbsubstreamsrpc.BlockScopedData{
		Clock:             clock,
		Output:            mapModuleOutput,
		DebugMapOutputs:   extraMapModuleOutputs,
		DebugStoreOutputs: extraStoreModuleOutputs,
		Cursor:            cursor.ToOpaque(),
		FinalBlockHeight:  cursor.LIB.Num(),
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
		case *pbsubstreams.Module_Input_Params_:
			out = append(out, wasm.NewParamsInput(input.GetParams().GetValue()))
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

package pipeline

import (
	"context"
	"fmt"
	"io"
	"math"
	"runtime/debug"
	"strings"
	"time"

	"github.com/streamingfast/bstream"
	"github.com/streamingfast/derr"
	"github.com/streamingfast/dstore"
	"github.com/streamingfast/substreams"
	"github.com/streamingfast/substreams/manifest"
	"github.com/streamingfast/substreams/orchestrator"
	"github.com/streamingfast/substreams/orchestrator/worker"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/pipeline/outputs"
	"github.com/streamingfast/substreams/progress"
	"github.com/streamingfast/substreams/state"
	"github.com/streamingfast/substreams/wasm"
	"go.uber.org/zap"
	"golang.org/x/exp/slices"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type Pipeline struct {
	vmType    string // wasm/rust-v1, native
	blockType string

	requestedStartBlockNum uint64
	maxStoreSyncRangeSize  uint64
	partialMode            bool

	preBlockHooks  []substreams.BlockHook
	postBlockHooks []substreams.BlockHook
	postJobHooks   []substreams.PostJobHook

	wasmRuntime    *wasm.Runtime
	wasmExtensions []wasm.WASMExtensioner
	builders       map[string]*state.Builder

	context           context.Context
	request           *pbsubstreams.Request
	graph             *manifest.ModuleGraph
	outputModuleNames []string
	outputModuleMap   map[string]bool

	modules         []*pbsubstreams.Module
	stores          []*pbsubstreams.Module
	moduleExecutors []ModuleExecutor
	wasmOutputs     map[string][]byte

	progressTracker    *progress.Tracker
	allowInvalidState  bool
	baseStateStore     dstore.Store
	storesSaveInterval uint64

	clock         *pbsubstreams.Clock
	moduleOutputs []*pbsubstreams.ModuleOutput
	logs          []string

	moduleOutputCache *outputs.ModulesOutputCache

	currentBlockRef bstream.BlockRef

	outputCacheSaveBlockInterval uint64
	blockRangeSizeSubrequests    int
	grpcClientFactory            func() (pbsubstreams.StreamClient, []grpc.CallOption, error)
}

func New(
	ctx context.Context,
	request *pbsubstreams.Request,
	graph *manifest.ModuleGraph,
	blockType string,
	baseStateStore dstore.Store,
	outputCacheSaveBlockInterval uint64,
	wasmExtensions []wasm.WASMExtensioner,
	grpcClientFactory func() (pbsubstreams.StreamClient, []grpc.CallOption, error),
	blockRangeSizeSubRequests int,
	opts ...Option) *Pipeline {

	pipe := &Pipeline{
		context:                      ctx,
		request:                      request,
		builders:                     map[string]*state.Builder{},
		graph:                        graph,
		baseStateStore:               baseStateStore,
		outputModuleNames:            request.OutputModules,
		outputModuleMap:              map[string]bool{},
		blockType:                    blockType,
		progressTracker:              progress.NewProgressTracker(),
		wasmExtensions:               wasmExtensions,
		grpcClientFactory:            grpcClientFactory,
		outputCacheSaveBlockInterval: outputCacheSaveBlockInterval,
		blockRangeSizeSubrequests:    blockRangeSizeSubRequests,

		maxStoreSyncRangeSize: math.MaxUint64,
	}

	for _, name := range request.OutputModules {
		pipe.outputModuleMap[name] = true
	}

	for _, opt := range opts {
		opt(pipe)
	}

	return pipe
}

// `store` aura 4 modes d'opération:
//   * fetch an absolute snapshot from disk at EXACTLY the point we're starting
//   * fetch a partial snapshot, and fuse with previous snapshots, in which I need local "pairExtractor" building.
//   * connect to a remote firehose (I can cut the upstream dependencies
//   * if resources are available, SCHEDULE on BACKING NODES a parallel processing for that segment
//   * completely roll out LOCALLY the full historic reprocessing BEFORE continuing

func (p *Pipeline) HandlerFactory(workerPool *worker.Pool, respFunc func(resp *pbsubstreams.Response) error) (bstream.Handler, error) {
	ctx := p.context
	// WARN: we don't support < 0 StartBlock for now
	p.requestedStartBlockNum = uint64(p.request.StartBlockNum)
	zlog.Info("initializing handler", zap.Uint64("requested_start_block", p.requestedStartBlockNum), zap.Uint64("requested_stop_block", p.request.StopBlockNum), zap.Bool("partial_mode", p.partialMode), zap.Strings("outputs", p.request.OutputModules))
	p.moduleOutputCache = outputs.NewModuleOutputCache(p.outputCacheSaveBlockInterval)

	var err error
	zlog.Info("building store and executor")
	var stores []*state.Builder
	p.modules, _, stores, err = p.build()
	if err != nil {
		return nil, fmt.Errorf("building pipeline: %w", err)
	}

	for _, module := range p.modules {
		isOutput := p.outputModuleMap[module.Name]
		if isOutput && p.requestedStartBlockNum < module.InitialBlock {
			return nil, fmt.Errorf("invalid request: start block %d smaller that request outputs for module: %q start block %d", p.requestedStartBlockNum, module.Name, module.InitialBlock)
		}

		hash := manifest.HashModuleAsString(p.request.Modules, p.graph, module)
		_, err := p.moduleOutputCache.RegisterModule(ctx, module, hash, p.baseStateStore, p.requestedStartBlockNum)
		if err != nil {
			return nil, fmt.Errorf("registering output cache for module %q: %w", module.Name, err)
		}

	}

	p.progressTracker.StartTracking(ctx)

	if !p.partialMode {
		err = SynchronizeStores(
			ctx,
			workerPool,
			p.request, stores,
			p.graph, p.moduleOutputCache.OutputCaches, p.requestedStartBlockNum, respFunc, p.blockRangeSizeSubrequests,
			p.storesSaveInterval,
			p.maxStoreSyncRangeSize,
		)
		if err != nil {
			return nil, fmt.Errorf("synchonizing stores: %w", err)
		}
	}

	zlog.Info("initializing stores")
	if err = p.InitializeStores(ctx); err != nil {
		return nil, fmt.Errorf("initializing stores: %w", err)
	}

	err = p.buildWASM(ctx, p.request, p.modules)
	if err != nil {
		return nil, fmt.Errorf("initiating module output caches: %w", err)
	}

	for _, cache := range p.moduleOutputCache.OutputCaches {
		atBlock := outputs.ComputeStartBlock(p.requestedStartBlockNum, p.outputCacheSaveBlockInterval)
		if _, err := cache.Load(ctx, atBlock); err != nil {
			return nil, fmt.Errorf("loading outputs caches")
		}
	}

	return bstream.HandlerFunc(func(block *bstream.Block, obj interface{}) (err error) {
		handleStart := time.Now()

		defer func() {
			if r := recover(); r != nil {
				err = fmt.Errorf("panic at block %d: %s", block.Num(), r)
				zlog.Error("panic while process block", zap.Uint64("block_num", block.Num()), zap.Error(err))
				zlog.Error(string(debug.Stack()))
			}
			if err != nil {
				for _, hook := range p.postJobHooks {
					if err := hook(ctx, p.clock); err != nil {
						zlog.Warn("post job hook failed", zap.Error(err))
					}
				}
			}
		}()

		p.clock = &pbsubstreams.Clock{
			Number:    block.Num(),
			Id:        block.Id,
			Timestamp: timestamppb.New(block.Time()),
		}

		p.currentBlockRef = block.AsRef()

		if err = p.moduleOutputCache.Update(ctx, p.currentBlockRef); err != nil {
			return fmt.Errorf("updating module output cache: %w", err)
		}

		//requestedOutputStores := p.request.GetOutputModules()
		//optimizedModuleExecutors, skipBlockSource := OptimizeExecutors(p.moduleOutputCache.outputCaches, p.moduleExecutors, requestedOutputStores)
		//optimizedModuleExecutors, skipBlockSource := OptimizeExecutors(p.moduleOutputCache.outputCaches, p.moduleExecutors, requestedOutputStores)

		for _, hook := range p.preBlockHooks {
			if err := hook(ctx, p.clock); err != nil {
				return fmt.Errorf("pre block hook: %w", err)
			}
		}

		p.moduleOutputs = nil
		p.wasmOutputs = map[string][]byte{}

		//todo? should we only save store if in partial mode or in catchup?
		// no need to save store if loaded from cache?
		// TODO: eventually, handle the `undo` signals.
		//  NOTE: The RUNTIME will handle the undo signals. It'll have all it needs.
		isFirstRequestBlock := p.requestedStartBlockNum == p.clock.Number
		intervalReached := p.storesSaveInterval != 0 && p.clock.Number%p.storesSaveInterval == 0
		if !isFirstRequestBlock && intervalReached {
			if err := p.saveStoresSnapshots(ctx); err != nil {
				return fmt.Errorf("saving stores: %w", err)
			}

		}

		if p.clock.Number >= p.request.StopBlockNum && p.request.StopBlockNum != 0 {
			if p.partialMode {
				zlog.Debug("about to save partial output", zap.Uint64("clock", p.clock.Number), zap.Uint64("stop_block", p.request.StopBlockNum))
				if err := p.moduleOutputCache.Save(ctx); err != nil {
					return fmt.Errorf("saving partial caches")
				}
			}
			return io.EOF
		}

		zlog.Debug("processing block", zap.Uint64("block_num", block.Number))

		cursor := obj.(bstream.Cursorable).Cursor()
		step := obj.(bstream.Stepable).Step()

		//if !skipBlockSource {
		if err = p.assignSource(block); err != nil {
			return fmt.Errorf("setting up sources: %w", err)
		}
		//}

		for _, executor := range p.moduleExecutors {
			zlog.Debug("executing", zap.Stringer("module_name", executor))
			err := executor.run(p.wasmOutputs, p.clock, block)
			if err != nil {
				if returnErr := p.returnFailureProgress(err, executor, respFunc); returnErr != nil {
					return returnErr
				}

				return err
			}

			logs, truncated := executor.moduleLogs()

			p.moduleOutputs = append(p.moduleOutputs, &pbsubstreams.ModuleOutput{
				Name:          executor.Name(),
				Data:          executor.moduleOutputData(),
				Logs:          logs,
				LogsTruncated: truncated,
			})
		}

		p.progressTracker.BlockProcessed(block, time.Since(handleStart))

		if p.clock.Number >= p.requestedStartBlockNum {
			if err := p.returnOutputs(step, cursor, respFunc); err != nil {
				return err
			}
		}

		for _, s := range p.builders {
			s.Flush()
		}
		zlog.Debug("block processed", zap.Uint64("block_num", block.Number))
		return nil
	}), nil
}

func (p *Pipeline) returnOutputs(step bstream.StepType, cursor *bstream.Cursor, respFunc substreams.ResponseFunc) error {
	if len(p.moduleOutputs) > 0 {
		zlog.Debug("got modules outputs", zap.Int("module_output_count", len(p.moduleOutputs)))
		out := &pbsubstreams.BlockScopedData{
			Outputs: p.moduleOutputs,
			Clock:   p.clock,
			Step:    pbsubstreams.StepToProto(step),
			Cursor:  cursor.ToOpaque(),
		}

		if err := respFunc(substreams.NewBlockScopedDataResponse(out)); err != nil {
			return fmt.Errorf("calling return func: %w", err)
		}
	}

	if p.partialMode {
		var modules []*pbsubstreams.ModuleProgress

		for _, mod := range p.modules {
			// FIXME: build a list so we don't need to check "slices.Contains" here in the hot path
			if slices.Contains(p.request.OutputModules, mod.Name) {
				modules = append(modules, &pbsubstreams.ModuleProgress{
					Name: mod.Name,
					Type: &pbsubstreams.ModuleProgress_ProcessedRanges{
						ProcessedRanges: &pbsubstreams.ModuleProgress_ProcessedRange{
							ProcessedRanges: []*pbsubstreams.BlockRange{
								{
									StartBlock: p.requestedStartBlockNum,
									EndBlock:   p.progressTracker.LastBlock,
								},
							},
						},
					},
				})
			}
		}

		if err := respFunc(substreams.NewModulesProgressResponse(modules)); err != nil {
			return fmt.Errorf("calling return func: %w", err)
		}
	}
	return nil
}

func (p *Pipeline) returnFailureProgress(err error, failedExecutor ModuleExecutor, respFunc substreams.ResponseFunc) error {
	modules := make([]*pbsubstreams.ModuleProgress, len(p.moduleOutputs)+1)

	for i, moduleOutput := range p.moduleOutputs {
		modules[i] = &pbsubstreams.ModuleProgress{
			Name: moduleOutput.Name,
			Type: &pbsubstreams.ModuleProgress_Failed_{
				Failed: &pbsubstreams.ModuleProgress_Failed{
					Logs:          moduleOutput.Logs,
					LogsTruncated: moduleOutput.LogsTruncated,
				},
			},
		}
	}

	logs, truncated := failedExecutor.moduleLogs()

	modules[len(p.moduleOutputs)] = &pbsubstreams.ModuleProgress{
		Name: failedExecutor.Name(),

		Type: &pbsubstreams.ModuleProgress_Failed_{
			Failed: &pbsubstreams.ModuleProgress_Failed{
				// Should we maybe extract specific WASM error and improved the "printing" here?
				Reason:        err.Error(),
				Logs:          logs,
				LogsTruncated: truncated,
			},
		},
	}

	return respFunc(substreams.NewModulesProgressResponse(modules))
}

func (p *Pipeline) assignSource(block *bstream.Block) error {

	switch p.vmType {
	case "wasm/rust-v1":
		// TODO: avoid serializing/deserializing and all, just pass in the BYTES directly
		// we have decided NEVER to do mutations in here, but rather to have data fixing processes
		// when it is possible to do, otherwise have a full blown different VERSION of data,
		// so that DATA is always the reference, and not a living process.
		// So we can TRUST the data blindly here
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

func (p *Pipeline) build() (modules []*pbsubstreams.Module, storeModules []*pbsubstreams.Module, stores []*state.Builder, err error) {
	for _, binary := range p.request.Modules.Binaries {
		if binary.Type != "wasm/rust-v1" {
			return nil, nil, nil, fmt.Errorf("unsupported binary type: %q, supported: %q", binary.Type, p.vmType)
		}
		p.vmType = binary.Type
	}

	modules, err = p.graph.ModulesDownTo(p.outputModuleNames)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("building execution graph: %w", err)
	}

	p.builders = make(map[string]*state.Builder)
	storeModules, err = p.graph.StoresDownTo(p.outputModuleNames)
	for _, storeModule := range storeModules {
		var options []state.BuilderOption

		builder, err := state.NewBuilder(
			storeModule.Name,
			p.storesSaveInterval,
			storeModule.InitialBlock,
			manifest.HashModuleAsString(p.request.Modules, p.graph, storeModule),
			storeModule.GetKindStore().UpdatePolicy,
			storeModule.GetKindStore().ValueType,
			p.baseStateStore,
			options...,
		)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("creating builder %s: %w", storeModule.Name, err)
		}

		stores = append(stores, builder)

		p.builders[builder.Name] = builder
	}

	return
}

func (p *Pipeline) buildWASM(ctx context.Context, request *pbsubstreams.Request, modules []*pbsubstreams.Module) error {
	p.wasmOutputs = map[string][]byte{}
	p.wasmRuntime = wasm.NewRuntime(p.wasmExtensions)

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
						Store:  p.builders[inputName],
						Deltas: true,
					})
				} else {
					inputs = append(inputs, &wasm.Input{
						Type:  wasm.InputStore,
						Name:  inputName,
						Store: p.builders[inputName],
					})
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
		wasmModule, err := p.wasmRuntime.NewModule(ctx, request, code.Content, module.Name)
		if err != nil {
			return fmt.Errorf("new wasm module: %w", err)
		}

		switch kind := module.Kind.(type) {
		case *pbsubstreams.Module_KindMap_:
			outType := strings.TrimPrefix(module.Output.Type, "proto:")

			executor := &MapperModuleExecutor{
				BaseExecutor: BaseExecutor{
					moduleName: module.Name,
					wasmModule: wasmModule,
					entrypoint: entrypoint,
					wasmInputs: inputs,
					isOutput:   isOutput,
					cache:      p.moduleOutputCache.OutputCaches[module.Name],
				},
				outputType: outType,
			}

			p.moduleExecutors = append(p.moduleExecutors, executor)
			continue
		case *pbsubstreams.Module_KindStore_:
			updatePolicy := kind.KindStore.UpdatePolicy
			valueType := kind.KindStore.ValueType

			outputStore := p.builders[modName]
			inputs = append(inputs, &wasm.Input{
				Type:         wasm.OutputStore,
				Name:         modName,
				Store:        outputStore,
				UpdatePolicy: updatePolicy,
				ValueType:    valueType,
			})

			s := &StoreModuleExecutor{
				BaseExecutor: BaseExecutor{
					moduleName: modName,
					isOutput:   isOutput,
					wasmModule: wasmModule,
					entrypoint: entrypoint,
					wasmInputs: inputs,
					cache:      p.moduleOutputCache.OutputCaches[module.Name],
				},
				outputStore: outputStore,
			}

			p.moduleExecutors = append(p.moduleExecutors, s)
			continue
		default:
			return fmt.Errorf("invalid kind %q input module %q", module.Kind, module.Name)
		}
	}

	return nil
}

func SynchronizeStores(
	ctx context.Context,
	workerPool *worker.Pool,
	originalRequest *pbsubstreams.Request,
	builders []*state.Builder,
	graph *manifest.ModuleGraph,
	outputCache map[string]*outputs.OutputCache,
	upToBlockNum uint64,
	respFunc substreams.ResponseFunc,
	blockRangeSizeSubRequests int,
	storeSaveInterval uint64,
	maxSubrequestRangeSize uint64) error {

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	zlog.Info("synchronizing stores")

	requestPool := orchestrator.NewRequestPool()

	squasher, err := orchestrator.NewSquasher(ctx, builders, outputCache, storeSaveInterval, orchestrator.WithNotifier(requestPool))
	if err != nil {
		return fmt.Errorf("initializing squasher: %w", err)
	}

	strategy, err := orchestrator.NewOrderedStrategy(ctx, originalRequest, builders, graph, requestPool, upToBlockNum, blockRangeSizeSubRequests, maxSubrequestRangeSize)
	if err != nil {
		return fmt.Errorf("creating strategy: %w", err)
	}

	scheduler, err := orchestrator.NewScheduler(ctx, strategy, squasher, blockRangeSizeSubRequests)
	if err != nil {
		return fmt.Errorf("initializing scheduler: %w", err)
	}

	requestCount := strategy.RequestCount()
	if requestCount == 0 {
		return nil
	}
	result := make(chan error)
	for {
		req, err := scheduler.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		job := &worker.Job{
			Request: req,
		}

		start := time.Now()
		zlog.Info("waiting worker", zap.Object("job", job))
		jobWorker := workerPool.Borrow()
		zlog.Info("got worker", zap.Object("job", job), zap.Duration("in", time.Since(start)))

		select {
		case <-ctx.Done():
			zlog.Info("synchronize stores quit on cancel context")
			return nil
		default:
		}

		go func() {
			w := jobWorker
			j := job

			err := derr.RetryContext(ctx, 2, func(ctx context.Context) error {
				return w.Run(ctx, j, respFunc)
			})
			workerPool.ReturnWorker(w)
			if err != nil {
				result <- err
			}
			err = scheduler.Callback(ctx, req)
			if err != nil {
				result <- fmt.Errorf("calling back scheduler: %w", err)
			}
			result <- nil
		}()
	}

	resultCount := 0

done:
	for {
		select {
		case <-ctx.Done():
			return nil
		case err := <-result:
			resultCount++
			if err != nil {
				return fmt.Errorf("from worker: %w", err)
			}
			zlog.Debug("received result", zap.Int("result_count", resultCount), zap.Int("request_count", requestCount), zap.Error(err))
			if resultCount == requestCount {
				break done
			}
		}
	}

	zlog.Info("store sync completed")

	if err := squasher.Close(); err != nil {
		return fmt.Errorf("closing squasher: %w", err)
	}

	return nil
}

func (p *Pipeline) saveStoresSnapshots(ctx context.Context) error {
	for _, builder := range p.builders {
		err := builder.WriteState(ctx)
		if err != nil {
			return fmt.Errorf("writing store '%s' state: %w", builder.Name, err)
		}

		if p.partialMode {
			builder.RollPartial()
			continue
		}
		builder.Roll()

		zlog.Info("state written", zap.String("store_name", builder.Name))
	}

	return nil
}

func (p *Pipeline) InitializeStores(ctx context.Context) error {
	var initFunc func(builder *state.Builder) error
	initFunc = func(builder *state.Builder) error {

		if p.partialMode && slices.Contains(p.request.OutputModules, builder.Name) {
			return builder.InitializePartial(ctx, p.requestedStartBlockNum)
		}
		outputCache := p.moduleOutputCache.OutputCaches[builder.Name]
		return builder.Initialize(ctx, p.requestedStartBlockNum, p.moduleOutputCache.SaveBlockInterval, outputCache.Store)
	}

	for _, builder := range p.builders {
		err := initFunc(builder)
		if err != nil {
			return fmt.Errorf("reading state for builder %q: %w", builder.Name, err)
		}
	}
	return nil
}

package pipeline

import (
	"context"
	"fmt"
	"github.com/streamingfast/substreams/scheduler"
	"github.com/streamingfast/substreams/squasher"
	"io"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/streamingfast/bstream"
	"github.com/streamingfast/dstore"
	"github.com/streamingfast/substreams"
	"github.com/streamingfast/substreams/manifest"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/pipeline/outputs"
	"github.com/streamingfast/substreams/state"
	"github.com/streamingfast/substreams/wasm"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type Pipeline struct {
	vmType    string // wasm/rust-v1, native
	blockType string

	requestedStartBlockNum uint64
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
	manifest          *pbsubstreams.Manifest
	outputModuleNames []string
	outputModuleMap   map[string]bool

	modules         []*pbsubstreams.Module
	moduleExecutors []ModuleExecutor
	wasmOutputs     map[string][]byte

	progressTracker    *progressTracker
	allowInvalidState  bool
	baseStateStore     dstore.Store
	storesSaveInterval uint64

	clock         *pbsubstreams.Clock
	moduleOutputs []*pbsubstreams.ModuleOutput
	logs          []string

	moduleOutputCache *outputs.ModulesOutputCache

	currentBlockRef bstream.BlockRef

	grpcClient                   pbsubstreams.StreamClient
	grpcCallOpts                 []grpc.CallOption
	outputCacheSaveBlockInterval uint64
}

func New(
	ctx context.Context,
	request *pbsubstreams.Request,
	graph *manifest.ModuleGraph,
	blockType string,
	baseStateStore dstore.Store,
	outputCacheSaveBlockInterval uint64,
	wasmExtensions []wasm.WASMExtensioner,
	grpcClient pbsubstreams.StreamClient,
	grpcCallOpts []grpc.CallOption,
	opts ...Option) *Pipeline {

	pipe := &Pipeline{
		context:                      ctx,
		request:                      request,
		builders:                     map[string]*state.Builder{},
		graph:                        graph,
		baseStateStore:               baseStateStore,
		manifest:                     request.Manifest,
		outputModuleNames:            request.OutputModules,
		outputModuleMap:              map[string]bool{},
		blockType:                    blockType,
		progressTracker:              newProgressTracker(),
		wasmExtensions:               wasmExtensions,
		grpcClient:                   grpcClient,
		grpcCallOpts:                 grpcCallOpts,
		outputCacheSaveBlockInterval: outputCacheSaveBlockInterval,
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

func (p *Pipeline) HandlerFactory(returnFunc substreams.ReturnFunc, progressFunc substreams.ProgressFunc) (bstream.Handler, error) {
	zlog.Info("initializing handler", zap.Uint64("requested_start_block", p.requestedStartBlockNum))
	ctx := p.context
	// WARN: we don't support < 0 StartBlock for now
	p.requestedStartBlockNum = uint64(p.request.StartBlockNum)
	p.moduleOutputCache = outputs.NewModuleOutputCache(p.outputCacheSaveBlockInterval)

	var err error
	zlog.Info("building store and executor")
	p.modules, _, err = p.build()
	if err != nil {
		return nil, fmt.Errorf("building pipeline: %w", err)
	}

	for _, module := range p.modules {
		isOutput := p.outputModuleMap[module.Name]
		if isOutput && p.requestedStartBlockNum < module.StartBlock {
			return nil, fmt.Errorf("invalid request: start block %d smaller that request outputs for module: %q start block %d", p.requestedStartBlockNum, module.Name, module.StartBlock)
		}

		hash := manifest.HashModuleAsString(p.manifest, p.graph, module)
		_, err := p.moduleOutputCache.RegisterModule(ctx, module, hash, p.baseStateStore, p.requestedStartBlockNum)
		if err != nil {
			return nil, fmt.Errorf("registering output cache for module %q: %w", module.Name, err)
		}

	}

	p.progressTracker.startTracking(ctx)

	if err = SynchronizeStores(ctx, p.grpcClient, p.grpcCallOpts, p.request, p.builders, p.moduleOutputCache.OutputCaches, p.requestedStartBlockNum, returnFunc); err != nil {
		return nil, fmt.Errorf("synchonizing stores: %w", err)
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
		if err := cache.Load(ctx, atBlock); err != nil {
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
		if err := p.saveStoresSnapshots(ctx); err != nil {
			return fmt.Errorf("saving stores: %w", err)
		}

		if p.clock.Number >= p.request.StopBlockNum && p.request.StopBlockNum != 0 {
			if err := p.moduleOutputCache.Save(ctx); err != nil {
				zlog.Error("saving mode")
			}
			return io.EOF
		}

		zlog.Debug("processing block", zap.Uint64("block_num", block.Number))

		cursor := obj.(bstream.Cursorable).Cursor()
		step := obj.(bstream.Stepable).Step()

		//if !skipBlockSource {
		if err = p.setupSource(block); err != nil {
			return fmt.Errorf("setting up sources: %w", err)
		}
		//}

		for _, executor := range p.moduleExecutors {
			zlog.Debug("executing", zap.Stringer("module_name", executor))
			err := executor.run(p.wasmOutputs, p.clock, block)
			if err != nil {
				if returnErr := p.returnFailureProgress(err, executor, progressFunc); returnErr != nil {
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

		p.progressTracker.blockProcessed(block, time.Since(handleStart))

		if p.clock.Number >= p.requestedStartBlockNum {
			if err := p.returnOutputs(step, cursor, returnFunc); err != nil {
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

func (p *Pipeline) returnOutputs(step bstream.StepType, cursor *bstream.Cursor, returnFunc substreams.ReturnFunc) error {
	var out *pbsubstreams.BlockScopedData
	if len(p.moduleOutputs) > 0 {
		zlog.Debug("got modules outputs", zap.Int("module_output_count", len(p.moduleOutputs)))
		out = &pbsubstreams.BlockScopedData{
			Outputs: p.moduleOutputs,
			Clock:   p.clock,
			Step:    pbsubstreams.StepToProto(step),
			Cursor:  cursor.ToOpaque(),
		}

	}

	var modules []*pbsubstreams.ModuleProgress
	for _, mod := range p.modules {
		modules = append(modules, &pbsubstreams.ModuleProgress{
			Name:            mod.Name,
			ProcessedRanges: []*pbsubstreams.BlockRange{{StartBlock: p.requestedStartBlockNum, EndBlock: p.progressTracker.lastBlock}},
		})
	}
	progress := &pbsubstreams.ModulesProgress{Modules: modules}

	if err := returnFunc(out, progress); err != nil {
		return fmt.Errorf("calling return func: %w", err)
	}
	return nil
}

func (p *Pipeline) returnFailureProgress(err error, failedExecutor ModuleExecutor, progressFunc substreams.ProgressFunc) error {
	modules := make([]*pbsubstreams.ModuleProgress, len(p.moduleOutputs)+1)

	for i, moduleOutput := range p.moduleOutputs {
		modules[i] = &pbsubstreams.ModuleProgress{
			Name: moduleOutput.Name,

			Failed: false,
			// It's a bit weird that for successful module, there is still FailureLogs, maybe we should revisit the semantic and
			// maybe change back to `Logs`.
			FailureLogs:          moduleOutput.Logs,
			FailureLogsTruncated: moduleOutput.LogsTruncated,

			// Where those comes from, should we have them populate on failure?
			ProcessedRanges:   nil,
			TotalBytesRead:    0,
			TotalBytesWritten: 0,
		}
	}

	logs, truncated := failedExecutor.moduleLogs()

	modules[len(p.moduleOutputs)] = &pbsubstreams.ModuleProgress{
		Name: failedExecutor.Name(),

		Failed: true,
		// Should we maybe extract specific WASM error and improved the "printing" here?
		FailureReason:        err.Error(),
		FailureLogs:          logs,
		FailureLogsTruncated: truncated,

		// Where those comes from, should we have them populate on failure?
		ProcessedRanges:   nil,
		TotalBytesRead:    0,
		TotalBytesWritten: 0,
	}

	return progressFunc(&pbsubstreams.ModulesProgress{Modules: modules})
}

func (p *Pipeline) setupSource(block *bstream.Block) error {
	blk := block.ToProtocol()

	switch p.vmType {
	case "native":
		panic("not implemented")
	case "wasm/rust-v1":
		// block.Payload.Get() could do the same, but does it go through the same
		// CORRECTIONS of the block, that the BlockDecoder does?
		blkBytes, err := proto.Marshal(blk.(proto.Message))
		if err != nil {
			return fmt.Errorf("packing block: %w", err)
		}

		clockBytes, err := proto.Marshal(p.clock)

		p.wasmOutputs[p.blockType] = blkBytes
		p.wasmOutputs["sf.substreams.v1.Clock"] = clockBytes
	default:
		panic("unsupported vmType " + p.vmType)
	}
	return nil
}

func (p *Pipeline) build() (modules []*pbsubstreams.Module, storeModules []*pbsubstreams.Module, err error) {
	for _, module := range p.manifest.Modules {
		vmType := ""
		switch module.Code.(type) {
		case *pbsubstreams.Module_WasmCode_:
			vmType = module.GetWasmCode().GetType()
		case *pbsubstreams.Module_NativeCode_:
			vmType = "native"
		default:
			return nil, nil, fmt.Errorf("invalid code type for modules %s ", module.Name)
		}

		if p.vmType != "" && vmType != p.vmType {
			return nil, nil, fmt.Errorf("cannot process modules of different code types: %s vs %s", p.vmType, vmType)
		}
		p.vmType = vmType
	}

	modules, err = p.graph.ModulesDownTo(p.outputModuleNames)
	if err != nil {
		return nil, nil, fmt.Errorf("building execution graph: %w", err)
	}

	p.builders = make(map[string]*state.Builder)
	storeModules, err = p.graph.StoresDownTo(p.outputModuleNames)
	for _, storeModule := range storeModules {
		var options []state.BuilderOption

		builder, err := state.NewBuilder(
			storeModule.Name,
			p.storesSaveInterval,
			storeModule.StartBlock,
			manifest.HashModuleAsString(p.manifest, p.graph, storeModule),
			storeModule.GetKindStore().UpdatePolicy,
			storeModule.GetKindStore().ValueType,
			p.baseStateStore,
			options...,
		)
		if err != nil {
			return nil, nil, fmt.Errorf("creating builder %s: %w", storeModule.Name, err)
		}

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
		wasmCodeRef := module.GetWasmCode()
		if wasmCodeRef == nil {
			return fmt.Errorf("build_wasm cannot use modules that are not of type wasm")
		}
		entrypoint := wasmCodeRef.Entrypoint

		code := p.manifest.ModulesCode[wasmCodeRef.Index]
		wasmModule, err := p.wasmRuntime.NewModule(ctx, request, code, module.Name)
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
	grpcClient pbsubstreams.StreamClient,
	grpcCallOpts []grpc.CallOption,
	request *pbsubstreams.Request,
	builders map[string]*state.Builder,
	outputCache map[string]*outputs.OutputCache,
	upToBlockNum uint64,
	returnFunc substreams.ReturnFunc,
) error {
	zlog.Info("synchronizing stores")

	squasher, err := squasher.NewSquasher(ctx, builders, outputCache)
	if err != nil {
		return fmt.Errorf("initializing squasher: %w", err)
	}

	s, err := scheduler.NewScheduler(ctx, request, builders, upToBlockNum, squasher)
	if err != nil {
		return fmt.Errorf("initializing scheduler: %w", err)
	}

	const numJobs = 5 // todo: get from parameter from firehose
	jobs := make(chan *job)

	wg := &sync.WaitGroup{}
	wg.Add(numJobs)

	go func() {
		for w := 0; w < numJobs; w++ {
			worker(ctx, grpcClient, grpcCallOpts, returnFunc, jobs)
			wg.Done()
		}
	}()

	for {
		err := s.Next(func(request *pbsubstreams.Request, callback func(r *pbsubstreams.Request, err error)) {
			jobs <- &job{
				request:  request,
				callback: callback,
			}
		})

		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
	}

	close(jobs)
	wg.Wait()

	return nil
}

type job struct {
	request  *pbsubstreams.Request
	callback func(r *pbsubstreams.Request, err error)
}

func worker(ctx context.Context, grpcClient pbsubstreams.StreamClient, grpcCallOpts []grpc.CallOption, returnFunc substreams.ReturnFunc, jobs <-chan *job) {
	for {
		select {
		case j, ok := <-jobs:
			if !ok {
				return
			}
			stream, err := grpcClient.Blocks(ctx, j.request, grpcCallOpts...)
			if err != nil {
				j.callback(j.request, fmt.Errorf("call sf.substreams.v1.Stream/Blocks: %w", err))
			}

			for {
				zlog.Debug("waiting for stream response...")
				resp, err := stream.Recv()

				if err != nil {
					if err == io.EOF {
						j.callback(j.request, nil)
						return
					}
					j.callback(j.request, err)
				}

				switch r := resp.Message.(type) {
				case *pbsubstreams.Response_Progress:
					zlog.Debug("resp received", zap.String("type", "progress"))
					//todo: forward progress to end user
					err := returnFunc(nil, r.Progress)
					if err != nil {
						j.callback(j.request, err)
					}
				case *pbsubstreams.Response_SnapshotData:
					_ = r.SnapshotData
				case *pbsubstreams.Response_SnapshotComplete:
					_ = r.SnapshotComplete
				case *pbsubstreams.Response_Data:
					zlog.Debug("resp received", zap.String("type", "data"))
					for _, output := range r.Data.Outputs {
						for _, log := range output.Logs {
							fmt.Println("LOG: ", log)
							//todo: maybe return log ...
						}
					}
				}
			}
		case <-ctx.Done():
			return
		}
	}
}

func (p *Pipeline) saveStoresSnapshots(ctx context.Context) error {
	isFirstRequestBlock := p.requestedStartBlockNum == p.clock.Number
	intervalReached := p.storesSaveInterval != 0 && p.clock.Number%p.storesSaveInterval == 0

	zlog.Debug("maybe saving stores snapshots",
		zap.Uint64("req_start_block", p.requestedStartBlockNum),
		zap.Uint64("block_num", p.clock.Number),
		zap.Bool("is_first_request_block", isFirstRequestBlock),
		zap.Bool("reach_save_interval", intervalReached),
	)

	if !isFirstRequestBlock && intervalReached {
		for _, builder := range p.builders {
			err := builder.WriteState(ctx)
			if err != nil {
				return fmt.Errorf("writing store '%s' state: %w", builder.Name, err)
			}

			//var nextBlockRangeStart *uint64
			if p.partialMode {
				//nextBlockRangeStart = uint64Pointer(builder.BlockRange.ExclusiveEndBlock)
				builder.RollPartial()
				continue
			}
			builder.Roll()
			//nextBlockRangeEnd := uint64Pointer(builder.BlockRange.ExclusiveEndBlock + p.storesSaveInterval)
			//
			//builder.UpdateBlockRange(nextBlockRangeStart, nextBlockRangeEnd)

			zlog.Info("state written", zap.String("store_name", builder.Name))
		}
	}
	return nil
}

func (p *Pipeline) InitializeStores(ctx context.Context) error {
	var initFunc func(builder *state.Builder) error
	if p.partialMode {
		initFunc = func(builder *state.Builder) error {
			return builder.InitializePartial(ctx, p.requestedStartBlockNum)
		}
	} else {
		initFunc = func(builder *state.Builder) error {
			outputCache := p.moduleOutputCache.OutputCaches[builder.Name]
			return builder.Initialize(ctx, p.requestedStartBlockNum, p.moduleOutputCache.SaveBlockInterval, outputCache.Store)
		}
	}

	for _, builder := range p.builders {
		err := initFunc(builder)
		if err != nil {
			return fmt.Errorf("reading state for builder %q: %w", builder.Name, err)
		}
	}
	return nil
}

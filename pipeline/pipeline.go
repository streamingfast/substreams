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
	"github.com/streamingfast/substreams"
	"github.com/streamingfast/substreams/block"
	"github.com/streamingfast/substreams/manifest"
	"github.com/streamingfast/substreams/orchestrator"
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

	requestedStartBlockNum uint64 // rename to: requestStartBlock, SET UPON receipt of the request
	maxStoreSyncRangeSize  uint64
	isBackprocessing       bool

	preBlockHooks  []substreams.BlockHook
	postBlockHooks []substreams.BlockHook
	postJobHooks   []substreams.PostJobHook

	wasmRuntime    *wasm.Runtime
	wasmExtensions []wasm.WASMExtensioner

	context context.Context
	request *pbsubstreams.Request
	graph   *manifest.ModuleGraph

	modules              []*pbsubstreams.Module
	outputModuleNames    []string
	outputModuleMap      map[string]bool
	storeModules         []*pbsubstreams.Module
	storeMap             map[string]*state.Store
	backprocessingStores []*state.Store

	moduleExecutors []ModuleExecutor
	wasmOutputs     map[string][]byte

	baseStateStore     dstore.Store
	storesSaveInterval uint64

	clock         *pbsubstreams.Clock
	moduleOutputs []*pbsubstreams.ModuleOutput
	logs          []string

	moduleOutputCache *outputs.ModulesOutputCache

	partialsWritten block.Ranges // when backprocessing, to report back to orchestrator

	currentBlockRef bstream.BlockRef

	outputCacheSaveBlockInterval uint64
	blockRangeSizeSubRequests    int
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
		context: ctx,
		request: request,
		// WARN: we don't support < 0 StartBlock for now
		requestedStartBlockNum:       uint64(request.StartBlockNum),
		storeMap:                     map[string]*state.Store{},
		graph:                        graph,
		baseStateStore:               baseStateStore,
		outputModuleNames:            request.OutputModules,
		outputModuleMap:              map[string]bool{},
		blockType:                    blockType,
		wasmExtensions:               wasmExtensions,
		grpcClientFactory:            grpcClientFactory,
		outputCacheSaveBlockInterval: outputCacheSaveBlockInterval,
		blockRangeSizeSubRequests:    blockRangeSizeSubRequests,

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

func (p *Pipeline) IsOutputModule(name string) bool {
	_, found := p.outputModuleMap[name]
	return found
}

func (p *Pipeline) HandlerFactory(workerPool *orchestrator.WorkerPool, respFunc func(resp *pbsubstreams.Response) error) (out bstream.Handler, err error) {
	ctx := p.context
	zlog.Info("initializing handler", zap.Uint64("requested_start_block", p.requestedStartBlockNum), zap.Uint64("requested_stop_block", p.request.StopBlockNum), zap.Bool("is_backprocessing", p.isBackprocessing), zap.Strings("outputs", p.request.OutputModules))

	p.moduleOutputCache = outputs.NewModuleOutputCache(p.outputCacheSaveBlockInterval)

	if err := p.build(); err != nil {
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

	// Fetch the stores

	if p.isBackprocessing {
		storeMap, err := p.buildStoreMap()
		if err != nil {
			return nil, fmt.Errorf("building store map: %w", err)
		}

		totalOutputModules := len(p.outputModuleNames)
		outputName := p.outputModuleNames[0]
		backProcessingStore := storeMap[outputName]
		lastStoreName := p.storeModules[len(p.storeModules)-1].Name
		isLastStore := lastStoreName == backProcessingStore.Name

		if totalOutputModules == 1 && backProcessingStore != nil && isLastStore {
			// totalOutputModels is a temporary restrictions, for when the orchestrator
			// will be able to run two leaf stores from the same job
			zlog.Info("marking leaf store for partial processing", zap.String("module", outputName))
			r := block.NewRange(p.requestedStartBlockNum, p.requestedStartBlockNum+backProcessingStore.SaveInterval)
			backProcessingStore.BlockRange = r //todo: smell like s...
			p.backprocessingStores = append(p.backprocessingStores, backProcessingStore)
		} else {
			zlog.Info("conditions for leaf store not met",
				zap.String("module", outputName),
				zap.Bool("is_last_store", isLastStore),
				zap.Int("output_module_count", totalOutputModules))
		}

		zlog.Info("initializing and loading stores")
		if err = loadStores(ctx, storeMap, p.requestedStartBlockNum); err != nil {
			return nil, fmt.Errorf("loading stores: %w", err)
		}

		p.storeMap = storeMap
	} else {
		newStores, err := p.backprocessStores(ctx, workerPool, respFunc)
		if err != nil {
			return nil, fmt.Errorf("synchronizing stores: %w", err)
		}

		p.storeMap = newStores
		p.backprocessingStores = nil

		if len(p.request.InitialStoreSnapshotForModules) != 0 {
			zlog.Info("sending snapshot", zap.Strings("modules", p.request.InitialStoreSnapshotForModules))
			if err := p.sendSnapshots(p.request.InitialStoreSnapshotForModules, respFunc); err != nil {
				return nil, fmt.Errorf("send initial snapshots: %w", err)
			}
		}
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

		// TODO(abourget): eventually, handle the `undo` signals.

		p.clock = &pbsubstreams.Clock{
			Number:    block.Num(),
			Id:        block.Id,
			Timestamp: timestamppb.New(block.Time()),
		}

		p.currentBlockRef = block.AsRef()

		if err = p.moduleOutputCache.Update(ctx, p.currentBlockRef); err != nil {
			return fmt.Errorf("updating module output cache: %w", err)
		}

		for _, hook := range p.preBlockHooks {
			if err := hook(ctx, p.clock); err != nil {
				return fmt.Errorf("pre block hook: %w", err)
			}
		}

		p.moduleOutputs = nil
		p.wasmOutputs = map[string][]byte{}

		//todo? should we only save store if in partial mode or in catchup?
		// no need to save store if loaded from cache?
		isFirstRequestBlock := p.requestedStartBlockNum == p.clock.Number
		intervalReached := p.storesSaveInterval != 0 && p.clock.Number%p.storesSaveInterval == 0
		isTemporaryStore := p.isBackprocessing && p.request.StopBlockNum != 0 && p.clock.Number == p.request.StopBlockNum

		if !isFirstRequestBlock && (intervalReached || isTemporaryStore) {
			if err := p.saveStoresSnapshots(ctx); err != nil {
				return fmt.Errorf("saving stores: %w", err)
			}
		}

		if p.clock.Number >= p.request.StopBlockNum && p.request.StopBlockNum != 0 {
			// FIXME: HERE WE KNOW THAT we've gone OVER the ExclusiveEnd boundary,
			// and we will trigger this EVEN if we have chains that SKIP BLOCKS.

			if p.isBackprocessing {
				// TODO: why wouldn't we do that when we're live?! Why only when orchestrated?
				zlog.Debug("about to save cache output", zap.Uint64("clock", p.clock.Number), zap.Uint64("stop_block", p.request.StopBlockNum))
				if err := p.moduleOutputCache.Save(ctx); err != nil {
					return fmt.Errorf("saving partial caches")
				}
			}
			return io.EOF
		}

		zlog.Debug("processing block", zap.Uint64("block_num", block.Number))

		cursor := obj.(bstream.Cursorable).Cursor()
		step := obj.(bstream.Stepable).Step()

		if err = p.assignSource(block); err != nil {
			return fmt.Errorf("setting up sources: %w", err)
		}

		for _, executor := range p.moduleExecutors {
			executorName := executor.Name()
			zlog.Debug("executing", zap.String("module_name", executorName))

			executionError := executor.run(p.wasmOutputs, p.clock, block)

			if isOutput := p.outputModuleMap[executorName]; isOutput {
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
			}

			if executionError != nil {
				if returnErr := p.returnFailureProgress(executionError, executor, respFunc); returnErr != nil {
					return fmt.Errorf("progress error: %w", returnErr)
				}
				return fmt.Errorf("exec error: %w", executionError)
			}

			executor.Reset()
		}

		if p.clock.Number >= p.requestedStartBlockNum {
			if err := p.returnOutputs(step, cursor, respFunc); err != nil {
				return err
			}
		}

		for _, s := range p.storeMap {
			s.Flush()
		}

		zlog.Debug("block processed", zap.Uint64("block_num", block.Number))
		return nil
	}), nil
}

func (p *Pipeline) returnOutputs(step bstream.StepType, cursor *bstream.Cursor, respFunc substreams.ResponseFunc) error {
	if p.isBackprocessing {
		// TODO(abourget): we might want to send progress for the segment after batch execution
		var progress []*pbsubstreams.ModuleProgress

		for _, store := range p.backprocessingStores {
			progress = append(progress, &pbsubstreams.ModuleProgress{
				Name: store.Name,
				Type: &pbsubstreams.ModuleProgress_ProcessedRanges{
					ProcessedRanges: &pbsubstreams.ModuleProgress_ProcessedRange{
						ProcessedRanges: []*pbsubstreams.BlockRange{
							{
								StartBlock: store.BlockRange.StartBlock,
								EndBlock:   p.clock.Number,
							},
						},
					},
				},
			})
		}

		if err := respFunc(substreams.NewModulesProgressResponse(progress)); err != nil {
			return fmt.Errorf("calling return func: %w", err)
		}
	} else {
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
	return nil
}

func (p *Pipeline) returnFailureProgress(err error, failedExecutor ModuleExecutor, respFunc substreams.ResponseFunc) error {
	var out []*pbsubstreams.ModuleProgress

	for _, moduleOutput := range p.moduleOutputs {
		var reason string
		if moduleOutput.Name == failedExecutor.Name() {
			reason = err.Error()
		}

		// FIXME(abourget): eventually, would we also return the data for each of
		// the modules that produced some?
		if len(moduleOutput.Logs) != 0 || len(reason) != 0 {
			out = append(out, &pbsubstreams.ModuleProgress{
				Name: moduleOutput.Name,
				Type: &pbsubstreams.ModuleProgress_Failed_{
					Failed: &pbsubstreams.ModuleProgress_Failed{
						Reason:        reason,
						Logs:          moduleOutput.Logs,
						LogsTruncated: moduleOutput.LogsTruncated,
					},
				},
			})
		}
	}

	return respFunc(substreams.NewModulesProgressResponse(out))
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
	modules, err := p.graph.ModulesDownTo(p.outputModuleNames)
	if err != nil {
		return fmt.Errorf("building execution graph: %w", err)
	}
	p.modules = modules

	storeModules, err := p.graph.StoresDownTo(p.outputModuleNames)
	if err != nil {
		return err
	}
	p.storeModules = storeModules

	return nil
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

func (p *Pipeline) saveStoresSnapshots(ctx context.Context) error {
	// FIXME: lastBlock NEEDS to BE ALIGNED on boundaries!! The caller or this function
	// should handle this, with a previous boundary that was passed, etc.. to support
	// chains that skip blocks.
	for _, builder := range p.storeMap {
		// TODO: implement parallel writing and upload for the different stores involved.
		err := builder.WriteState(ctx)
		if err != nil {
			return fmt.Errorf("writing store '%s' state: %w", builder.Name, err)
		}

		if builder.IsPartial() {
			if p.IsOutputModule(builder.Name) {
				r := block.NewRange(builder.BlockRange.StartBlock, builder.BlockRange.ExclusiveEndBlock)
				p.partialsWritten = append(p.partialsWritten, r)
				zlog.Debug("adding partials written", zap.Object("range", builder.BlockRange), zap.Stringer("ranges", p.partialsWritten))
			}
			builder.Truncate()
		}
		builder.Roll(p.isBackprocessing)

		zlog.Info("state written", zap.String("store_name", builder.Name))
	}

	return nil
}

func (p *Pipeline) buildStoreMap() (storeMap map[string]*state.Store, err error) {
	storeMap = map[string]*state.Store{}
	for _, storeModule := range p.storeModules {
		newStore, err := state.NewBuilder(
			storeModule.Name,
			p.storesSaveInterval,
			storeModule.InitialBlock,
			manifest.HashModuleAsString(p.request.Modules, p.graph, storeModule),
			storeModule.GetKindStore().UpdatePolicy,
			storeModule.GetKindStore().ValueType,
			p.baseStateStore,
		)
		if err != nil {
			return nil, fmt.Errorf("creating builder %s: %w", storeModule.Name, err)
		}
		storeMap[newStore.Name] = newStore
	}
	return storeMap, nil
}

func loadStores(ctx context.Context, storeMap map[string]*state.Store, requestedStartBlock uint64) error {
	for _, store := range storeMap {
		if store.IsPartial() {
			continue
		}
		if store.BlockRange.StartBlock == requestedStartBlock {
			continue
		}

		err := store.LoadState(ctx)
		if err != nil {
			return fmt.Errorf("reading state for builder %q: %w", store.Name, err)
		}
	}
	return nil
}

func (p *Pipeline) PartialsWritten() block.Ranges {
	return p.partialsWritten
}

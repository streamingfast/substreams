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

	context  context.Context
	request  *pbsubstreams.Request
	graph    *manifest.ModuleGraph
	respFunc func(resp *pbsubstreams.Response) error

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

	moduleOutputCache *outputs.ModulesOutputCache

	partialsWritten block.Ranges // when backprocessing, to report back to orchestrator

	currentBlockRef bstream.BlockRef

	outputCacheSaveBlockInterval uint64
	subrequestSplitSize          int
	grpcClientFactory            substreams.GrpcClientFactory

	cacheEnabled       bool
	partialModeEnabled bool
}

func New(
	ctx context.Context,
	request *pbsubstreams.Request,
	graph *manifest.ModuleGraph,
	blockType string,
	baseStateStore dstore.Store,
	outputCacheSaveBlockInterval uint64,
	wasmExtensions []wasm.WASMExtensioner,
	grpcClientFactory substreams.GrpcClientFactory,
	subrequestSplitSize int,
	respFunc func(resp *pbsubstreams.Response) error,
	opts ...Option) *Pipeline {

	pipe := &Pipeline{
		context: ctx,
		request: request,
		// WARN: we don't support < 0 StartBlock for now
		requestedStartBlockNum:       uint64(request.StartBlockNum),
		storeMap:                     map[string]*state.Store{},
		graph:                        graph,
		baseStateStore:               baseStateStore,
		outputModuleMap:              map[string]bool{},
		blockType:                    blockType,
		wasmExtensions:               wasmExtensions,
		grpcClientFactory:            grpcClientFactory,
		outputCacheSaveBlockInterval: outputCacheSaveBlockInterval,
		subrequestSplitSize:          subrequestSplitSize,
		maxStoreSyncRangeSize:        math.MaxUint64,
		respFunc:                     respFunc,
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

func (p *Pipeline) Init(workerPool *orchestrator.WorkerPool) (err error) {
	ctx := p.context

	zlog.Info("initializing handler", zap.Uint64("requested_start_block", p.requestedStartBlockNum), zap.Uint64("requested_stop_block", p.request.StopBlockNum), zap.Bool("is_backprocessing", p.isSubrequest), zap.Strings("outputs", p.request.OutputModules))

	p.moduleOutputCache = outputs.NewModuleOutputCache(p.outputCacheSaveBlockInterval)

	if err := p.build(); err != nil {
		return fmt.Errorf("building pipeline: %w", err)
	}

	if p.cacheEnabled || p.partialModeEnabled { // always load/save/update cache when you are in partialMode
		for _, module := range p.modules {
			isOutput := p.outputModuleMap[module.Name]

			if isOutput && p.requestedStartBlockNum < module.InitialBlock {
				return fmt.Errorf("invalid request: start block %d smaller that request outputs for module: %q start block %d", p.requestedStartBlockNum, module.Name, module.InitialBlock)
			}

			hash := manifest.HashModuleAsString(p.request.Modules, p.graph, module)
			_, err := p.moduleOutputCache.RegisterModule(module, hash, p.baseStateStore)
			if err != nil {
				return fmt.Errorf("registering output cache for module %q: %w", module.Name, err)
			}
		}
	}

	zlog.Info("initializing and loading stores")
	initialStoreMap, err := p.buildStoreMap()
	zlog.Info("stores load", zap.Int("number_of_stores", len(initialStoreMap)))
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
			zlog.Warn("conditions for leaf store not met",
				zap.String("module", outputName),
				zap.Bool("is_last_store", isLastStore),
				zap.Int("output_module_count", totalOutputModules))
			return fmt.Errorf("invalid conditions to backprocess leaf store %q", outputName)
		}

		zlog.Info("marking leaf store for partial processing", zap.String("module", outputName))

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
			zlog.Info("sending snapshot", zap.Strings("modules", p.request.InitialStoreSnapshotForModules))
			if err := p.sendSnapshots(p.request.InitialStoreSnapshotForModules); err != nil {
				return fmt.Errorf("send initial snapshots: %w", err)
			}
		}

		p.partialModeEnabled = false
	}

	p.initStoreSaveBoundary()

	err = p.buildWASM(ctx, p.request, p.modules)
	if err != nil {
		return fmt.Errorf("initiating module output caches: %w", err)
	}

	if p.cacheEnabled || p.partialModeEnabled { // always load cache when you are in partialMode
		for _, cache := range p.moduleOutputCache.OutputCaches {
			atBlock := outputs.ComputeStartBlock(p.requestedStartBlockNum, p.outputCacheSaveBlockInterval)
			if _, err := cache.LoadAtBlock(ctx, atBlock); err != nil {
				return fmt.Errorf("loading outputs caches")
			}
		}
	}

	return nil
}

func (p *Pipeline) initStoreSaveBoundary() {
	p.nextStoreSaveBoundary = p.computeNextStoreSaveBoundary(p.requestedStartBlockNum)
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
	ctx := p.context
	zlog.Debug("processing block", zap.Uint64("block_num", block.Number))
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

	blockNum := block.Num()
	p.clock = &pbsubstreams.Clock{
		Number:    blockNum,
		Id:        block.Id,
		Timestamp: timestamppb.New(block.Time()),
	}
	p.currentBlockRef = block.AsRef()

	// if obj.Step() == UNDO {
	//  loop ALL the STORES that we have in `map[obj.BlockID]outputs`, and Apply the in REVERSE
	//  PUSH OUT the outputs as a step UNDO without executing, until we are back to the
	//  fork point.
	//  return nil
	// }
	// if obj.Step() == NEW && map[blockID] != nil {
	//   SEnd all the cached stuff from map[obj.BlockID]outputs
	//   return nil
	// }
	// TODO(abourget): eventually, handle the `undo` signals.
	// if obj.Step() == IRREVERSIBLE  || STALLED {
	//    if obj.Step() == IRREVESRIBLE { p.moduleOutputCache.Update(ctx, map[blockID]) }
	//    delete(map[blockID], outputs)
	// }

	if p.cacheEnabled || p.partialModeEnabled { // always load/save/update cache when you are in partialMode
		if err = p.moduleOutputCache.Update(ctx, p.currentBlockRef); err != nil {
			return fmt.Errorf("updating module output cache: %w", err)
		}
	}

	for _, hook := range p.preBlockHooks {
		if err := hook(ctx, p.clock); err != nil {
			return fmt.Errorf("pre block hook: %w", err)
		}
	}

	// NOTE: the tests for this code test on a COPY of these lines: (TestBump)
	for p.nextStoreSaveBoundary <= blockNum {
		if err := p.saveStoresSnapshots(ctx, p.nextStoreSaveBoundary); err != nil {
			return fmt.Errorf("saving stores: %w", err)
		}
		p.bumpStoreSaveBoundary()
		if isStopBlockReached(blockNum, p.request.StopBlockNum) {
			break
		}
	}

	if isStopBlockReached(blockNum, p.request.StopBlockNum) {
		if p.cacheEnabled || p.partialModeEnabled { // always load/save/update cache when you are in partialMode
			zlog.Debug("about to save cache output", zap.Uint64("clock", blockNum), zap.Uint64("stop_block", p.request.StopBlockNum))
			if err := p.moduleOutputCache.Flush(ctx); err != nil {
				return fmt.Errorf("saving partial caches")
			}
			return io.EOF
		}
	}

	cursor := obj.(bstream.Cursorable).Cursor()
	step := obj.(bstream.Stepable).Step()

	if err = p.assignSource(block); err != nil {
		return fmt.Errorf("setting up sources: %w", err)
	}

	for _, executor := range p.moduleExecutors {
		err = p.runExecutor(ctx, executor, cursor.ToOpaque())
		if err != nil {
			if returnErr := p.returnFailureProgress(err, executor); returnErr != nil {
				return fmt.Errorf("progress error: %w", returnErr)
			}

			return io.EOF
		}
	}

	// Snapshot all outputs, in case we undo
	// map[block_id]outputs

	if shouldReturnProgress(p.isSubrequest) {
		if err := p.returnModuleProgressOutputs(); err != nil {
			return err
		}
	}
	if shouldReturnDataOutputs(blockNum, p.requestedStartBlockNum, p.isSubrequest) {
		if err := p.returnModuleDataOutputs(step, cursor); err != nil {
			return err
		}
	}

	for _, s := range p.storeMap {
		s.Flush()
	}

	p.moduleOutputs = nil
	p.wasmOutputs = map[string][]byte{}

	zlog.Debug("block processed", zap.Uint64("block_num", block.Number))
	return nil
}

func (p *Pipeline) runExecutor(ctx context.Context, executor ModuleExecutor, cursor string) error {
	//FIXME(abourget): should we ever skip that work?
	// if executor.ModuleInitialBlock < block.Number {
	// 	continue ??
	// }
	executorName := executor.Name()
	zlog.Debug("executing", zap.String("module_name", executorName))

	err := executor.run(ctx, p.wasmOutputs, p.clock, p.cacheEnabled, p.partialModeEnabled, cursor)
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
			p.moduleOutputs = append(p.moduleOutputs, &pbsubstreams.ModuleOutput{
				Name:          executorName,
				Data:          outputData,
				Logs:          logs,
				LogsTruncated: truncated,
			})
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

func (p *Pipeline) returnModuleDataOutputs(step bstream.StepType, cursor *bstream.Cursor) error {
	zlog.Debug("got modules outputs", zap.Int("module_output_count", len(p.moduleOutputs)))
	out := &pbsubstreams.BlockScopedData{
		Outputs: p.moduleOutputs,
		Clock:   p.clock,
		Step:    pbsubstreams.StepToProto(step),
		Cursor:  cursor.ToOpaque(),
	}

	if err := p.respFunc(substreams.NewBlockScopedDataResponse(out)); err != nil {
		return fmt.Errorf("calling return func: %w", err)
	}

	return nil
}

func (p *Pipeline) returnFailureProgress(err error, failedExecutor ModuleExecutor) error {
	var out []*pbsubstreams.ModuleProgress

	for _, moduleOutput := range p.moduleOutputs {
		var reason string
		if moduleOutput.Name == failedExecutor.Name() {
			reason = err.Error()
		}

		//FIXME(abourget): eventually, would we also return the data for each of
		// the modules that produced some?
		if len(moduleOutput.Logs) != 0 || len(reason) != 0 {
			out = append(out, &pbsubstreams.ModuleProgress{
				Name: moduleOutput.Name,
				Type: &pbsubstreams.ModuleProgress_Failed_{
					Failed: &pbsubstreams.ModuleProgress_Failed{
						Reason:        reason,
						Logs:          failedExecutor.getCurrentExecutionStack(),
						LogsTruncated: moduleOutput.LogsTruncated,
					},
				},
			})
		}
	}

	zlog.Debug("return failed progress", zap.Int("progress_len", len(out)))
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
			}

			if p.cacheEnabled || p.partialModeEnabled { // always load/save/update cache when you are in partialMode
				baseExecutor.cache = p.moduleOutputCache.OutputCaches[module.Name]
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
			}

			if p.cacheEnabled || p.partialModeEnabled { // always load/save/update cache when you are in partialMode
				baseExecutor.cache = p.moduleOutputCache.OutputCaches[module.Name]
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

func (p *Pipeline) saveStoresSnapshots(ctx context.Context, boundaryBlock uint64) error {
	for _, builder := range p.storeMap {
		stateWriter, err := builder.WriteState(ctx, boundaryBlock)
		if err != nil {
			return fmt.Errorf("store writer %q: %w", builder.Name, err)
		}
		if err := stateWriter.Write(); err != nil {
			return fmt.Errorf("writing store '%s' state: %w", builder.Name, err)
		}
		zlog.Info("store written", zap.String("store_name", builder.Name), zap.Object("store", builder))

		if p.isSubrequest && p.isOutputModule(builder.Name) {
			r := block.NewRange(builder.StoreInitialBlock(), boundaryBlock)
			p.partialsWritten = append(p.partialsWritten, r)
			zlog.Debug("adding partials written", zap.Object("range", r), zap.Stringer("ranges", p.partialsWritten), zap.Uint64("boundary_block", boundaryBlock))
			builder.Roll(boundaryBlock)
		}
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

func (p *Pipeline) PartialsWritten() block.Ranges {
	return p.partialsWritten
}

package pipeline

import (
	"context"
	"fmt"
	"io"
	"runtime/debug"
	"strings"

	"github.com/abourget/llerrgroup"
	"github.com/streamingfast/bstream"
	"github.com/streamingfast/dstore"
	"github.com/streamingfast/substreams"
	"github.com/streamingfast/substreams/block"
	"github.com/streamingfast/substreams/manifest"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/state"
	"github.com/streamingfast/substreams/wasm"
	"go.uber.org/zap"
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

	moduleExecutors []ModuleExecutor
	wasmOutputs     map[string][]byte

	progressTracker    *progressTracker
	allowInvalidState  bool
	baseStateStore     dstore.Store
	storesSaveInterval uint64

	clock         *pbsubstreams.Clock
	moduleOutputs []*pbsubstreams.ModuleOutput
	logs          []string

	moduleOutputCache *ModulesOutputCache

	blocksFunc      func(ctx context.Context, r *pbsubstreams.Request) error
	currentBlockRef bstream.BlockRef
}

func New(
	ctx context.Context,
	request *pbsubstreams.Request,
	graph *manifest.ModuleGraph,
	blockType string,
	baseStateStore dstore.Store,
	wasmExtensions []wasm.WASMExtensioner,
	blocksFunc func(ctx context.Context, r *pbsubstreams.Request) error,
	opts ...Option) *Pipeline {

	pipe := &Pipeline{
		context:           ctx,
		request:           request,
		builders:          map[string]*state.Builder{},
		graph:             graph,
		baseStateStore:    baseStateStore,
		manifest:          request.Manifest,
		outputModuleNames: request.OutputModules,
		outputModuleMap:   map[string]bool{},
		blockType:         blockType,
		progressTracker:   newProgressTracker(),
		wasmExtensions:    wasmExtensions,
		blocksFunc:        blocksFunc,
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
	ctx := p.context
	// WARN: we don't support < 0 StartBlock for now
	requestedStartBlockNum := uint64(p.request.StartBlockNum)
	p.requestedStartBlockNum = uint64(p.request.StartBlockNum)
	p.moduleOutputCache = NewModuleOutputCache()
	modules, _, err := p.build(ctx, p.request)
	if err != nil {
		return nil, fmt.Errorf("building pipeline: %w", err)
	}

	for _, module := range modules {
		isOutput := p.outputModuleMap[module.Name]
		if isOutput && requestedStartBlockNum < module.StartBlock {
			return nil, fmt.Errorf("invalid request: start block %d smaller that request outputs for module: %q start block %d", requestedStartBlockNum, module.Name, module.StartBlock)
		}
	}

	p.progressTracker.startTracking(ctx)

	err = p.SynchronizeStores(ctx)
	if err != nil {
		return nil, fmt.Errorf("synchonizing store: %w", err)
	}

	if err != nil {
		return nil, fmt.Errorf("initiatin module output caches: %w", err)
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

		p.clock = &pbsubstreams.Clock{
			Number:    block.Num(),
			Id:        block.Id,
			Timestamp: timestamppb.New(block.Time()),
		}

		p.currentBlockRef = block.AsRef()

		if err = p.moduleOutputCache.update(ctx, p.currentBlockRef); err != nil {
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
	if len(p.moduleOutputs) > 0 {
		zlog.Debug("got modules outputs", zap.Int("module_output_count", len(p.moduleOutputs)))
		out := &pbsubstreams.BlockScopedData{
			Outputs: p.moduleOutputs,
			Clock:   p.clock,
			Step:    pbsubstreams.StepToProto(step),
			Cursor:  cursor.ToOpaque(),
		}
		if err := returnFunc(out); err != nil {
			return fmt.Errorf("calling return func: %w", err)
		}
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

func (p *Pipeline) build(ctx context.Context, request *pbsubstreams.Request) (modules []*pbsubstreams.Module, storeModules []*pbsubstreams.Module, err error) {
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
		if p.partialMode {
			options = append(options, state.WithPartialMode(p.requestedStartBlockNum))
		}

		builder, err := state.NewBuilder(
			ctx,
			storeModule.Name,
			storeModule.StartBlock,
			p.storesSaveInterval,
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

	if p.vmType == "native" {
		return nil, nil, fmt.Errorf("native schtuff not supported yet")
	}
	err = p.buildWASM(ctx, request, modules)
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

		hash := manifest.HashModuleAsString(p.manifest, p.graph, module)
		cache, err := p.moduleOutputCache.registerModule(ctx, module, hash, p.baseStateStore, p.requestedStartBlockNum)
		if err != nil {
			return fmt.Errorf("registering output cache for module %q: %w", module.Name, err)
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
					cache:      cache,
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
					cache:      cache,
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

func (p *Pipeline) synchronizeBlocks(ctx context.Context) error {
	type syncItem struct {
		Builder *state.Builder
		Range   *block.Range
	}
	var syncItems []*syncItem

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	alreadyAdded := map[string]bool{}
	computedStartBlock := p.requestedStartBlockNum - (p.requestedStartBlockNum % p.storesSaveInterval)
	for _, store := range p.builders {
		if computedStartBlock <= store.ModuleStartBlock {
			continue
		}

		if _, ok := alreadyAdded[store.Name]; ok {
			continue
		}

		info := store.Info()
		if info.LastKVSavedBlock < computedStartBlock {
			storesInTree, err := p.graph.AncestorStoresOf(store.Name)
			if err != nil {
				return fmt.Errorf("getting stores down to %s: %w", store.Name, err)
			}

			for _, s := range storesInTree {
				alreadyAdded[s.Name] = true
			}

			info := store.Info()
			brs := (&block.Range{
				StartBlock:        info.LastKVSavedBlock,
				ExclusiveEndBlock: computedStartBlock,
			}).Split(p.storesSaveInterval)

			for _, br := range brs {
				syncItems = append(syncItems, &syncItem{
					Builder: store,
					Range:   br,
				})
			}

		}
	}

	eg := llerrgroup.New(len(syncItems))
	for _, item := range syncItems {
		if eg.Stop() {
			continue
		}

		store := item.Builder
		blockRange := item.Range
		eg.Go(func() error {
			startBlock := blockRange.StartBlock
			endBlock := blockRange.ExclusiveEndBlock

			zlog.Info("waiting for request for store to finish",
				zap.String("store", store.Name),
				zap.Uint64("start_block", startBlock),
				zap.Uint64("end_block", endBlock),
			)

			request := &pbsubstreams.Request{
				StartBlockNum:                  int64(startBlock),
				StopBlockNum:                   endBlock,
				ForkSteps:                      p.request.ForkSteps,
				IrreversibilityCondition:       p.request.IrreversibilityCondition,
				Manifest:                       p.manifest,
				OutputModules:                  []string{store.Name},
				InitialStoreSnapshotForModules: p.request.InitialStoreSnapshotForModules,
			}

			err := p.blocksFunc(ctx, request)
			if err != nil {
				cancel()
				return fmt.Errorf("getting blocks for store %s (%d, %d): %w", store.Name, startBlock, endBlock, err)
			}
			return nil
		})
	}

	err := eg.Wait()
	if err != nil {
		return fmt.Errorf("catching up blocks for stores: %w", err)
	}

	return nil
}

func (p *Pipeline) SynchronizeStores(ctx context.Context) error {
	err := p.synchronizeBlocks(ctx)
	if err != nil {
		return fmt.Errorf("synchronizing blocks: %w", err)
	}

	if p.partialMode {
		ancestorStores, _ := p.graph.AncestorStoresOf(p.outputModuleNames[0]) //todo: new the list of parent store.
		outputStreamModule := p.builders[p.outputModuleNames[0]]
		var builders []*state.Builder
		for _, ancestorStore := range ancestorStores {
			builder := p.builders[ancestorStore.Name]
			builders = append(builders, builder)
		}

		fileWaiter := state.NewFileWaiter(p.requestedStartBlockNum, builders)
		err := fileWaiter.Wait(ctx, p.requestedStartBlockNum, outputStreamModule.ModuleStartBlock) //block until all parent storeModules have completed their tasks
		if err != nil {
			return fmt.Errorf("fileWaiter: %w", err)
		}
	}

	for _, store := range p.builders {
		if p.requestedStartBlockNum <= store.ModuleStartBlock+p.storesSaveInterval {
			continue
		}
		//if p.effectiveStartBlockNum < store.ModuleStartBlock {
		//	stateAtBlockNum = store.ModuleStartBlock
		//	continue
		//}
		if _, err := store.ReadState(ctx, p.requestedStartBlockNum); err != nil {
			e := fmt.Errorf("could not load state for store %s at block num %d: %s: %w", store.Name, p.requestedStartBlockNum, store.Store.BaseURL(), err)
			if !p.allowInvalidState {
				return e
			}
			zlog.Warn("reading state", zap.Error(e))
		}
		zlog.Info("adding store", zap.String("module_name", store.Name))
	}
	return nil
}

func (p *Pipeline) saveStoresSnapshots(ctx context.Context) error {
	isFirstRequestBlock := p.requestedStartBlockNum == p.clock.Number
	reachInterval := p.storesSaveInterval != 0 && p.clock.Number%p.storesSaveInterval == 0

	if !isFirstRequestBlock && reachInterval {
		for _, s := range p.builders {
			fileName, err := s.WriteState(ctx, p.clock.Number, p.partialMode)
			if err != nil {
				return fmt.Errorf("writing store '%s' state: %w", s.Name, err)
			}
			zlog.Info("state written", zap.String("store_name", s.Name), zap.String("file_name", fileName))
		}
	}
	return nil
}

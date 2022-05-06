package pipeline

import (
	"context"
	"fmt"
	"io"
	"runtime/debug"
	"strings"
	"time"

	"github.com/streamingfast/bstream"
	"github.com/streamingfast/dstore"
	"github.com/streamingfast/substreams"
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

	currentClock  *pbsubstreams.Clock
	moduleOutputs []*pbsubstreams.ModuleOutput
	logs          []string

	moduleOutputCache    *ModulesOutputCache
	baseOutputCacheStore dstore.Store
}

func New(
	ctx context.Context,
	request *pbsubstreams.Request,
	graph *manifest.ModuleGraph,
	blockType string,
	baseStateStore dstore.Store,
	baseOutputCacheStore dstore.Store,
	wasmExtensions []wasm.WASMExtensioner,
	opts ...Option) *Pipeline {

	pipe := &Pipeline{
		context:              ctx,
		request:              request,
		builders:             map[string]*state.Builder{},
		graph:                graph,
		baseStateStore:       baseStateStore,
		baseOutputCacheStore: baseOutputCacheStore,
		manifest:             request.Manifest,
		outputModuleNames:    request.OutputModules,
		outputModuleMap:      map[string]bool{},
		blockType:            blockType,
		progressTracker:      newProgressTracker(),
		wasmExtensions:       wasmExtensions,
	}

	for _, name := range request.OutputModules {
		pipe.outputModuleMap[name] = true
	}

	for _, opt := range opts {
		opt(pipe)
	}

	return pipe
}

// build will determine and run the builder that corresponds to the correct code type
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
			storeModule.Name,
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
				name := in.Store.ModuleName
				inputs = append(inputs, &wasm.Input{
					Type:   wasm.InputStore,
					Name:   name,
					Store:  p.builders[name],
					Deltas: true,
				})
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
		cache, err := p.moduleOutputCache.registerModule(ctx, module, hash, p.baseOutputCacheStore, p.requestedStartBlockNum)
		if err != nil {
			return fmt.Errorf("registering output cache for module %q: %w", module.Name, err)
		}

		switch kind := module.Kind.(type) {
		case *pbsubstreams.Module_KindMap_:
			outType := strings.TrimPrefix(module.Output.Type, "proto:")

			executor := &MapperModuleExecutor{
				BaseExecutor: &BaseExecutor{
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
				BaseExecutor: &BaseExecutor{
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

func (p *Pipeline) SynchronizeStores(ctx context.Context) error {
	//todo: compute modules start block. use module magic on the p.requestedStartBlockNum assume 10_000 block per file.
	// get last saved state block num for each modules
	// if last saved more the x * 10_000 behind computed start block we create a new substreams request for that module to process data upto computed start block
	// ex: token@100_000 computedStart@300_0000. we create a request for "tokens" with start block@100_000 and stop_block@300_000
	// we wait for that request to complete that we do the same with next modules.

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
		if p.requestedStartBlockNum == store.ModuleStartBlock {
			continue
		}
		if err := store.ReadState(ctx, p.requestedStartBlockNum); err != nil {
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

// `store` aura 4 modes d'opération:
//   * fetch an absolute snapshot from disk at EXACTLY the point we're starting
//   * fetch a partial snapshot, and fuse with previous snapshots, in which I need local "pairExtractor" building.
//   * connect to a remote firehose (I can cut the upstream dependencies
//   * if resources are available, SCHEDULE on BACKING NODES a parallel processing for that segment
//   * completely roll out LOCALLY the full historic reprocessing BEFORE continuing

func (p *Pipeline) HandlerFactory(returnFunc substreams.ReturnFunc) (bstream.Handler, error) {
	ctx := p.context
	// WARN: we don't support < 0 StartBlock for now
	p.requestedStartBlockNum = uint64(p.request.StartBlockNum)
	p.moduleOutputCache = NewModuleOutputCache()

	_, _, err := p.build(ctx, p.request)
	if err != nil {
		return nil, fmt.Errorf("building pipeline: %w", err)
	}
	stopBlock := p.request.StopBlockNum

	p.progressTracker.startTracking(ctx)

	err = p.SynchronizeStores(ctx)
	if err != nil {
		return nil, fmt.Errorf("synchonizing store: %w", err)
	}

	blockCount := 0

	go func() {
		for {
			time.Sleep(time.Second)
			blockCount = 0
		}
	}()

	if err != nil {
		return nil, fmt.Errorf("initiatin module output caches: %w", err)
	}

	return bstream.HandlerFunc(func(block *bstream.Block, obj interface{}) (err error) {
		clock := &pbsubstreams.Clock{
			Number:    block.Num(),
			Id:        block.Id,
			Timestamp: timestamppb.New(block.Time()),
		}

		defer func() {
			if r := recover(); r != nil {
				err = fmt.Errorf("panic at block %d: %s", block.Num(), r)
				zlog.Error("panic while process block", zap.Uint64("block_num", block.Num()), zap.Error(err))
				zlog.Error(string(debug.Stack()))
			}
			if err != nil {
				for _, hook := range p.postJobHooks {
					if err := hook(ctx, clock); err != nil {
						zlog.Warn("post job hook failed", zap.Error(err))
					}
				}
			}
		}()
		zlog.Debug("processing block", zap.Uint64("block_num", block.Number))
		blockCount++
		handleBlockStart := time.Now()

		p.moduleOutputs = nil
		p.wasmOutputs = map[string][]byte{}

		cursorable := obj.(bstream.Cursorable)
		cursor := cursorable.Cursor()

		stepable := obj.(bstream.Stepable)
		step := stepable.Step()

		p.currentClock = clock

		// TODO: eventually, handle the `undo` signals.
		//  NOTE: The RUNTIME will handle the undo signals. It'll have all it needs.

		if (p.requestedStartBlockNum != block.Number && p.storesSaveInterval != 0 && block.Num()%p.storesSaveInterval == 0) || block.Number >= stopBlock {
			if err := p.saveStoresSnapshots(ctx, clock.Number); err != nil {
				return err
			}
		}

		if block.Number >= stopBlock {
			return io.EOF
		}

		err = p.moduleOutputCache.update(ctx, block.AsRef())
		if err != nil {
			return fmt.Errorf("updating module output cache: %w", err)
		}

		for _, hook := range p.preBlockHooks {
			if err := hook(ctx, clock); err != nil {
				return fmt.Errorf("pre block hook: %w", err)
			}
		}

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

			clockBytes, err := proto.Marshal(clock)

			p.wasmOutputs[p.blockType] = blkBytes
			p.wasmOutputs["sf.substreams.v1.Clock"] = clockBytes
		default:
			panic("unsupported vmType " + p.vmType)
		}

		//todo: update module output kv with current block ref ????

		for _, executor := range p.moduleExecutors {
			if err := executor.run(p.wasmOutputs, clock, block); err != nil {
				return err
			}
			p.moduleOutputs = executor.appendOutput(p.moduleOutputs)
		}

		if len(p.moduleOutputs) > 0 {
			// TODO: package, after each execution, ALL of the modules we want changes for,
			// and add LOGS
			zlog.Debug("got modules outputs", zap.Int("module_output_count", len(p.moduleOutputs)))
			out := &pbsubstreams.BlockScopedData{
				Outputs: p.moduleOutputs,
				Clock:   clock,
				Step:    stepToProto(step),
				Cursor:  cursor.ToOpaque(),
			}
			if err := returnFunc(out); err != nil {
				return err
			}
		}

		for _, s := range p.builders {
			s.Flush()
		}
		zlog.Debug("block processed", zap.Uint64("block_num", block.Number))
		p.progressTracker.blockProcessed(block, time.Since(handleBlockStart))
		return nil
	}), nil
}

func (p *Pipeline) saveStoresSnapshots(ctx context.Context, blockNum uint64) error {
	for _, s := range p.builders {
		fileName, err := s.WriteState(ctx, blockNum, p.partialMode)
		if err != nil {
			return fmt.Errorf("writing store '%s' state: %w", s.Name, err)
		}
		zlog.Info("state written", zap.String("store_name", s.Name), zap.String("file_name", fileName))
	}
	return nil
}

func stepToProto(step bstream.StepType) pbsubstreams.ForkStep {
	switch step {
	case bstream.StepNew:
		return pbsubstreams.ForkStep_STEP_NEW
	case bstream.StepUndo:
		return pbsubstreams.ForkStep_STEP_UNDO
	case bstream.StepIrreversible:
		return pbsubstreams.ForkStep_STEP_IRREVERSIBLE
	}
	return pbsubstreams.ForkStep_STEP_UNKNOWN
}

type progressTracker struct {
	startAt                  time.Time
	processedBlockLastSecond int
	processedBlockCount      int
	blockSecond              int
	lastBlock                uint64
	timeSpentInStreamFuncs   time.Duration
}

func newProgressTracker() *progressTracker {
	return &progressTracker{}
}

func (p *progressTracker) startTracking(ctx context.Context) {
	p.startAt = time.Now()

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-time.After(1 * time.Second):
				p.blockSecond = p.processedBlockCount - p.processedBlockLastSecond
				p.processedBlockLastSecond = p.processedBlockCount
			}
		}
	}()

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-time.After(1 * time.Second):
				p.log()
			}
		}
	}()
}

func (p *progressTracker) blockProcessed(block *bstream.Block, delta time.Duration) {
	p.processedBlockCount += 1
	p.lastBlock = block.Num()
	p.timeSpentInStreamFuncs += delta
}

func (p *progressTracker) log() {
	zlog.Info("progress",
		zap.Uint64("last_block", p.lastBlock),
		zap.Int("total_processed_block", p.processedBlockCount),
		zap.Int("block_second", p.blockSecond),
		zap.Duration("stream_func_deltas", p.timeSpentInStreamFuncs))
	p.timeSpentInStreamFuncs = 0
}

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
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type Pipeline struct {
	vmType                 string // wasm/rust-v1, native
	requestedStartBlockNum uint64

	partialMode bool
	fileWaiter  *state.FileWaiter

	blockType string

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

	moduleExecutors []*ModuleExecutor
	wasmOutputs     map[string][]byte

	progressTracker    *progressTracker
	allowInvalidState  bool
	stateStore         dstore.Store
	storesSaveInterval uint64

	currentClock  *pbsubstreams.Clock
	moduleOutputs []*pbsubstreams.ModuleOutput
	logs          []string
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

func New(
	ctx context.Context,
	request *pbsubstreams.Request,
	graph *manifest.ModuleGraph,
	blockType string,
	stateStore dstore.Store,
	wasmExtensions []wasm.WASMExtensioner,
	opts ...Option) *Pipeline {

	pipe := &Pipeline{
		context:           ctx,
		request:           request,
		builders:          map[string]*state.Builder{},
		graph:             graph,
		stateStore:        stateStore,
		manifest:          request.Manifest,
		outputModuleNames: request.OutputModules,
		outputModuleMap:   map[string]bool{},
		blockType:         blockType,
		progressTracker:   newProgressTracker(),
		wasmExtensions:    wasmExtensions,
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
	for _, mod := range p.manifest.Modules {
		vmType := ""
		switch {
		case mod.GetWasmCode() != nil:
			vmType = mod.GetWasmCode().GetType()
		case mod.GetNativeCode() != nil:
			vmType = "native"
		default:
			return nil, nil, fmt.Errorf("invalid code type for modules %s ", mod.Name)
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

		store, err := state.NewStore(storeModule.Name, manifest.HashModuleAsString(p.manifest, p.graph, storeModule), storeModule.StartBlock, p.stateStore)
		if err != nil {
			return nil, nil, fmt.Errorf("init new store: %w", err)
		}

		builder := state.NewBuilder(
			storeModule.Name,
			storeModule.StartBlock,
			storeModule.GetKindStore().UpdatePolicy,
			storeModule.GetKindStore().ValueType,
			store,
			options...,
		)
		p.builders[builder.Name] = builder
	}

	if p.vmType == "native" {
		return nil, nil, fmt.Errorf("native schtuff not supported yet")
	}
	err = p.buildWASM(ctx, request, modules)
	return
}

type ModuleExecutor struct {
	moduleName string
	wasmModule *wasm.Module
	wasmInputs []*wasm.Input
	pipeline   *Pipeline
	isStore    bool
	isOutput   bool // whether output is enabled for this module

	outputStore *state.Builder

	mapperOutput []byte
	outputType   string
	entrypoint   string
}

func (p *Pipeline) buildWASM(ctx context.Context, request *pbsubstreams.Request, modules []*pbsubstreams.Module) error {
	p.wasmOutputs = map[string][]byte{}
	p.wasmRuntime = wasm.NewRuntime(p.wasmExtensions)

	for _, mod := range modules {
		isOutput := p.outputModuleMap[mod.Name]
		var inputs []*wasm.Input

		for _, in := range mod.Inputs {
			if inputMap := in.GetMap(); inputMap != nil {
				inputs = append(inputs, &wasm.Input{
					Type: wasm.InputSource,
					Name: inputMap.ModuleName,
				})
			} else if inputStore := in.GetStore(); inputStore != nil {
				inputName := inputStore.ModuleName
				if inputStore.Mode == pbsubstreams.Module_Input_Store_DELTAS {
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
			} else if inputSource := in.GetSource(); inputSource != nil {
				inputs = append(inputs, &wasm.Input{
					Type: wasm.InputSource,
					Name: inputSource.Type,
				})
			} else {
				return fmt.Errorf("invalid input struct for module %q", mod.Name)
			}
		}
		modName := mod.Name // to ensure it's enclosed

		wasmCodeRef := mod.GetWasmCode()
		if wasmCodeRef == nil {
			return fmt.Errorf("build_wasm cannot use modules that are not of type wasm")
		}
		entrypoint := wasmCodeRef.Entrypoint

		code := p.manifest.ModulesCode[wasmCodeRef.Index]
		wasmModule, err := p.wasmRuntime.NewModule(ctx, request, code, mod.Name)
		if err != nil {
			return fmt.Errorf("new wasm module: %w", err)
		}

		// TODO: turn into switch
		if v := mod.GetKindMap(); v != nil {

			outType := strings.TrimPrefix(mod.Output.Type, "proto:")

			s := &ModuleExecutor{
				moduleName: mod.Name,
				isStore:    false,
				pipeline:   p, // for currentClock, and wasmOutputs
				wasmModule: wasmModule,
				entrypoint: entrypoint,
				wasmInputs: inputs,
				isOutput:   isOutput,
				outputType: outType,
			}
			p.moduleExecutors = append(p.moduleExecutors, s)
			continue
		}
		if v := mod.GetKindStore(); v != nil {
			updatePolicy := v.UpdatePolicy
			valueType := v.ValueType

			outputStore := p.builders[modName]
			inputs = append(inputs, &wasm.Input{
				Type:         wasm.OutputStore,
				Name:         modName,
				Store:        outputStore,
				UpdatePolicy: updatePolicy,
				ValueType:    valueType,
			})

			p.moduleExecutors = append(p.moduleExecutors, &ModuleExecutor{
				moduleName:  modName,
				isStore:     true,
				outputStore: outputStore,
				pipeline:    p, // for currentClock, and wasmOutputs
				isOutput:    isOutput,
				wasmModule:  wasmModule,
				entrypoint:  entrypoint,
				wasmInputs:  inputs,
			})

			continue
		}
		return fmt.Errorf("invalid kind %q in module %q", mod.Kind, mod.Name)

	}

	return nil
}

func (p *Pipeline) SynchronizeStores(ctx context.Context) error {
	if p.partialMode {
		ancestorStores, _ := p.graph.AncestorStoresOf(p.outputModuleNames[0]) //todo: new the list of parent store.
		outputStreamModule := p.builders[p.outputModuleNames[0]]
		var stores []*state.Store
		for _, ancestorStore := range ancestorStores {
			builder := p.builders[ancestorStore.Name]
			stores = append(stores, builder.Store)
		}

		p.fileWaiter = state.NewFileWaiter(p.requestedStartBlockNum, stores)
		err := p.fileWaiter.Wait(ctx, p.requestedStartBlockNum, outputStreamModule.ModuleStartBlock) //block until all parent storeModules have completed their tasks
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

type StreamFunc func() error

func (p *Pipeline) HandlerFactory(returnFunc substreams.ReturnFunc) (bstream.Handler, error) {
	ctx := p.context
	// WARN: we don't support < 0 StartBlock for now
	p.requestedStartBlockNum = uint64(p.request.StartBlockNum)
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

	return bstream.HandlerFunc(func(block *bstream.Block, obj interface{}) (err error) {
		clock := &pbsubstreams.Clock{
			Number:    block.Num(),
			Id:        block.Id,
			Timestamp: timestamppb.New(block.Time()),
		}

		defer func() {
			if r := recover(); r != nil {
				err = fmt.Errorf("panic at block %d: %s", block.Num(), r)
				zlog.Error("panic while process block", zap.Uint64("block_nub", block.Num()), zap.Error(err))
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

		//cachedOutputs = modulesCacheEngine.outputsForBlock(blk.ID)
		//// do I have cached files for a given BLOCK ID,
		//// for modules X, Y, Z, and Y
		//// if I had but I don't anymore I recall build(), and prep a new stack
		//// otherwise I know I'm ready to use the cached X, Y, Z for block ID.
		//p.wasmOutputs.Merge(cachedOutputs)
		//
		//// upon build(), I'll check what I *realllly* need as immediate dependencies
		//// and I'll check if they are cached.
		//// I then need to constantly check if those dependencies are met, otherwise I need
		//// to RECHECK the true dependencies that *are* met, until I fallback completely
		//// on reexecution from scratch.
		//
		//if modulesCacheEngine.HitBoundaryForOneOfTheModules() {
		//	p.build()
		//}
		for _, executor := range p.moduleExecutors {
			if err := executor.run(); err != nil {
				return err
			}
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

type Printer interface {
	Print()
}

func printer(in interface{}) {
	if p, ok := in.(Printer); ok {
		p.Print()
	}
}

func (e *ModuleExecutor) run() (err error) {
	if e.isStore {
		err = e.wasmStoreCall()
	} else {
		err = e.wasmMapCall()
	}
	if err != nil {
		return err
	}

	if e.isOutput {
		e.appendOutput()
	}

	return nil
}
func (e *ModuleExecutor) wasmMapCall() (err error) {
	var vm *wasm.Instance
	if vm, err = e.wasmCall(); err != nil {
		return err
	}

	vals := e.pipeline.wasmOutputs
	name := e.moduleName
	if vm != nil {
		out := vm.Output()
		vals[name] = out
		e.mapperOutput = out

	} else {
		// This means wasm execution was skipped because all inputs were empty.
		vals[name] = nil
		e.mapperOutput = nil
	}
	return nil
}

func (e *ModuleExecutor) wasmStoreCall() (err error) {
	if _, err := e.wasmCall(); err != nil {
		return err
	}

	return nil
}

func (e *ModuleExecutor) wasmCall() (instance *wasm.Instance, err error) {
	hasInput := false
	vals := e.pipeline.wasmOutputs
	for _, input := range e.wasmInputs {
		switch input.Type {
		case wasm.InputSource:
			val := vals[input.Name]
			if len(val) != 0 {
				input.StreamData = val
				hasInput = true
			} else {
				input.StreamData = nil
			}
		case wasm.InputStore:
			hasInput = true
		case wasm.OutputStore:

		default:
			panic(fmt.Sprintf("Invalid input type %d", input.Type))
		}
	}

	// This allows us to skip the execution of the VM if there are no inputs.
	// This assumption should either be configurable by the manifest, or clearly documented:
	//  state builders will not be called if their input streams are 0 bytes length (and there'e no
	//  state store in read mode)
	if hasInput {
		instance, err = e.wasmModule.NewInstance(e.pipeline.currentClock, e.entrypoint, e.wasmInputs)
		if err != nil {
			return nil, fmt.Errorf("new wasm instance: %w", err)
		}
		if err = instance.Execute(); err != nil {
			return nil, fmt.Errorf("module %q: wasm execution failed: %w", e.moduleName, err)
		}
	}
	return
}

func (e *ModuleExecutor) appendOutput() {
	var logs []string
	if e.wasmModule.CurrentInstance != nil {
		logs = e.wasmModule.CurrentInstance.Logs
	}

	if e.isStore {
		if len(e.outputStore.Deltas) != 0 || len(logs) != 0 {
			zlog.Debug("append to output, store")
			e.pipeline.moduleOutputs = append(e.pipeline.moduleOutputs, &pbsubstreams.ModuleOutput{
				Name: e.moduleName,
				Data: &pbsubstreams.ModuleOutput_StoreDeltas{
					StoreDeltas: &pbsubstreams.StoreDeltas{Deltas: e.outputStore.Deltas},
				},
				Logs: logs,
			})
		}
	} else {
		if e.mapperOutput != nil || len(logs) != 0 {
			zlog.Debug("append to output, map")
			e.pipeline.moduleOutputs = append(e.pipeline.moduleOutputs, &pbsubstreams.ModuleOutput{
				Name: e.moduleName,
				Data: &pbsubstreams.ModuleOutput_MapOutput{
					MapOutput: &anypb.Any{TypeUrl: "type.googleapis.com/" + e.outputType, Value: e.mapperOutput},
				},
				Logs: logs,
			})
		}
	}
}

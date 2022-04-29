package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"runtime/debug"
	"strings"
	"time"

	"github.com/streamingfast/bstream"
	"github.com/streamingfast/dstore"
	"github.com/streamingfast/substreams"
	"github.com/streamingfast/substreams/manifest"
	imports "github.com/streamingfast/substreams/native-imports"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/registry"
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
	nativeImports  *imports.Imports
	builders       map[string]*state.Builder

	context           context.Context
	request           *pbsubstreams.Request
	graph             *manifest.ModuleGraph
	manifest          *pbsubstreams.Manifest
	outputModuleNames []string
	outputModuleMap   map[string]bool

	streamFuncs   []StreamFunc
	nativeOutputs map[string]reflect.Value
	wasmOutputs   map[string][]byte

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
		nativeImports:     imports.NewImports(),
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
		err = p.BuildNative(modules)
		return
	}
	err = p.buildWASM(ctx, request, modules)
	return
}

func (p *Pipeline) BuildNative(modules []*pbsubstreams.Module) error {
	// TODO: this would need to become on the same level as the WASM
	// modules, so using bytes as input and output and being there
	// only to avoid the overhead of the WASM VM.
	//
	// Perhaps it just disappears at some point. It already doesn't
	// support RPC anymore for Eth, and we're not going to build a
	// native abstraction for chain-specific things (yet?).
	nativeStreams := registry.Init(p.nativeImports)

	p.nativeOutputs = map[string]reflect.Value{}

	for _, mod := range modules {
		modName := mod.Name // to ensure it's enclosed

		nativeCode := mod.GetNativeCode()
		if nativeCode == nil {
			return fmt.Errorf("build_native cannot use modules that are not of type native")
		}
		f, found := nativeStreams[nativeCode.Entrypoint]
		if !found {
			return fmt.Errorf("native code not found for %q entry point %s", modName, nativeCode.Entrypoint)
		}

		debugOutput := p.outputModuleMap[modName]
		var inputs []string
		for _, in := range mod.Inputs {
			if v := in.GetMap(); v != nil {
				inputs = append(inputs, v.ModuleName)
			} else if v := in.GetStore(); v != nil {
				inputs = append(inputs, v.ModuleName)
			} else if v := in.GetSource(); v != nil {
				inputs = append(inputs, v.GetType())
			}
		}
		switch mod.Kind.(type) {
		case *pbsubstreams.Module_KindMap_:
			method := f.MethodByName("Map")
			if method.Kind() == reflect.Invalid {
				return fmt.Errorf("map() method not found on %T", f.Interface())
			}
			if method.IsZero() {
				return fmt.Errorf("map() method not found on %T", f.Interface())
			}
			outputFunc := func(msg proto.Message) error { return nil }
			if debugOutput {
				outputFunc = func(msg proto.Message) (err error) {
					if msg != nil {
						anyMsg, err := anypb.New(msg)
						if err != nil {
							return err
						}

						p.moduleOutputs = append(p.moduleOutputs, &pbsubstreams.ModuleOutput{
							Name: modName,
							Data: &pbsubstreams.ModuleOutput_MapOutput{
								MapOutput: anyMsg,
							},
							// FIXME: handle Logs when we need them.
						})
					}
					return
				}
			}
			p.streamFuncs = append(p.streamFuncs, func() error {
				return nativeMapCall(p.nativeOutputs, method, modName, inputs, outputFunc)
			})
			continue
		case *pbsubstreams.Module_KindStore_:
			method := f.MethodByName("store")
			if method.Kind() == reflect.Invalid {
				return fmt.Errorf("store() method not found on %T", f.Interface())
			}
			if method.IsZero() {
				return fmt.Errorf("store() method not found on %T", f.Interface())
			}

			outputStore := p.builders[mod.Name]
			p.nativeOutputs[mod.Name] = reflect.ValueOf(outputStore)

			outputFunc := func() error { return nil }
			if debugOutput {
				outputFunc = func() (err error) {
					if len(outputStore.Deltas) != 0 {
						p.moduleOutputs = append(p.moduleOutputs, &pbsubstreams.ModuleOutput{
							Name: modName,
							Data: &pbsubstreams.ModuleOutput_StoreDeltas{
								StoreDeltas: &pbsubstreams.StoreDeltas{Deltas: outputStore.Deltas},
							},
							// FIXME: handle Logs when we need them.
						})
					}
					return
				}
			}

			p.streamFuncs = append(p.streamFuncs, func() error {
				return nativeStoreCall(p.nativeOutputs, method, modName, inputs, outputFunc)
			})

			continue
		default:
			return fmt.Errorf("unknown value %q for 'kind' in stream %q", mod.Kind, mod.Name)
		}
	}

	return nil
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

		if v := mod.GetKindMap(); v != nil {

			outType := strings.TrimPrefix(mod.Output.Type, "proto:")

			outputFunc := func(out []byte) {}
			if isOutput {
				outputFunc = func(out []byte) {
					var logs []string
					if wasmModule.CurrentInstance != nil {
						logs = wasmModule.CurrentInstance.Logs
					}
					if out != nil || len(logs) != 0 {
						zlog.Debug("append to output, map")
						p.moduleOutputs = append(p.moduleOutputs, &pbsubstreams.ModuleOutput{
							Name: modName,
							Data: &pbsubstreams.ModuleOutput_MapOutput{
								MapOutput: &anypb.Any{TypeUrl: "type.googleapis.com/" + outType, Value: out},
							},
							Logs: logs,
						})
					}
				}
			}

			p.streamFuncs = append(p.streamFuncs, func() error {
				zlog.Debug("wasm map call", zap.String("module_name", modName))
				return wasmMapCall(p.currentClock, p.wasmOutputs, wasmModule, entrypoint, modName, inputs, outputFunc)
			})
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

			outputFunc := func() error { return nil }
			if isOutput {
				outputFunc = func() (err error) {
					var logs []string
					if wasmModule.CurrentInstance != nil {
						logs = wasmModule.CurrentInstance.Logs
					}
					if len(outputStore.Deltas) != 0 || len(logs) != 0 {
						zlog.Debug("append to output, store")
						p.moduleOutputs = append(p.moduleOutputs, &pbsubstreams.ModuleOutput{
							Name: modName,
							Data: &pbsubstreams.ModuleOutput_StoreDeltas{
								StoreDeltas: &pbsubstreams.StoreDeltas{Deltas: outputStore.Deltas},
							},
							Logs: logs,
						})
					}
					return
				}
			}

			p.streamFuncs = append(p.streamFuncs, func() error {
				zlog.Debug("wasm store call", zap.String("module_name", modName))
				return wasmStoreCall(p.currentClock, p.wasmOutputs, wasmModule, entrypoint, modName, inputs, outputFunc)
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
		p.nativeImports.SetCurrentBlock(block)

		// TODO: eventually, handle the `undo` signals.
		//  NOTE: The RUNTIME will handle the undo signals. It'll have all it needs.

		if (p.storesSaveInterval != 0 && block.Num()%p.storesSaveInterval == 0) || block.Number >= stopBlock {
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
			p.nativeOutputs[p.blockType /* "sf.ethereum.type.v1.Block" */] = reflect.ValueOf(blk)
			p.nativeOutputs["sf.substreams.v1.Clock"] = reflect.ValueOf(clock)
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

		for _, streamFunc := range p.streamFuncs {
			if err := streamFunc(); err != nil {
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

func nativeMapCall(vals map[string]reflect.Value, method reflect.Value, name string, inputs []string, output func(v proto.Message) error) error {
	var inputVals []reflect.Value
	for _, in := range inputs {
		inputVals = append(inputVals, vals[in])
	}
	out := method.Call(inputVals)
	if len(out) != 2 {
		return fmt.Errorf("invalid number of outputs for Map call in code for module %q, should be 2 (data, error), got %d", name, len(out))
	}
	vals[name] = out[0]

	if err, ok := out[1].Interface().(error); ok && err != nil {
		return fmt.Errorf("mapper module %q: %w", name, err)
	}

	// p, ok := out[0].Interface().(Printer)
	// if ok && printOutputs {
	// 	p.Print()
	// }

	cnt, err := json.Marshal(out[0].Interface())
	if err != nil {
		return fmt.Errorf("THIS IS HORRIBLE json encoding and failed, will be taken out: %w", err)
	}
	if err := output(&anypb.Any{TypeUrl: "json", Value: cnt}); err != nil {
		return fmt.Errorf("output native map call: %w", err)
	}

	return nil
}

func nativeStoreCall(vals map[string]reflect.Value, method reflect.Value, name string, inputs []string, output func() error) error {
	var inputVals []reflect.Value
	for _, in := range inputs {
		inputVals = append(inputVals, vals[in])
	}
	inputVals = append(inputVals, vals[name])

	// TODO: we can cache the `Method` retrieved on the stream.
	out := method.Call(inputVals)
	if len(out) != 1 {
		return fmt.Errorf("invalid number of outputs for 'store' call in code for module %q, should be 1 (error)", name)
	}
	if err, ok := out[0].Interface().(error); ok && err != nil {
		return fmt.Errorf("state builder module %q: %w", name, err)
	}

	if err := output(); err != nil {
		return fmt.Errorf("output native store call: %w", err)
	}
	// p, ok := vals[name].Interface().(Printer)
	// if ok && printOutputs {
	// 	p.Print()
	// }

	return nil
}

func wasmMapCall(clock *pbsubstreams.Clock,
	vals map[string][]byte,
	mod *wasm.Module,
	entrypoint string,
	name string,
	inputs []*wasm.Input,
	output func(out []byte),
) (err error) {
	var vm *wasm.Instance
	if vm, err = wasmCall(clock, vals, mod, entrypoint, name, inputs); err != nil {
		return err
	}

	if vm != nil {
		out := vm.Output()
		vals[name] = out
		output(out)

	} else {
		// This means wasm execution was skipped because all inputs were empty.
		vals[name] = nil
	}
	return nil
}

func wasmStoreCall(clock *pbsubstreams.Clock,
	vals map[string][]byte,
	mod *wasm.Module,
	entrypoint string,
	name string,
	inputs []*wasm.Input,
	output func() error,
) (err error) {
	if _, err := wasmCall(clock, vals, mod, entrypoint, name, inputs); err != nil {
		return err
	}

	if err := output(); err != nil {
		return fmt.Errorf("output wasm store call: %w", err)
	}

	return nil
}

func wasmCall(clock *pbsubstreams.Clock,
	vals map[string][]byte,
	mod *wasm.Module,
	entrypoint string,
	name string,
	inputs []*wasm.Input) (instance *wasm.Instance, err error) {

	hasInput := false
	for _, input := range inputs {
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
	//  state builders will not be called if their input streams are 0 bytes length (and there's no
	//  state store in read mode)
	if hasInput {
		instance, err = mod.NewInstance(clock, entrypoint, inputs)
		if err != nil {
			return nil, fmt.Errorf("new wasm instance: %w", err)
		}
		if err = instance.Execute(); err != nil {
			return nil, fmt.Errorf("module %q: wasm execution failed: %w", name, err)
		}
	}
	return
}

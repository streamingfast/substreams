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
	"github.com/streamingfast/eth-go/rpc"
	"github.com/streamingfast/substreams/manifest"
	imports "github.com/streamingfast/substreams/native-imports"
	pbethsubstreams "github.com/streamingfast/substreams/pb/sf/ethereum/substreams/v1"
	pbtransform "github.com/streamingfast/substreams/pb/sf/substreams/transform/v1"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/registry"
	ssrpc "github.com/streamingfast/substreams/rpc"
	"github.com/streamingfast/substreams/state"
	"github.com/streamingfast/substreams/wasm"
	"github.com/wasmerio/wasmer-go/wasmer"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

type Pipeline struct {
	vmType                 string // wasm/rust-v1, native
	requestedStartBlockNum uint64

	partialMode bool
	fileWaiter  *state.FileWaiter

	blockType string

	rpcCache      *ssrpc.Cache
	nativeImports *imports.Imports
	stores        map[string]*state.Builder

	ioFactory        state.FactoryInterface
	graph            *manifest.ModuleGraph
	manifest         *pbtransform.Manifest
	outputStreamName string

	streamFuncs       []StreamFunc
	nativeOutputs     map[string]reflect.Value
	wasmOutputs       map[string][]byte
	progressTracker   *progressTracker
	nextReturnValue   *anypb.Any
	allowInvalidState bool
}

type progressTracker struct {
	startAt                  time.Time
	processedBlockLastSecond int
	processedBlockCount      int
	blockSecond              int
	lastBlock                uint64
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
			case <-time.After(5 * time.Second):
				p.log()
			}
		}
	}()
}

func (p *progressTracker) blockProcessed(block *bstream.Block) {
	p.processedBlockCount += 1
	p.lastBlock = block.Num()
}

func (p *progressTracker) log() {
	zlog.Info("progress",
		zap.Uint64("last_block", p.lastBlock),
		zap.Int("total_processed_block", p.processedBlockCount),
		zap.Int("block_second", p.blockSecond))
}

type RpcProvider interface {
	RPC(calls *pbethsubstreams.RpcCalls) *pbethsubstreams.RpcResponses
}

type Option func(p *Pipeline)

func WithPartialMode() Option {
	return func(p *Pipeline) {
		p.partialMode = true
	}
}

func WithAllowInvalidState() Option {
	return func(p *Pipeline) {
		p.allowInvalidState = true
	}
}

func New(
	rpcClient *rpc.Client,
	rpcCache *ssrpc.Cache,
	manifest *pbtransform.Manifest,
	graph *manifest.ModuleGraph,
	outputStreamName string,
	blockType string,
	ioFactory state.FactoryInterface,
	opts ...Option) *Pipeline {
	pipe := &Pipeline{
		rpcCache:         rpcCache,
		nativeImports:    imports.NewImports(rpcClient, rpcCache),
		stores:           map[string]*state.Builder{},
		graph:            graph,
		ioFactory:        ioFactory,
		manifest:         manifest,
		outputStreamName: outputStreamName,
		blockType:        blockType,
		progressTracker:  newProgressTracker(),
	}

	for _, opt := range opts {
		opt(pipe)
	}

	return pipe
}

// build will determine and run the builder that corresponds to the correct code type
func (p *Pipeline) build() (modules []*pbtransform.Module, stores []*pbtransform.Module, err error) {

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

	modules, err = p.graph.ModulesDownTo(p.outputStreamName)
	if err != nil {
		return nil, nil, fmt.Errorf("building execution graph: %w", err)
	}

	p.stores = make(map[string]*state.Builder)
	stores, err = p.graph.StoresDownTo(p.outputStreamName)
	for _, store := range stores {
		var options []state.BuilderOption
		if p.partialMode {
			options = append(options, state.WithPartialMode(p.requestedStartBlockNum, p.outputStreamName))
		}
		store := state.NewBuilder(store.Name, store.StartBlock, store.GetKindStore().UpdatePolicy, store.GetKindStore().ValueType, p.ioFactory, options...)
		p.stores[store.Name] = store
	}

	if p.vmType == "native" {
		err = p.BuildNative(modules)
		return
	}
	err = p.buildWASM(modules)
	return
}

func (p *Pipeline) BuildNative(modules []*pbtransform.Module) error {

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

		debugOutput := modName == p.outputStreamName
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
		case *pbtransform.Module_KindMap:
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
						p.nextReturnValue, err = anypb.New(msg)
					}
					return
				}
			}
			fmt.Printf("Adding mapper for module %q\n", mod.Name)
			p.streamFuncs = append(p.streamFuncs, func() error {
				return nativeMapCall(p.nativeOutputs, method, modName, inputs, outputFunc)
			})
			continue
		case *pbtransform.Module_KindStore:
			method := f.MethodByName("store")
			if method.Kind() == reflect.Invalid {
				return fmt.Errorf("store() method not found on %T", f.Interface())
			}
			if method.IsZero() {
				return fmt.Errorf("store() method not found on %T", f.Interface())
			}

			outputStore := p.stores[mod.Name]
			p.nativeOutputs[mod.Name] = reflect.ValueOf(outputStore)

			outputFunc := func() error { return nil }
			if debugOutput {
				outputFunc = func() (err error) {
					if len(outputStore.Deltas) != 0 {
						p.nextReturnValue, err = anypb.New(&pbsubstreams.StoreDeltas{Deltas: outputStore.Deltas})
					}
					return
				}
			}

			fmt.Printf("Adding state builder for stream %q\n", mod.Name)
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

func (p *Pipeline) buildWASM(modules []*pbtransform.Module) error {

	p.wasmOutputs = map[string][]byte{}

	for _, mod := range modules {
		isOutput := mod.Name == p.outputStreamName
		var inputs []*wasm.Input

		for _, in := range mod.Inputs {
			if inputMap := in.GetMap(); inputMap != nil {
				inputs = append(inputs, &wasm.Input{
					Type: wasm.InputSource,
					Name: inputMap.ModuleName,
				})
			} else if inputStore := in.GetStore(); inputStore != nil {
				inputName := inputStore.ModuleName
				if inputStore.Mode == pbtransform.InputStore_DELTAS {
					inputs = append(inputs, &wasm.Input{
						Type:   wasm.InputStore,
						Name:   inputName,
						Store:  p.stores[inputName],
						Deltas: true,
					})
				} else {
					inputs = append(inputs, &wasm.Input{
						Type:  wasm.InputStore,
						Name:  inputName,
						Store: p.stores[inputName],
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
		wasmModule, err := wasm.NewModule(code, mod.Name)
		if err != nil {
			return fmt.Errorf("new wasm module: %w", err)
		}

		rpcWasmFuncFact := GetRPCWasmFunctionFactory(p.nativeImports)
		if v := mod.GetKindMap(); v != nil {
			fmt.Printf("Adding mapper for module %q\n", modName)

			outType := strings.TrimPrefix(mod.Output.Type, "proto:")

			outputFunc := func(out []byte) {}
			if isOutput {
				outputFunc = func(out []byte) {
					if out != nil {
						p.nextReturnValue = &anypb.Any{TypeUrl: "type.googleapis.com/" + outType, Value: out}
					}
				}
			}

			p.streamFuncs = append(p.streamFuncs, func() error {
				return wasmMapCall(p.wasmOutputs, wasmModule, entrypoint, modName, inputs, outputFunc, rpcWasmFuncFact)
			})
			continue
		}
		if v := mod.GetKindStore(); v != nil {
			updatePolicy := v.UpdatePolicy
			valueType := v.ValueType

			outputStore := p.stores[modName]
			inputs = append(inputs, &wasm.Input{
				Type:         wasm.OutputStore,
				Name:         modName,
				Store:        outputStore,
				UpdatePolicy: updatePolicy,
				ValueType:    valueType,
			})
			fmt.Printf("Adding state builder for module %q\n", modName)

			outputFunc := func() error { return nil }
			if isOutput {
				outputFunc = func() (err error) {
					if len(outputStore.Deltas) != 0 {
						p.nextReturnValue, err = anypb.New(&pbsubstreams.StoreDeltas{Deltas: outputStore.Deltas})
					}
					return
				}
			}

			p.streamFuncs = append(p.streamFuncs, func() error {
				return wasmStoreCall(p.wasmOutputs, wasmModule, entrypoint, modName, inputs, outputFunc, rpcWasmFuncFact)
			})
			continue
		}
		return fmt.Errorf("invalid kind %q in module %q", mod.Kind, mod.Name)

	}

	return nil
}

func GetRPCWasmFunctionFactory(rpcProv RpcProvider) wasm.WasmerFunctionFactory {
	return func(instance *wasm.Instance) (namespace string, name string, wasmerFunc *wasmer.Function) {
		namespace = "rpc"
		name = "eth_call"
		wasmerFunc = wasmer.NewFunction(
			instance.Store(),
			wasmer.NewFunctionType(
				wasm.Params(wasmer.I32, wasmer.I32, wasmer.I32), // 0(READ): proto RPCCalls offset,  1(READ): proto RPCCalls len, 2(WRITE): offset for proto RPCResponses
				wasm.Returns()),
			func(args []wasmer.Value) ([]wasmer.Value, error) {

				heap := instance.Heap()

				message, err := heap.ReadBytes(args[0].I32(), args[1].I32())
				if err != nil {
					return nil, fmt.Errorf("read message argument: %w", err)
				}

				rpcCalls := &pbethsubstreams.RpcCalls{}
				err = proto.Unmarshal(message, rpcCalls)
				if err != nil {
					return nil, fmt.Errorf("unmarshal message %w", err)
				}

				responses := rpcProv.RPC(rpcCalls)
				responsesBytes, err := proto.Marshal(responses)
				if err != nil {
					return nil, fmt.Errorf("marshall message: %w", err)
				}

				err = instance.WriteOutputToHeap(args[2].I32(), responsesBytes)
				if err != nil {
					return nil, fmt.Errorf("write output to heap %w", err)
				}
				return nil, nil
			},
		)
		return
	}
}

func (p *Pipeline) SynchronizeStores(ctx context.Context) error {
	if p.partialMode {
		p.fileWaiter = state.NewFileWaiter(p.outputStreamName, p.graph, p.ioFactory, p.requestedStartBlockNum)
		err := p.fileWaiter.Wait(ctx, p.requestedStartBlockNum) //block until all parent storeModules have completed their tasks
		if err != nil {
			return fmt.Errorf("fileWaiter: %w", err)
		}
	}

	for _, store := range p.stores {
		if err := store.ReadState(ctx, p.requestedStartBlockNum); err != nil {
			e := fmt.Errorf("could not load state for store %s at block num %d: %w", store.Name, p.requestedStartBlockNum, err)
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

func (p *Pipeline) HandlerFactory(ctx context.Context, requestedStartBlockNum uint64, stopBlock uint64, returnFunc func(any *anypb.Any) error) (bstream.Handler, error) {

	p.requestedStartBlockNum = requestedStartBlockNum
	_, _, err := p.build()
	if err != nil {
		return nil, fmt.Errorf("building pipeline: %w", err)
	}

	err = p.SynchronizeStores(ctx)
	if err != nil {
		return nil, fmt.Errorf("synchonizing store: %w", err)
	}

	fmt.Println(p.stores)
	p.progressTracker.startTracking(ctx)

	return bstream.HandlerFunc(func(block *bstream.Block, obj interface{}) (err error) {
		defer func() {
			if r := recover(); r != nil {
				err = fmt.Errorf("panic at block %d: %s", block.Num(), r)
				zlog.Error("panic while process block", zap.Uint64("block_nub", block.Num()), zap.Error(err))
				zlog.Error(string(debug.Stack()))
			}
		}()

		// TODO: eventually, handle the `undo` signals.
		//  NOTE: The RUNTIME will handle the undo signals. It'll have all it needs.
		if block.Number >= stopBlock {
			for _, s := range p.stores {
				err := s.WriteState(context.Background(), block.Num())
				if err != nil {
					return fmt.Errorf("error writing block %d to store %s: %w", block.Num(), s.Name, err)
				}
			}

			p.rpcCache.Save(context.Background())

			return io.EOF
		}

		p.nativeImports.SetCurrentBlock(block)
		p.nextReturnValue = nil

		//clock := toClock(block)

		blk := block.ToProtocol()
		switch p.vmType {
		case "native":
			p.nativeOutputs[p.blockType /*"sf.ethereum.type.v1.Block" */] = reflect.ValueOf(blk)
		case "wasm/rust-v1":
			// block.Payload.Get() could do the same, but does it go through the same
			// CORRECTIONS of the block, that the BlockDecoder does?
			blkBytes, err := proto.Marshal(blk.(proto.Message))
			if err != nil {
				return fmt.Errorf("packing block: %w", err)
			}

			p.wasmOutputs[p.blockType] = blkBytes
			//p.wasmOutputs["sf.substreams.v1.Clock"] = clock //FIXME stepd implement clock
		default:
			panic("unsupported vmType " + p.vmType)
		}

		fmt.Println("-------------------------------------------------------------------")
		fmt.Printf("BLOCK +%d %d %s\n", block.Num()-p.requestedStartBlockNum, block.Num(), block.ID())

		for _, streamFunc := range p.streamFuncs {
			if err := streamFunc(); err != nil {
				return err
			}
		}

		// Prep for next block, clean-up all deltas.
		for _, s := range p.stores {
			s.Flush()
		}

		p.progressTracker.blockProcessed(block)

		if err := returnFunc(p.nextReturnValue); err != nil {
			return err
		}

		return nil
	}), nil
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

func wasmMapCall(vals map[string][]byte,
	mod *wasm.Module,
	entrypoint string,
	name string,
	inputs []*wasm.Input,
	output func(out []byte),
	rpcFactory wasm.WasmerFunctionFactory,
) (err error) {
	var vm *wasm.Instance
	if vm, err = wasmCall(vals, mod, entrypoint, name, inputs, rpcFactory); err != nil {
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

func wasmStoreCall(vals map[string][]byte,
	mod *wasm.Module,
	entrypoint string,
	name string,
	inputs []*wasm.Input,
	output func() error,
	rpcFactory wasm.WasmerFunctionFactory,
) (err error) {
	if _, err := wasmCall(vals, mod, entrypoint, name, inputs, rpcFactory); err != nil {
		return err
	}

	if err := output(); err != nil {
		return fmt.Errorf("output wasm store call: %w", err)
	}

	return nil
}

func wasmCall(vals map[string][]byte,
	mod *wasm.Module,
	entrypoint string,
	name string,
	inputs []*wasm.Input,
	rpcFactory wasm.WasmerFunctionFactory) (instance *wasm.Instance, err error) {

	hasInput := false
	for _, input := range inputs {
		switch input.Type {
		case wasm.InputSource:
			val := vals[input.Name]
			if len(val) != 0 {
				input.StreamData = val
				hasInput = true
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
		instance, err = mod.NewInstance(entrypoint, inputs, rpcFactory)
		if err != nil {
			return nil, fmt.Errorf("new wasm instance: %w", err)
		}
		if err = instance.Execute(); err != nil {
			return nil, fmt.Errorf("module %q: wasm execution failed: %w", name, err)
		}
		instance.Close()
	}
	return
}

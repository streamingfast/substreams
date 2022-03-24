package pipeline

import (
	"context"
	"fmt"
	"io"
	"reflect"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/streamingfast/bstream"
	"github.com/streamingfast/eth-go/rpc"
	"github.com/streamingfast/substreams/manifest"
	imports "github.com/streamingfast/substreams/native-imports"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/ethereum/substreams/v1"
	pbtransform "github.com/streamingfast/substreams/pb/sf/substreams/transform/v1"
	"github.com/streamingfast/substreams/registry"
	ssrpc "github.com/streamingfast/substreams/rpc"
	"github.com/streamingfast/substreams/state"
	"github.com/streamingfast/substreams/wasm"
	"github.com/wasmerio/wasmer-go/wasmer"
	"google.golang.org/protobuf/proto"
)

type Pipeline struct {
	vmType        string // wasm, native
	startBlockNum uint64

	partialMode bool
	fileWaiter  *state.FileWaiter

	blockType string

	rpcClient *rpc.Client
	rpcCache  *ssrpc.Cache

	nativeImports *imports.Imports
	stores        map[string]*state.Builder

	ioFactory        state.FactoryInterface
	graph            *manifest.ModuleGraph
	manifest         *pbtransform.Manifest
	outputStreamName string

	streamFuncs   []StreamFunc
	nativeOutputs map[string]reflect.Value
	wasmOutputs   map[string][]byte
	progress      *progress
}

type progress struct {
	startAt                  time.Time
	lastLogAt                time.Time
	processedBlockLastSecond int
	processedBlockCount      int
	blockSecond              int
	lastBlock                uint64
}

func (p *progress) startTracking() {
	p.startAt = time.Now()
	go func() {
		for {
			<-time.After(1 * time.Second)
			p.blockSecond = p.processedBlockCount - p.processedBlockLastSecond
			p.processedBlockLastSecond = p.processedBlockCount
		}
	}()
	go func() {
		for {
			<-time.After(5 * time.Second)
			p.log()
		}
	}()
}
func (p *progress) blockProcessed(block *bstream.Block) {
	p.processedBlockCount += 1
	p.lastBlock = block.Num()
}

func (p *progress) log() {
	zlog.Info("progress",
		zap.Uint64("last_block", p.lastBlock),
		zap.Int("total_processed_block", p.processedBlockCount),
		zap.Int("block_second", p.blockSecond))
}

type RpcProvider interface {
	RPC(calls *pbsubstreams.RpcCalls) *pbsubstreams.RpcResponses
}

type Option func(p *Pipeline)

func WithPartialMode(reqStartBlock uint64) Option {
	return func(p *Pipeline) {
		p.partialMode = true
		p.fileWaiter = state.NewFileWaiter(p.outputStreamName, p.graph, p.ioFactory, reqStartBlock)
	}
}

func New(startBlockNum uint64,
	rpcClient *rpc.Client,
	rpcCache *ssrpc.Cache,
	manif *pbtransform.Manifest,
	graph *manifest.ModuleGraph,
	outputStreamName string,
	blockType string,
	ioFactory state.FactoryInterface,
	opts ...Option) *Pipeline {
	pipe := &Pipeline{
		startBlockNum:    startBlockNum,
		rpcClient:        rpcClient,
		rpcCache:         rpcCache,
		nativeImports:    imports.NewImports(rpcClient, rpcCache, true),
		stores:           map[string]*state.Builder{},
		graph:            graph,
		ioFactory:        ioFactory,
		manifest:         manif,
		outputStreamName: outputStreamName,
		blockType:        blockType,
		progress:         &progress{},
	}

	for _, opt := range opts {
		opt(pipe)
	}

	return pipe
}

// Build will determine and run the builder that corresponds to the correct code type
func (p *Pipeline) Build(ctx context.Context, forceLoadState bool) error {

	for _, mod := range p.manifest.Modules {
		vmType := ""
		switch {
		case mod.GetWasmCode() != nil:
			vmType = mod.GetWasmCode().GetType()
		case mod.GetNativeCode() != nil:
			vmType = "native"
		default:
			return fmt.Errorf("invalid code type for modules %s ", mod.Name)
		}
		if p.vmType != "" && vmType != p.vmType {
			return fmt.Errorf("cannot process modules of different code types: %s vs %s", p.vmType, vmType)
		}
		p.vmType = vmType
	}

	if p.vmType == "native" {
		return p.BuildNative(ctx, forceLoadState)
	}

	return p.BuildWASM(ctx, forceLoadState)
}

func (p *Pipeline) BuildNative(ctx context.Context, forceLoadState bool) error {
	modules, err := p.graph.ModulesDownTo(p.outputStreamName)
	if err != nil {
		return fmt.Errorf("whoops: %w", err)
	}

	nativeStreams := registry.Init(p.nativeImports)

	if err := p.setupStores(ctx, p.graph, p.ioFactory, forceLoadState); err != nil {
		return fmt.Errorf("setting up stores: %w", err)
	}
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

		// debugOutput := modName == p.outputStreamName
		inputs := []string{}
		for _, in := range mod.Inputs {
			if v := in.GetMap(); v != nil {
				inputs = append(inputs, v.ModuleName)
			} else if v := in.GetStore(); v != nil {
				inputs = append(inputs, v.ModuleName)
			} else if v := in.GetSource(); v != nil {
				inputs = append(inputs, v.GetType())
			}
		}

		if v := mod.GetKindMap(); v != nil {
			method := f.MethodByName("Map")
			if method.Kind() == reflect.Invalid {
				return fmt.Errorf("Map() method not found on %T", f.Interface())
			}
			if method.IsZero() {
				return fmt.Errorf("Map() method not found on %T", f.Interface())
			}
			fmt.Printf("Adding mapper for module %q\n", mod.Name)
			p.streamFuncs = append(p.streamFuncs, func() error {
				return nativeMapCall(p.nativeOutputs, method, modName, inputs, true)
			})
			continue
		}

		if v := mod.GetKindStore(); v != nil {
			method := f.MethodByName("Store")
			if method.Kind() == reflect.Invalid {
				return fmt.Errorf("store() method not found on %T", f.Interface())
			}
			if method.IsZero() {
				return fmt.Errorf("store() method not found on %T", f.Interface())
			}

			p.nativeOutputs[mod.Name] = reflect.ValueOf(p.stores[mod.Name])

			fmt.Printf("Adding state builder for stream %q\n", mod.Name)
			p.streamFuncs = append(p.streamFuncs, func() error {
				return nativeStoreCall(p.nativeOutputs, method, modName, inputs, true)
			})

			continue
		}

		return fmt.Errorf("unknown value %q for 'kind' in stream %q", mod.Kind, mod.Name)

	}

	return nil
}

func (p *Pipeline) BuildWASM(ctx context.Context, forceLoadState bool) error {
	modules, err := p.graph.ModulesDownTo(p.outputStreamName)
	if err != nil {
		return fmt.Errorf("building execution graph: %w", err)
	}

	if err := p.setupStores(ctx, p.graph, p.ioFactory, forceLoadState); err != nil {
		return fmt.Errorf("setting up stores: %w", err)
	}

	p.wasmOutputs = map[string][]byte{}

	for _, mod := range modules {
		debugOutput := mod.Name == p.outputStreamName
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

			outType := mod.Output.Type
			if strings.HasPrefix(outType, "proto:") {
				outType = outType[6:]
			}
			p.streamFuncs = append(p.streamFuncs, func() error {
				return wasmMapCall(p.wasmOutputs, wasmModule, entrypoint, modName, inputs, debugOutput, rpcWasmFuncFact, outType)
			})
			continue
		}
		if v := mod.GetKindStore(); v != nil {
			updatePolicy := v.UpdatePolicy
			valueType := v.ValueType

			inputs = append(inputs, &wasm.Input{
				Type:         wasm.OutputStore,
				Name:         modName,
				Store:        p.stores[modName],
				UpdatePolicy: updatePolicy,
				ValueType:    valueType,
			})
			fmt.Printf("Adding state builder for module %q\n", modName)

			p.streamFuncs = append(p.streamFuncs, func() error {
				return wasmStoreCall(p.wasmOutputs, wasmModule, entrypoint, modName, inputs, debugOutput, rpcWasmFuncFact)
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

				rpcCalls := &pbsubstreams.RpcCalls{}
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

func (p *Pipeline) setupStores(ctx context.Context, graph *manifest.ModuleGraph, ioFactory state.FactoryInterface, forceLoadState bool) error {
	if p.fileWaiter != nil {
		err := p.fileWaiter.Wait(ctx) //block until all parent stores have completed their tasks
		if err != nil {
			return fmt.Errorf("fileWaiter: %w", err)
		}
	}

	modules, err := graph.StoresDownTo(p.outputStreamName)
	if err != nil {
		return err
	}

	p.stores = make(map[string]*state.Builder)
	for _, s := range modules {
		store := state.NewBuilder(s.Name, s.GetKindStore().UpdatePolicy, s.GetKindStore().ValueType, ioFactory,
			state.WithPartialMode(p.partialMode, p.startBlockNum, s.InitialBlock, p.outputStreamName),
		)

		var initializeStore bool
		if p.partialMode {
			/// initialize all parent store data
			if p.outputStreamName != s.Name {
				initializeStore = true
			}
		} else {
			if forceLoadState {
				initializeStore = true
			}
		}

		if initializeStore {
			if err := store.Init(ctx, p.startBlockNum); err != nil {
				return fmt.Errorf("could not load state for store %s at block num %d: %w", s.Name, p.startBlockNum, err)
			}
		}

		p.stores[s.Name] = store
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

func (p *Pipeline) HandlerFactory(stopBlock uint64) bstream.Handler {

	p.progress.startTracking()
	return bstream.HandlerFunc(func(block *bstream.Block, obj interface{}) (err error) {
		// defer func() {
		// 	if r := recover(); r != nil {
		// 		err = fmt.Errorf("panic: %w", r)
		// 	}
		// }()

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
		fmt.Printf("BLOCK +%d %d %s\n", block.Num()-p.startBlockNum, block.Num(), block.ID())

		// LockOSThread is to avoid this goroutine to be MOVED by the Go runtime to another system thread,
		// while wasmer is using some instances in a given thread. Wasmer will not be happy if the goroutine
		// switched thread and tries to access a wasmer instance from a different one.
		//runtime.LockOSThread()
		//defer runtime.UnlockOSThread()
		for _, streamFunc := range p.streamFuncs {
			if err := streamFunc(); err != nil {
				return err
			}
		}

		// Prep for next block, clean-up all deltas.
		for _, s := range p.stores {
			s.Flush()
		}

		p.progress.blockProcessed(block)

		return nil
	})
}

type Printer interface {
	Print()
}

func printer(in interface{}) {
	if p, ok := in.(Printer); ok {
		p.Print()
	}
}

func nativeMapCall(vals map[string]reflect.Value, method reflect.Value, name string, inputs []string, printOutputs bool) error {
	var inputVals []reflect.Value
	for _, in := range inputs {
		inputVals = append(inputVals, vals[in])
	}
	out := method.Call(inputVals)
	if len(out) != 2 {
		return fmt.Errorf("invalid number of outputs for Map call in code for module %q, should be 2 (data, error), got %d", name, len(out))
	}
	vals[name] = out[0]

	p, ok := out[0].Interface().(Printer)
	if ok && printOutputs {
		p.Print()
	}

	if err, ok := out[1].Interface().(error); ok && err != nil {
		return fmt.Errorf("mapper module %q: %w", name, err)
	}
	return nil
}

func nativeStoreCall(vals map[string]reflect.Value, method reflect.Value, name string, inputs []string, printOutputs bool) error {
	var inputVals []reflect.Value
	for _, in := range inputs {
		inputVals = append(inputVals, vals[in])
	}
	inputVals = append(inputVals, vals[name])

	// TODO: we can cache the `Method` retrieved on the stream.
	out := method.Call(inputVals)
	if len(out) != 1 {
		return fmt.Errorf("invalid number of outputs for 'Store' call in code for module %q, should be 1 (error)", name)
	}
	p, ok := vals[name].Interface().(Printer)
	if ok && printOutputs {
		p.Print()
	}
	if err, ok := out[0].Interface().(error); ok && err != nil {
		return fmt.Errorf("state builder module %q: %w", name, err)
	}
	return nil
}

func wasmMapCall(vals map[string][]byte,
	mod *wasm.Module,
	entrypoint string,
	name string,
	inputs []*wasm.Input,
	printOutputs bool,
	rpcFactory wasm.WasmerFunctionFactory,
	msgType string,
) (err error) {
	var vm *wasm.Instance
	if vm, err = wasmCall(vals, mod, entrypoint, name, inputs, rpcFactory); err != nil {
		return err
	}
	if vm != nil {
		out := vm.Output()
		vals[name] = out
		//FIXME printoutput
	} else {
		vals[name] = nil
	}
	return nil
}

func wasmStoreCall(vals map[string][]byte,
	mod *wasm.Module,
	entrypoint string,
	name string,
	inputs []*wasm.Input,
	printOutputs bool,
	rpcFactory wasm.WasmerFunctionFactory,
) (err error) {
	var vm *wasm.Instance
	if vm, err = wasmCall(vals, mod, entrypoint, name, inputs, rpcFactory); err != nil {
		return err
	}
	if vm != nil && printOutputs {
		vm.PrintDeltas()
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

package pipeline

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"

	"github.com/jhump/protoreflect/desc"
	"github.com/jhump/protoreflect/dynamic"
	"github.com/streamingfast/bstream"
	"github.com/streamingfast/eth-go/rpc"
	"github.com/streamingfast/substreams/manifest"
	imports "github.com/streamingfast/substreams/native-imports"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/ethereum/substreams/v1"
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

	initFunc func() error

	partialMode bool

	blockType string

	rpcClient *rpc.Client
	rpcCache  *ssrpc.Cache

	nativeImports *imports.Imports
	stores        map[string]*state.Builder

	manifest         *manifest.Manifest
	outputStreamName string

	streamFuncs   []StreamFunc
	nativeOutputs map[string]reflect.Value
	wasmOutputs   map[string][]byte
}

type RpcProvider interface {
	RPC(calls *pbsubstreams.RpcCalls) *pbsubstreams.RpcResponses
}

func New(startBlockNum uint64, rpcClient *rpc.Client, rpcCache *ssrpc.Cache, manif *manifest.Manifest, outputStreamName string, blockType string, partialMode bool) *Pipeline {
	pipe := &Pipeline{
		startBlockNum:    startBlockNum,
		rpcClient:        rpcClient,
		rpcCache:         rpcCache,
		nativeImports:    imports.NewImports(rpcClient, rpcCache, true),
		stores:           map[string]*state.Builder{},
		manifest:         manif,
		outputStreamName: outputStreamName,
		vmType:           manif.CodeType,
		blockType:        blockType,
		partialMode:      partialMode,
	}
	return pipe
}

func (p *Pipeline) SetInitFunc(f func() error) {
	p.initFunc = f
}

func (p *Pipeline) BuildNative(ioFactory state.IOFactory, forceLoadState bool) error {
	modules, err := p.manifest.Graph.ModulesDownTo(p.outputStreamName)
	if err != nil {
		return fmt.Errorf("whoops: %w", err)
	}

	nativeStreams := registry.Init(p.nativeImports)

	if err := p.setupStores(modules, ioFactory, forceLoadState); err != nil {
		return fmt.Errorf("setting up stores: %w", err)
	}
	p.nativeOutputs = map[string]reflect.Value{}

	for _, mod := range modules {
		f, found := nativeStreams[mod.Code.Native]
		if !found {
			return fmt.Errorf("native code not found for %q", mod.Code)
		}

		debugOutput := mod.Name == p.outputStreamName
		inputs := []string{}
		for _, in := range mod.Inputs {
			inputs = append(inputs, strings.Split(in.Name, ":")[1])
		}
		modName := mod.Name // to ensure it's enclosed

		switch mod.Kind {
		case "map":
			method := f.MethodByName("Map")
			if method.Kind() == reflect.Invalid {
				return fmt.Errorf("Map() method not found on %T", f.Interface())
			}
			if method.IsZero() {
				return fmt.Errorf("Map() method not found on %T", f.Interface())
			}
			fmt.Printf("Adding mapper for module %q\n", mod.Name)
			p.streamFuncs = append(p.streamFuncs, func() error {
				return nativeMapCall(p.nativeOutputs, method, modName, inputs, debugOutput)
			})
		case "store":
			method := f.MethodByName("Store")
			if method.Kind() == reflect.Invalid {
				return fmt.Errorf("Store() method not found on %T", f.Interface())
			}
			if method.IsZero() {
				return fmt.Errorf("Store() method not found on %T", f.Interface())
			}

			p.nativeOutputs[mod.Name] = reflect.ValueOf(p.stores[mod.Name])

			fmt.Printf("Adding state builder for stream %q\n", mod.Name)
			p.streamFuncs = append(p.streamFuncs, func() error {
				return nativeStoreCall(p.nativeOutputs, method, modName, inputs, debugOutput)
			})

		default:
			return fmt.Errorf("unknown value %q for 'kind' in stream %q", mod.Kind, mod.Name)
		}

	}

	p.vmType = "native"

	return nil
}

func (p *Pipeline) BuildWASM(ioFactory state.IOFactory, forceLoadState bool) error {
	modules, err := p.manifest.Graph.ModulesDownTo(p.outputStreamName)
	if err != nil {
		return fmt.Errorf("building execution graph: %w", err)
	}

	if err := p.setupStores(modules, ioFactory, forceLoadState); err != nil {
		return fmt.Errorf("setting up stores: %w", err)
	}

	p.wasmOutputs = map[string][]byte{}
	protoDescs := p.manifest.ProtoDescs
	protoMsgFactory := dynamic.NewMessageFactoryWithDefaults()

	for _, mod := range modules {
		debugOutput := mod.Name == p.outputStreamName
		var inputs []*wasm.Input
		for _, in := range mod.Inputs {
			if in.Map != "" {
				inputs = append(inputs, &wasm.Input{
					Type: wasm.InputSource,
					Name: in.Map,
				})
			} else if in.Store != "" {
				inputName := in.Store
				if in.Mode == "deltas" {
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
			} else if in.Source != "" {
				inputs = append(inputs, &wasm.Input{
					Type: wasm.InputSource,
					Name: in.Source,
				})
			} else {
				return fmt.Errorf("invalid input struct for module %q", mod.Name)
			}
		}
		modName := mod.Name // to ensure it's enclosed

		wasmModule, err := wasm.NewModule(mod.Code.Content, filepath.Base(mod.Code.File))
		if err != nil {
			return fmt.Errorf("new wasm module: %w", err)
		}

		rpcWasmFuncFact := GetRPCWasmFunctionFactory(p.nativeImports)
		switch mod.Kind {
		case "map":
			fmt.Printf("Adding mapper for module %q\n", modName)

			outType := mod.Output.Type
			if strings.HasPrefix(outType, "proto:") {
				outType = outType[6:]
			}
			var msgDesc *desc.MessageDescriptor
			for _, file := range protoDescs {
				msgDesc = file.FindMessage(outType)
				if msgDesc != nil {
					break
				}
			}
			var protoDecode func(in []byte) (string, error)
			if msgDesc != nil {
				protoDecode = func(in []byte) (string, error) {
					msg := protoMsgFactory.NewDynamicMessage(msgDesc)
					if err := msg.Unmarshal(in); err != nil {
						return "", fmt.Errorf("error unmarshalling protobuf %s to map: %w", outType, err)
					}
					cnt, err := msg.MarshalJSONIndent()
					if err != nil {
						return "", fmt.Errorf("error encoding protobuf %s into json: %w", outType, err)
					}
					return string(cnt), nil
				}
			}

			entrypoint := mod.Code.Entrypoint
			p.streamFuncs = append(p.streamFuncs, func() error {
				return wasmMapCall(p.wasmOutputs, wasmModule, entrypoint, modName, inputs, debugOutput, rpcWasmFuncFact, protoDecode, outType)
			})
		case "store":
			updatePolicy := mod.Output.UpdatePolicy
			valueType := mod.Output.ValueType
			protoType := mod.Output.ProtoType

			entrypoint := mod.Code.Entrypoint
			inputs = append(inputs, &wasm.Input{
				Type:         wasm.OutputStore,
				Name:         modName,
				Store:        p.stores[modName],
				UpdatePolicy: updatePolicy,
				ValueType:    valueType,
				ProtoType:    protoType,
			})
			fmt.Printf("Adding state builder for module %q\n", modName)

			p.streamFuncs = append(p.streamFuncs, func() error {
				return wasmStoreCall(p.wasmOutputs, wasmModule, entrypoint, modName, inputs, debugOutput, rpcWasmFuncFact)
			})

		default:
			return fmt.Errorf("unknown value %q for 'kind' in module %q", mod.Kind, mod.Name)
		}

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

func (p *Pipeline) setupStores(modules []*manifest.Module, ioFactory state.IOFactory, forceLoadState bool) error {
	p.stores = make(map[string]*state.Builder)
	for _, mod := range modules {
		if mod.Kind != "store" {
			continue
		}
		output := mod.Output
		store := state.NewBuilder(mod.Name, output.UpdatePolicy, output.ValueType, output.ProtoType, ioFactory,
			state.WithPartialMode(p.partialMode),
		)

		if forceLoadState {
			// Use AN ABSOLUTE store, or SQUASH ALL PARTIAL!
			if err := store.Init(p.startBlockNum); err != nil {
				return fmt.Errorf("could not load state for store %s at block num %d: %w", mod.Name, p.startBlockNum, err)
			}
		}
		p.stores[mod.Name] = store
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

func (p *Pipeline) HandlerFactory(blockCount uint64) bstream.Handler {
	initFunc := sync.Once{}

	return bstream.HandlerFunc(func(block *bstream.Block, obj interface{}) (err error) {
		initFunc.Do(func() {
			// All blocks will be processed in this goroutine
			runtime.LockOSThread()
			p.initFunc()
		})

		// defer func() {
		// 	if r := recover(); r != nil {
		// 		err = fmt.Errorf("panic: %w", r)
		// 	}
		// }()

		// TODO: eventually, handle the `undo` signals.
		//  NOTE: The RUNTIME will handle the undo signals. It'll have all it needs.
		if block.Number >= p.startBlockNum+blockCount {
			for _, s := range p.stores {
				s.WriteState(context.Background(), block)
			}

			p.rpcCache.Save(context.Background())

			return io.EOF
		}

		p.nativeImports.SetCurrentBlock(block)
		//clock := toClock(block)

		blk := block.ToProtocol()
		switch p.vmType {
		case "native":
			p.nativeOutputs[p.blockType /*"sf.ethereum.type.v1.Block" */ ] = reflect.ValueOf(blk)
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
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		for _, streamFunc := range p.streamFuncs {
			if err := streamFunc(); err != nil {
				return err
			}
		}

		// Prep for next block, clean-up all deltas.
		for _, s := range p.stores {
			s.Flush()
		}

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
		return fmt.Errorf("invalid number of outputs for Map call in code for module %q, should be 2 (data, error)", name)
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
	protoDecode func(in []byte) (string, error),
	msgType string,
) (err error) {

	var vm *wasm.Instance
	if vm, err = wasmCall(vals, mod, entrypoint, name, inputs, rpcFactory); err != nil {
		return err
	}
	if vm != nil {
		out := vm.Output()
		vals[name] = out
		if len(out) != 0 && printOutputs {
			printed := false
			if protoDecode != nil {
				js, err := protoDecode(out)
				if err != nil {
					fmt.Printf("WARN: Error encoding protobuf module %q's output: %s\n", name, err)
				} else {
					fmt.Printf("Module %q output for type %q:\n%s\n", name, msgType, js)
					printed = true
				}
			}
			if !printed {
				fmt.Printf("Module %q output for type %q:\n    echo %q | base64 -d | protoc -I ./proto proto/*proto --decode=%s\n", name, msgType, base64.StdEncoding.EncodeToString(out), msgType)
			}
		}
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
	rpcFactory wasm.WasmerFunctionFactory) (out *wasm.Instance, err error) {

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
		out, err = mod.NewInstance(entrypoint, inputs, rpcFactory)
		if err != nil {
			return nil, fmt.Errorf("new wasm instance: %w", err)
		}
		if err = out.Execute(); err != nil {
			return nil, fmt.Errorf("module %q: wasm execution failed: %w", name, err)
		}
	}
	return
}

package pipeline

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"

	"github.com/streamingfast/bstream"
	"github.com/streamingfast/eth-go/rpc"
	"github.com/streamingfast/substreams/manifest"
	imports "github.com/streamingfast/substreams/native-imports"
	"github.com/streamingfast/substreams/registry"
	ssrpc "github.com/streamingfast/substreams/rpc"
	"github.com/streamingfast/substreams/state"
	"github.com/streamingfast/substreams/wasm"
	"google.golang.org/protobuf/proto"
)

type Pipeline struct {
	vmType        string // wasm, native
	startBlockNum uint64

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

func New(startBlockNum uint64, rpcClient *rpc.Client, rpcCache *ssrpc.Cache, manif *manifest.Manifest, outputStreamName string, blockType string) *Pipeline {
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
	}
	// pipe.setupSubscriptionHub()
	// pipe.setupPrintPairUpdates()
	return pipe
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

	for _, mod := range modules {
		debugOutput := mod.Name == p.outputStreamName
		var inputs []*wasm.Input
		for _, in := range mod.Inputs {
			if in.Map != "" {
				inputs = append(inputs, &wasm.Input{
					Type: wasm.InputStream,
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
					Type: wasm.InputStream,
					Name: in.Source,
				})
			} else {
				return fmt.Errorf("invalid input struct for stream %q", mod.Name)
			}
		}
		streamName := mod.Name // to ensure it's enclosed

		wasmModule, err := wasm.NewModule(mod.Code.Content, filepath.Base(mod.Code.File))
		if err != nil {
			return fmt.Errorf("new wasm module: %w", err)
		}

		switch mod.Kind {
		case "map":
			fmt.Printf("Adding mapper for stream %q\n", streamName)
			entrypoint := mod.Code.Entrypoint
			p.streamFuncs = append(p.streamFuncs, func() error {
				return wasmMapCall(p.wasmOutputs, wasmModule, entrypoint, streamName, inputs, debugOutput)
			})
		case "store":
			updatePolicy := mod.Output.UpdatePolicy
			valueType := mod.Output.ValueType
			protoType := mod.Output.ProtoType

			entrypoint := mod.Code.Entrypoint
			inputs = append(inputs, &wasm.Input{
				Type:         wasm.OutputStore,
				Name:         streamName,
				Store:        p.stores[streamName],
				UpdatePolicy: updatePolicy,
				ValueType:    valueType,
				ProtoType:    protoType,
			})
			fmt.Printf("Adding state builder for stream %q\n", streamName)

			p.streamFuncs = append(p.streamFuncs, func() error {
				return wasmStoreCall(p.wasmOutputs, wasmModule, entrypoint, streamName, inputs, debugOutput)
			})

		default:
			return fmt.Errorf("unknown value %q for 'kind' in stream %q", mod.Kind, mod.Name)
		}

	}

	return nil
}

func (p *Pipeline) setupStores(modules []*manifest.Module, ioFactory state.IOFactory, forceLoadState bool) error {
	p.stores = make(map[string]*state.Builder)
	for _, mod := range modules {
		if mod.Kind != "store" {
			continue
		}
		output := mod.Output
		store := state.NewBuilder(mod.Name, output.UpdatePolicy, output.ValueType, output.ProtoType, ioFactory)
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
	return bstream.HandlerFunc(func(block *bstream.Block, obj interface{}) (err error) {
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

		blk := block.ToProtocol()
		switch p.vmType {
		case "native":
			p.nativeOutputs[p.blockType /*"sf.ethereum.codec.v1.Block" */] = reflect.ValueOf(blk)
		case "wasm/rust-v1":
			// block.Payload.Get() could do the same, but does it go through the same
			// CORRECTIONS of the block, that the BlockDecoder does?
			blkBytes, err := proto.Marshal(blk.(proto.Message))
			if err != nil {
				return fmt.Errorf("packing block: %w", err)
			}

			p.wasmOutputs[p.blockType] = blkBytes
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

		// Prep for next block, clean-up all deltas. This ought to be
		// done by the runtime, when doing clean-up between blocks.
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

func wasmMapCall(vals map[string][]byte, mod *wasm.Module, entrypoint string, name string, inputs []*wasm.Input, printOutputs bool) (err error) {
	var vm *wasm.Instance
	if vm, err = wasmCall(vals, mod, entrypoint, name, inputs); err != nil {
		return err
	}
	if vm != nil {
		out := vm.Output()
		vals[name] = out
		if len(out) != 0 && printOutputs {
			fmt.Printf("Module output %q:\n    %v\n", name, out)
		}
	} else {
		vals[name] = nil
	}
	return nil
}

func wasmStoreCall(vals map[string][]byte, mod *wasm.Module, entrypoint string, name string, inputs []*wasm.Input, printOutputs bool) (err error) {
	var vm *wasm.Instance
	if vm, err = wasmCall(vals, mod, entrypoint, name, inputs); err != nil {
		return err
	}
	if vm != nil && printOutputs {
		vm.PrintDeltas()
	}
	return nil
}

func wasmCall(vals map[string][]byte, mod *wasm.Module, entrypoint string, name string, inputs []*wasm.Input) (out *wasm.Instance, err error) {
	hasInput := false
	for _, input := range inputs {
		switch input.Type {
		case wasm.InputStream:
			val := vals[input.Name]
			if len(val) != 0 {
				input.StreamData = val
				hasInput = true
			}
		case wasm.InputStore:
			hasInput = true
		case wasm.OutputStore:
		}
	}

	// This allows us to skip the execution of the VM if there are no inputs.
	// This assumption should either be configurable by the manifest, or clearly documented:
	//  state builders will not be called if their input streams are 0 bytes length (and there's no
	//  state store in read mode)
	if hasInput {
		out, err = mod.NewInstance(entrypoint, inputs)
		if err != nil {
			return nil, fmt.Errorf("new wasm instance: %w", err)
		}
		if err = out.Execute(); err != nil {
			return nil, fmt.Errorf("stream %s: wasm execution failed: %w", name, err)
		}
	}
	return
}

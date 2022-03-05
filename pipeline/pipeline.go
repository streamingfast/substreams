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
	streams, err := p.manifest.Graph.StreamsFor(p.outputStreamName)
	if err != nil {
		return fmt.Errorf("whoops: %w", err)
	}

	nativeStreams := registry.Init(p.nativeImports)

	if err := p.setupStores(streams, ioFactory, forceLoadState); err != nil {
		return fmt.Errorf("setting up stores: %w", err)
	}
	p.nativeOutputs = map[string]reflect.Value{}

	for _, stream := range streams {
		f, found := nativeStreams[stream.Code.Native]
		if !found {
			return fmt.Errorf("native code not found for %q", stream.Code)
		}

		debugOutput := stream.Name == p.outputStreamName
		inputs := []string{}
		for _, in := range stream.Inputs {
			inputs = append(inputs, strings.Split(in, ":")[1])
		}
		streamName := stream.Name // to ensure it's enclosed

		switch stream.Kind {
		case "Mapper":
			method := f.MethodByName("Map")
			if method.Kind() == reflect.Invalid {
				return fmt.Errorf("Map() method not found on %T", f.Interface())
			}
			if method.IsZero() {
				return fmt.Errorf("Map() method not found on %T", f.Interface())
			}
			fmt.Printf("Adding mapper for stream %q\n", stream.Name)
			p.streamFuncs = append(p.streamFuncs, func() error {
				return nativeMapper(p.nativeOutputs, method, streamName, inputs, debugOutput)
			})
		case "StateBuilder":
			method := f.MethodByName("BuildState")
			if method.Kind() == reflect.Invalid {
				return fmt.Errorf("BuildState() method not found on %T", f.Interface())
			}
			if method.IsZero() {
				return fmt.Errorf("BuildState() method not found on %T", f.Interface())
			}

			p.nativeOutputs[stream.Name] = reflect.ValueOf(p.stores[stream.Name])

			fmt.Printf("Adding state builder for stream %q\n", stream.Name)
			p.streamFuncs = append(p.streamFuncs, func() error {
				return nativeStateBuilder(p.nativeOutputs, method, streamName, inputs, debugOutput)
			})

		default:
			return fmt.Errorf("unknown value %q for 'kind' in stream %q", stream.Kind, stream.Name)
		}

	}

	p.vmType = "native"

	return nil
}

func (p *Pipeline) BuildWASM(ioFactory state.IOFactory, forceLoadState bool) error {
	streams, err := p.manifest.Graph.StreamsFor(p.outputStreamName)
	if err != nil {
		return fmt.Errorf("building execution graph: %w", err)
	}

	if err := p.setupStores(streams, ioFactory, forceLoadState); err != nil {
		return fmt.Errorf("setting up stores: %w", err)
	}

	p.wasmOutputs = map[string][]byte{}

	for _, stream := range streams {
		debugOutput := stream.Name == p.outputStreamName
		var inputs []*wasm.Input
		for _, in := range stream.Inputs {
			streamInput := manifest.StreamInput(in)
			inputKind, inputName, err := streamInput.Parse()
			if err != nil {
				return err
			}

			switch inputKind {
			case "stream":
				inputs = append(inputs, &wasm.Input{
					Type: wasm.InputStream,
					Name: inputName,
				})
			case "store":
				inputs = append(inputs, &wasm.Input{
					Type:  wasm.InputStore,
					Name:  inputName,
					Store: p.stores[inputName],
				})
			case "proto":
				inputs = append(inputs, &wasm.Input{
					Type: wasm.InputStream,
					Name: inputName,
				})
			default:
				return fmt.Errorf("invalid input type %q for stream %q in input %q", inputKind, stream.Name, in)
			}
		}
		streamName := stream.Name // to ensure it's enclosed

		mod, err := wasm.NewModule(stream.Code.Content, filepath.Base(stream.Code.File))
		if err != nil {
			return fmt.Errorf("new wasm module: %w", err)
		}

		switch stream.Kind {
		case "Mapper":
			fmt.Printf("Adding mapper for stream %q\n", streamName)
			entrypoint := stream.Code.Entrypoint
			p.streamFuncs = append(p.streamFuncs, func() error {
				return wasmMapper(p.wasmOutputs, mod, entrypoint, streamName, inputs, debugOutput)
			})
		case "StateBuilder":
			updatePolicy := stream.Output.UpdatePolicy
			valueType := stream.Output.ValueType
			protoType := stream.Output.ProtoType

			entrypoint := stream.Code.Entrypoint
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
				return wasmStateBuilder(p.wasmOutputs, mod, entrypoint, streamName, inputs, debugOutput)
			})

		default:
			return fmt.Errorf("unknown value %q for 'kind' in stream %q", stream.Kind, stream.Name)
		}

	}

	return nil
}

func (p *Pipeline) setupStores(streams []*manifest.Stream, ioFactory state.IOFactory, forceLoadState bool) error {
	p.stores = make(map[string]*state.Builder)
	for _, s := range streams {
		if s.Kind != "StateBuilder" {
			continue
		}
		output := s.Output
		store := state.NewBuilder(s.Name, output.UpdatePolicy, output.ValueType, output.ProtoType, ioFactory)
		if forceLoadState {
			// Use AN ABSOLUTE store, or SQUASH ALL PARTIAL!

			if err := store.Init(p.startBlockNum); err != nil {
				return fmt.Errorf("could not load state for store %s at block num %d: %w", s.Name, p.startBlockNum, err)
			}
		}
		p.stores[s.Name] = store
	}
	return nil
}

// `stateBuilder` aura 4 modes d'opération:
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

func nativeMapper(vals map[string]reflect.Value, method reflect.Value, name string, inputs []string, printOutputs bool) error {
	var inputVals []reflect.Value
	for _, in := range inputs {
		inputVals = append(inputVals, vals[in])
	}
	out := method.Call(inputVals)
	if len(out) != 2 {
		return fmt.Errorf("invalid number of outputs for Map call in code for stream %q, should be 2 (data, error)", name)
	}
	vals[name] = out[0]

	p, ok := out[0].Interface().(Printer)
	if ok && printOutputs {
		p.Print()
	}

	if err, ok := out[1].Interface().(error); ok && err != nil {
		return fmt.Errorf("mapper stream %q: %w", name, err)
	}
	return nil
}

func nativeStateBuilder(vals map[string]reflect.Value, method reflect.Value, name string, inputs []string, printOutputs bool) error {
	var inputVals []reflect.Value
	for _, in := range inputs {
		inputVals = append(inputVals, vals[in])
	}
	inputVals = append(inputVals, vals[name])

	// TODO: we can cache the `Method` retrieved on the stream.
	out := method.Call(inputVals)
	if len(out) != 1 {
		return fmt.Errorf("invalid number of outputs for BuildState call in code for stream %q, should be 1 (error)", name)
	}
	p, ok := vals[name].Interface().(Printer)
	if ok && printOutputs {
		p.Print()
	}
	if err, ok := out[0].Interface().(error); ok && err != nil {
		return fmt.Errorf("state builder stream %q: %w", name, err)
	}
	return nil
}

func wasmMapper(vals map[string][]byte, mod *wasm.Module, entrypoint string, name string, inputs []*wasm.Input, printOutputs bool) (err error) {
	var vm *wasm.Instance
	if vm, err = wasmCall(vals, mod, entrypoint, name, inputs); err != nil {
		return err
	}
	if vm != nil {
		out := vm.Output()
		vals[name] = out
		if len(out) != 0 && printOutputs {
			fmt.Printf("Stream output %q:\n    %v\n", name, out)
		}
	} else {
		vals[name] = nil
	}
	return nil
}

func wasmStateBuilder(vals map[string][]byte, mod *wasm.Module, entrypoint string, name string, inputs []*wasm.Input, printOutputs bool) (err error) {
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

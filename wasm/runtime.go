package wasm

import (
	"fmt"

	"github.com/streamingfast/substreams/state"
	"github.com/wasmerio/wasmer-go/wasmer"
)

type Instance struct {
	module     *Module
	wasmStore  *wasmer.Store
	memory     *wasmer.Memory
	heap       *Heap
	entrypoint *wasmer.Function

	inputStores []state.Reader
	outputStore *state.Builder

	returnValue []byte
	panicError  *PanicError
}

type Module struct {
	engine *wasmer.Engine
	store  *wasmer.Store
	module *wasmer.Module
}

func NewModule(wasmCode []byte) (*Module, error) {
	engine := wasmer.NewUniversalEngine()
	store := wasmer.NewStore(engine)

	module, err := wasmer.NewModule(store, wasmCode)
	if err != nil {
		return nil, fmt.Errorf("loading wasm module: %w", err)
	}

	return &Module{
		engine: engine,
		store:  store,
		module: module,
	}, nil
}

func (m *Module) NewInstance(functionName string) (*Instance, error) {
	// WARN: An instance needs to be created on the same thread that it is consumed.
	instance := &Instance{
		wasmStore: m.store,
		module:    m,
	}
	imports := instance.newImports()
	vmInstance, err := wasmer.NewInstance(m.module, imports)
	if err != nil {
		return nil, fmt.Errorf("creating instance: %w", err)
	}

	memory, err := vmInstance.Exports.GetMemory("memory")
	if err != nil {
		return nil, fmt.Errorf("getting module memory: %w", err)
	}

	alloc, err := vmInstance.Exports.GetFunction("alloc")
	if err != nil {
		return nil, fmt.Errorf("getting alloc function: %w", err)
	}

	instance.memory = memory
	instance.heap = NewHeap(memory, alloc)
	instance.entrypoint, err = vmInstance.Exports.GetRawFunction(functionName)
	if err != nil {
		return nil, fmt.Errorf("getting wasm module function %q: %w", functionName, err)
	}

	return instance, nil
}

func (i *Instance) newImports() *wasmer.ImportObject {
	imports := wasmer.NewImportObject()
	imports.Register("env", map[string]wasmer.IntoExtern{
		"register_panic": wasmer.NewFunction(
			i.wasmStore,
			wasmer.NewFunctionType(
				params(wasmer.I32, wasmer.I32, wasmer.I32, wasmer.I32, wasmer.I32, wasmer.I32),
				returns(),
			),
			func(args []wasmer.Value) ([]wasmer.Value, error) {
				message, err := i.heap.ReadString(args[0].I32(), args[1].I32())
				if err != nil {
					return nil, fmt.Errorf("read message argument: %w", err)
				}

				var filename string
				filenamePtr := args[2].I32()
				if filenamePtr != 0 {
					filename, err = i.heap.ReadString(args[2].I32(), args[3].I32())
					if err != nil {
						return nil, fmt.Errorf("read filename argument: %w", err)
					}
				}

				lineNumber := int(args[4].I32())
				columnNumber := int(args[5].I32())

				i.panicError = &PanicError{message, filename, lineNumber, columnNumber}
				//fmt.Println(i.panicError.Error())

				return nil, i.panicError
			},
		),
		"println": wasmer.NewFunction(
			i.wasmStore,
			wasmer.NewFunctionType(
				params(wasmer.I32, wasmer.I32),
				returns(),
			),
			func(args []wasmer.Value) ([]wasmer.Value, error) {
				message, err := i.heap.ReadString(args[0].I32(), args[1].I32())
				if err != nil {
					return nil, fmt.Errorf("reading string: %w", err)
				}

				fmt.Println(message)

				return nil, nil
			},
		),
		"output": wasmer.NewFunction(
			i.wasmStore,
			wasmer.NewFunctionType(
				params(wasmer.I32, wasmer.I32),
				returns(),
			),
			func(args []wasmer.Value) ([]wasmer.Value, error) {
				message, err := i.heap.ReadBytes(args[0].I32(), args[1].I32())
				if err != nil {
					return nil, fmt.Errorf("reading bytes: %w", err)
				}

				i.returnValue = message

				return nil, nil
			},
		),
		"state_set": wasmer.NewFunction(
			i.wasmStore,
			wasmer.NewFunctionType(
				params(wasmer.I64, wasmer.I32, wasmer.I32, wasmer.I32, wasmer.I32),
				returns(),
			),
			func(args []wasmer.Value) ([]wasmer.Value, error) {
				ord := args[0].I64()
				key, err := i.heap.ReadString(args[1].I32(), args[2].I32())
				if err != nil {
					return nil, fmt.Errorf("reading string: %w", err)
				}
				value, err := i.heap.ReadBytes(args[3].I32(), args[4].I32())
				if err != nil {
					return nil, fmt.Errorf("reading bytes: %w", err)
				}

				i.outputStore.SetBytes(uint64(ord), key, value)

				return nil, nil
			},
		),
		"state_get_at": wasmer.NewFunction(
			i.wasmStore,
			wasmer.NewFunctionType(
				params(wasmer.I32 /* store index */, wasmer.I64 /* ordinal */, wasmer.I32, wasmer.I32 /* key */),
				returns(wasmer.I32, wasmer.I32, wasmer.I32),
			),
			func(args []wasmer.Value) ([]wasmer.Value, error) {
				readStore := i.inputStores[int(args[0].I32())]
				ord := args[1].I64()
				key, err := i.heap.ReadString(args[2].I32(), args[3].I32())
				if err != nil {
					return nil, fmt.Errorf("reading string: %w", err)
				}

				val, found := readStore.GetAt(uint64(ord), key)
				if !found {
					zero := wasmer.NewI32(0)
					return []wasmer.Value{zero, zero, zero}, nil
				} else {
					ptr, err := i.heap.Write(val)
					if err != nil {
						return nil, err
					}

					return []wasmer.Value{wasmer.NewI32(ptr), wasmer.NewI32(len(val)), wasmer.NewI32(1)}, nil
				}
			},
		),
		"state_get_first": wasmer.NewFunction(
			i.wasmStore,
			wasmer.NewFunctionType(
				params(wasmer.I32 /* store index */, wasmer.I32, wasmer.I32 /* key */),
				returns(wasmer.I32, wasmer.I32, wasmer.I32),
			),
			func(args []wasmer.Value) ([]wasmer.Value, error) {
				readStore := i.inputStores[int(args[0].I32())]
				key, err := i.heap.ReadString(args[1].I32(), args[2].I32())
				if err != nil {
					return nil, fmt.Errorf("reading string: %w", err)
				}

				val, found := readStore.GetFirst(key)
				if !found {
					zero := wasmer.NewI32(0)
					return []wasmer.Value{zero, zero, zero}, nil
				} else {
					ptr, err := i.heap.Write(val)
					if err != nil {
						return nil, err
					}

					return []wasmer.Value{wasmer.NewI32(ptr), wasmer.NewI32(len(val)), wasmer.NewI32(1)}, nil
				}
			},
		),
		"state_get_last": wasmer.NewFunction(
			i.wasmStore,
			wasmer.NewFunctionType(
				params(wasmer.I32 /* store index */, wasmer.I32, wasmer.I32 /* key */),
				returns(wasmer.I32, wasmer.I32, wasmer.I32),
			),
			func(args []wasmer.Value) ([]wasmer.Value, error) {
				readStore := i.inputStores[int(args[0].I32())]
				key, err := i.heap.ReadString(args[1].I32(), args[2].I32())
				if err != nil {
					return nil, fmt.Errorf("reading string: %w", err)
				}

				val, found := readStore.GetLast(key)
				if !found {
					zero := wasmer.NewI32(0)
					return []wasmer.Value{zero, zero, zero}, nil
				} else {
					ptr, err := i.heap.Write(val)
					if err != nil {
						return nil, err
					}

					return []wasmer.Value{wasmer.NewI32(ptr), wasmer.NewI32(len(val)), wasmer.NewI32(1)}, nil
				}
			},
		),
	})
	return imports
}

func (i *Instance) Execute(inputs []*Input) (err error) {
	i.returnValue = nil
	i.panicError = nil

	var args []interface{}
	for _, input := range inputs {
		switch input.Type {
		case InputStream:
			ptr, err := i.heap.Write(input.StreamData)
			if err != nil {
				return fmt.Errorf("writing %q to heap: %w", input.Name, err)
			}
			len := int32(len(input.StreamData))
			args = append(args, ptr, len)
		case InputStore:
			i.inputStores = append(i.inputStores, input.Store)
			args = append(args, len(i.inputStores)-1)
		case OutputStore:
			i.outputStore = input.Store
		}
	}
	fmt.Println("")

	_, err = i.entrypoint.Call(args...)
	return
}

func (i *Instance) Err() error {
	return i.panicError
}

func (i *Instance) Output() []byte {
	return i.returnValue
}

func (i *Instance) PrintDeltas() {
	if i.outputStore == nil {
		return
	}

	i.outputStore.Print()
}

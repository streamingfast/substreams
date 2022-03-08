package wasm

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math/big"

	"github.com/streamingfast/substreams/state"
	"github.com/wasmerio/wasmer-go/wasmer"
	"go.uber.org/zap"
)

type Instance struct {
	module    *Module
	wasmStore *wasmer.Store
	memory    *wasmer.Memory
	heap      *Heap

	inputStores  []state.Reader
	outputStore  *state.Builder
	updatePolicy string
	valueType    string

	entrypoint *wasmer.Function
	args       []interface{} // to the `entrypoint` function

	returnValue  []byte
	panicError   *PanicError
	functionName string
}

type Module struct {
	engine *wasmer.Engine
	store  *wasmer.Store
	module *wasmer.Module
	name   string
}

func NewModule(wasmCode []byte, name string) (*Module, error) {
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
		name:   name,
	}, nil
}

func (m *Module) NewInstance(functionName string, inputs []*Input) (*Instance, error) {
	// WARN: An instance needs to be created on the same thread that it is consumed.
	instance := &Instance{
		wasmStore:    m.store,
		module:       m,
		functionName: functionName,
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

	var args []interface{}
	for _, input := range inputs {
		switch input.Type {
		case InputStream:
			ptr, err := instance.heap.Write(input.StreamData)
			if err != nil {
				return nil, fmt.Errorf("writing %q to heap: %w", input.Name, err)
			}
			len := int32(len(input.StreamData))
			args = append(args, ptr, len)
		case InputStore:
			if input.Deltas {
				// TODO: Make it a proto thing before sending in
				cnt, _ := json.Marshal(input.Store.Deltas)

				ptr, err := instance.heap.Write(cnt)
				if err != nil {
					return nil, fmt.Errorf("writing %q (deltas=%v) to heap: %w", input.Name, input.Deltas, err)
				}

				args = append(args, ptr, int32(len(cnt)))
			} else {
				instance.inputStores = append(instance.inputStores, input.Store)
				args = append(args, len(instance.inputStores)-1)
			}
		case OutputStore:
			instance.outputStore = input.Store
			instance.updatePolicy = input.UpdatePolicy
			instance.valueType = input.ValueType
		}
	}
	instance.args = args

	return instance, nil
}

func (i *Instance) Execute() (err error) {
	_, err = i.entrypoint.Call(i.args...)
	return
}

func (i *Instance) newImports() *wasmer.ImportObject {
	imports := wasmer.NewImportObject()

	i.registerLoggerImports(imports)
	i.registerStateImports(imports)

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
	})
	return imports
}

func (i *Instance) registerLoggerImports(imports *wasmer.ImportObject) {
	imports.Register("logger", map[string]wasmer.IntoExtern{
		"debug": wasmer.NewFunction(
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
				zlog.Debug(message, zap.String("function_name", i.functionName), zap.String("wasm_file", i.module.name))
				return nil, nil
			},
		),
		"info": wasmer.NewFunction(
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
				zlog.Info(message, zap.String("function_name", i.functionName), zap.String("wasm_file", i.module.name))

				return nil, nil
			},
		),
	})
}
func (i *Instance) registerStateImports(imports *wasmer.ImportObject) {
	functions := map[string]wasmer.IntoExtern{}
	functions["set"] = wasmer.NewFunction(
		i.wasmStore,
		wasmer.NewFunctionType(
			params(wasmer.I64, wasmer.I32, wasmer.I32, wasmer.I32, wasmer.I32),
			returns(),
		),
		func(args []wasmer.Value) ([]wasmer.Value, error) {
			if i.outputStore == nil && i.updatePolicy != "replace" {
				return nil, fmt.Errorf("invalid store operation: 'set' only valid for stores with updatePolicy == 'replace'")
			}
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
	)

	functions["set_if_not_exists"] = wasmer.NewFunction(
		i.wasmStore,
		wasmer.NewFunctionType(
			params(wasmer.I64, wasmer.I32, wasmer.I32, wasmer.I32, wasmer.I32),
			returns(),
		),
		func(args []wasmer.Value) ([]wasmer.Value, error) {
			if i.outputStore == nil && i.updatePolicy != "ignore" {
				return nil, fmt.Errorf("invalid store operation: 'set_if_not_exists' only valid for stores with updatePolicy == 'ignore'")
			}
			ord := args[0].I64()
			key, err := i.heap.ReadString(args[1].I32(), args[2].I32())
			if err != nil {
				return nil, fmt.Errorf("reading string: %w", err)
			}
			value, err := i.heap.ReadBytes(args[3].I32(), args[4].I32())
			if err != nil {
				return nil, fmt.Errorf("reading bytes: %w", err)
			}

			i.outputStore.SetBytesIfNotExists(uint64(ord), key, value)

			return nil, nil
		},
	)

	functions["sum_bigint"] = wasmer.NewFunction(
		i.wasmStore,
		wasmer.NewFunctionType(
			params(wasmer.I64 /* ordinal */, wasmer.I32, wasmer.I32 /* key */, wasmer.I32, wasmer.I32 /* value */),
			returns(),
		),
		func(args []wasmer.Value) ([]wasmer.Value, error) {
			if i.outputStore == nil && i.updatePolicy != "sum" && i.valueType != "bigfloat" {
				return nil, fmt.Errorf("invalid store operation: 'sum_bigfloat' only valid for stores with updatePolicy == 'sum' and valueType == 'bigfloat'")
			}
			ord := args[0].I64()
			key, err := i.heap.ReadString(args[1].I32(), args[2].I32())
			if err != nil {
				return nil, fmt.Errorf("reading string: %w", err)
			}
			value, err := i.heap.ReadString(args[3].I32(), args[4].I32())
			if err != nil {
				return nil, fmt.Errorf("reading bytes: %w", err)
			}

			toAdd, _, err := big.ParseFloat(value, 10, 100, big.ToNearestEven) // corresponds to SumBigFloat's read of the kv value
			if err != nil {
				return nil, fmt.Errorf("parsing bigfloat value %q: %w", value, err)
			}

			i.outputStore.SumBigFloat(uint64(ord), key, toAdd)

			return nil, nil
		},
	)

	functions["sum_bigint"] = wasmer.NewFunction(
		i.wasmStore,
		wasmer.NewFunctionType(
			params(wasmer.I64 /* ordinal */, wasmer.I32, wasmer.I32 /* key */, wasmer.I32, wasmer.I32 /* value */),
			returns(),
		),
		func(args []wasmer.Value) ([]wasmer.Value, error) {
			if i.outputStore == nil && i.updatePolicy != "sum" && i.valueType != "bigint" {
				return nil, fmt.Errorf("invalid store operation: 'sum_bigint' only valid for stores with updatePolicy == 'sum' and valueType == 'bigint'")
			}
			ord := args[0].I64()
			key, err := i.heap.ReadString(args[1].I32(), args[2].I32())
			if err != nil {
				return nil, fmt.Errorf("reading string: %w", err)
			}
			value, err := i.heap.ReadString(args[3].I32(), args[4].I32())
			if err != nil {
				return nil, fmt.Errorf("reading bytes: %w", err)
			}

			toAdd, _ := new(big.Int).SetString(value, 10)
			i.outputStore.SumBigInt(uint64(ord), key, toAdd)

			return nil, nil
		},
	)

	functions["sum_int64"] = wasmer.NewFunction(
		i.wasmStore,
		wasmer.NewFunctionType(
			params(wasmer.I64 /* ordinal */, wasmer.I32, wasmer.I32 /* key */, wasmer.I64 /* value */),
			returns(),
		),
		func(args []wasmer.Value) ([]wasmer.Value, error) {
			if i.outputStore == nil && i.updatePolicy != "sum" && i.valueType != "int64" {
				return nil, fmt.Errorf("invalid store operation: 'sum_bigint' only valid for stores with updatePolicy == 'sum' and valueType == 'int64'")
			}
			ord := args[0].I64()
			key, err := i.heap.ReadString(args[1].I32(), args[2].I32())
			if err != nil {
				return nil, fmt.Errorf("reading string: %w", err)
			}
			value := args[3].I64()

			i.outputStore.SumInt64(uint64(ord), key, value)

			return nil, nil
		},
	)

	functions["sum_float64"] = wasmer.NewFunction(
		i.wasmStore,
		wasmer.NewFunctionType(
			params(wasmer.I64 /* ordinal */, wasmer.I32, wasmer.I32 /* key */, wasmer.F64 /* value */),
			returns(),
		),
		func(args []wasmer.Value) ([]wasmer.Value, error) {
			if i.outputStore == nil && i.updatePolicy != "sum" && i.valueType != "float64" {
				return nil, fmt.Errorf("invalid store operation: 'sum_float64' only valid for stores with updatePolicy == 'sum' and valueType == 'float64'")
			}
			ord := args[0].I64()
			key, err := i.heap.ReadString(args[1].I32(), args[2].I32())
			if err != nil {
				return nil, fmt.Errorf("reading string: %w", err)
			}

			value := args[3].F64()
			if err != nil {
				return nil, fmt.Errorf("reading bytes: %w", err)
			}
			i.outputStore.SumFloat64(uint64(ord), key, value)

			return nil, nil
		},
	)

	functions["sum_bigfloat"] = wasmer.NewFunction(
		i.wasmStore,
		wasmer.NewFunctionType(
			params(wasmer.I64 /* ordinal */, wasmer.I32, wasmer.I32 /* key */, wasmer.I32, wasmer.I32 /* value */),
			returns(),
		),
		func(args []wasmer.Value) ([]wasmer.Value, error) {
			if i.outputStore == nil && i.updatePolicy != "sum" && i.valueType != "bigfloat" {
				return nil, fmt.Errorf("invalid store operation: 'sum_bigfloat' only valid for stores with updatePolicy == 'sum' and valueType == 'bigfloat'")
			}
			ord := args[0].I64()
			key, err := i.heap.ReadString(args[1].I32(), args[2].I32())
			if err != nil {
				return nil, fmt.Errorf("reading string: %w", err)
			}
			value, err := i.heap.ReadString(args[3].I32(), args[4].I32())
			if err != nil {
				return nil, fmt.Errorf("reading bytes: %w", err)
			}

			toAdd, _, err := big.ParseFloat(value, 10, 100, big.ToNearestEven)
			i.outputStore.SumBigFloat(uint64(ord), key, toAdd)

			return nil, nil
		},
	)

	functions["set_min_int64"] = wasmer.NewFunction(
		i.wasmStore,
		wasmer.NewFunctionType(
			params(wasmer.I64 /* ordinal */, wasmer.I32, wasmer.I32 /* key */, wasmer.I64 /* value */),
			returns(),
		),
		func(args []wasmer.Value) ([]wasmer.Value, error) {
			if i.outputStore == nil && i.updatePolicy != "min" && i.valueType != "int64" {
				return nil, fmt.Errorf("invalid store operation: 'set_min_int64' only valid for stores with updatePolicy == 'min' and valueType == 'int64'")
			}
			ord := args[0].I64()
			key, err := i.heap.ReadString(args[1].I32(), args[2].I32())
			if err != nil {
				return nil, fmt.Errorf("reading string: %w", err)
			}
			value := args[3].I64()

			i.outputStore.SetMinInt64(uint64(ord), key, value)

			return nil, nil
		},
	)

	functions["set_min_bigint"] = wasmer.NewFunction(
		i.wasmStore,
		wasmer.NewFunctionType(
			params(wasmer.I64 /* ordinal */, wasmer.I32, wasmer.I32 /* key */, wasmer.I32, wasmer.I32 /* value */),
			returns(),
		),
		func(args []wasmer.Value) ([]wasmer.Value, error) {
			if i.outputStore == nil && i.updatePolicy != "min" && i.valueType != "bigfloat" {
				return nil, fmt.Errorf("invalid store operation: 'set_min_bigint' only valid for stores with updatePolicy == 'min' and valueType == 'bigint'")
			}
			ord := args[0].I64()
			key, err := i.heap.ReadString(args[1].I32(), args[2].I32())
			if err != nil {
				return nil, fmt.Errorf("reading string: %w", err)
			}
			value, err := i.heap.ReadString(args[3].I32(), args[4].I32())
			if err != nil {
				return nil, fmt.Errorf("reading bytes: %w", err)
			}

			toSet, _ := new(big.Int).SetString(value, 10)
			i.outputStore.SetMinBigInt(uint64(ord), key, toSet)

			return nil, nil
		},
	)
	functions["set_min_float64"] = wasmer.NewFunction(
		i.wasmStore,
		wasmer.NewFunctionType(
			params(wasmer.I64 /* ordinal */, wasmer.I32, wasmer.I32 /* key */, wasmer.F64 /* value */),
			returns(),
		),
		func(args []wasmer.Value) ([]wasmer.Value, error) {
			if i.outputStore == nil && i.updatePolicy != "min" && i.valueType != "float" {
				return nil, fmt.Errorf("invalid store operation: 'set_min_float64' only valid for stores with updatePolicy == 'min' and valueType == 'int64'")
			}
			ord := args[0].I64()
			key, err := i.heap.ReadString(args[1].I32(), args[2].I32())
			if err != nil {
				return nil, fmt.Errorf("reading string: %w", err)
			}
			value := args[3].F64()
			fmt.Println("parse float", value)
			i.outputStore.SetMinFloat64(uint64(ord), key, value)

			return nil, nil
		},
	)

	functions["set_min_bigfloat"] = wasmer.NewFunction(
		i.wasmStore,
		wasmer.NewFunctionType(
			params(wasmer.I64 /* ordinal */, wasmer.I32, wasmer.I32 /* key */, wasmer.I32, wasmer.I32 /* value */),
			returns(),
		),
		func(args []wasmer.Value) ([]wasmer.Value, error) {
			if i.outputStore == nil && i.updatePolicy != "min" && i.valueType != "bigint" {
				return nil, fmt.Errorf("invalid store operation: 'set_min_bigfloat' only valid for stores with updatePolicy == 'min' and valueType == 'bigint'")
			}
			ord := args[0].I64()
			key, err := i.heap.ReadString(args[1].I32(), args[2].I32())
			if err != nil {
				return nil, fmt.Errorf("reading string: %w", err)
			}
			value, err := i.heap.ReadString(args[3].I32(), args[4].I32())
			if err != nil {
				return nil, fmt.Errorf("reading bytes: %w", err)
			}

			toSet, _, err := big.ParseFloat(value, 10, 100, big.ToNearestEven)
			i.outputStore.SetMinBigFloat(uint64(ord), key, toSet)

			return nil, nil
		},
	)

	functions["set_max_int64"] = wasmer.NewFunction(
		i.wasmStore,
		wasmer.NewFunctionType(
			params(wasmer.I64 /* ordinal */, wasmer.I32, wasmer.I32 /* key */, wasmer.I64 /* value */),
			returns(),
		),
		func(args []wasmer.Value) ([]wasmer.Value, error) {
			if i.outputStore == nil && i.updatePolicy != "max" && i.valueType != "int64" {
				return nil, fmt.Errorf("invalid store operation: 'set_max_int64' only valid for stores with updatePolicy == 'max' and valueType == 'int64'")
			}
			ord := args[0].I64()
			key, err := i.heap.ReadString(args[1].I32(), args[2].I32())
			if err != nil {
				return nil, fmt.Errorf("reading string: %w", err)
			}
			value := args[3].I64()

			i.outputStore.SetMaxInt64(uint64(ord), key, value)

			return nil, nil
		},
	)

	functions["set_max_bigint"] = wasmer.NewFunction(
		i.wasmStore,
		wasmer.NewFunctionType(
			params(wasmer.I64 /* ordinal */, wasmer.I32, wasmer.I32 /* key */, wasmer.I32, wasmer.I32 /* value */),
			returns(),
		),
		func(args []wasmer.Value) ([]wasmer.Value, error) {
			if i.outputStore == nil && i.updatePolicy != "max" && i.valueType != "bigint" {
				return nil, fmt.Errorf("invalid store operation: 'set_max_bigint' only valid for stores with updatePolicy == 'max' and valueType == 'bigint'")
			}
			ord := args[0].I64()
			key, err := i.heap.ReadString(args[1].I32(), args[2].I32())
			if err != nil {
				return nil, fmt.Errorf("reading string: %w", err)
			}
			value, err := i.heap.ReadString(args[3].I32(), args[4].I32())
			if err != nil {
				return nil, fmt.Errorf("reading bytes: %w", err)
			}

			toSet, _ := new(big.Int).SetString(value, 10)
			i.outputStore.SetMaxBigInt(uint64(ord), key, toSet)

			return nil, nil
		},
	)
	functions["set_max_float64"] = wasmer.NewFunction(
		i.wasmStore,
		wasmer.NewFunctionType(
			params(wasmer.I64 /* ordinal */, wasmer.I32, wasmer.I32 /* key */, wasmer.F64 /* value */),
			returns(),
		),
		func(args []wasmer.Value) ([]wasmer.Value, error) {
			if i.outputStore == nil && i.updatePolicy != "max" && i.valueType != "float" {
				return nil, fmt.Errorf("invalid store operation: 'set_max_float64' only valid for stores with updatePolicy == 'max' and valueType == 'float64'")
			}
			ord := args[0].I64()
			key, err := i.heap.ReadString(args[1].I32(), args[2].I32())
			if err != nil {
				return nil, fmt.Errorf("reading string: %w", err)
			}
			value := args[3].F64()
			i.outputStore.SetMaxFloat64(uint64(ord), key, value)

			return nil, nil
		},
	)

	functions["set_max_bigfloat"] = wasmer.NewFunction(
		i.wasmStore,
		wasmer.NewFunctionType(
			params(wasmer.I64 /* ordinal */, wasmer.I32, wasmer.I32 /* key */, wasmer.I32, wasmer.I32 /* value */),
			returns(),
		),
		func(args []wasmer.Value) ([]wasmer.Value, error) {
			if i.outputStore == nil && i.updatePolicy != "max" && i.valueType != "bigint" {
				return nil, fmt.Errorf("invalid store operation: 'set_max_bigfloat' only valid for stores with updatePolicy == 'max' and valueType == 'bigfloat'")
			}
			ord := args[0].I64()
			key, err := i.heap.ReadString(args[1].I32(), args[2].I32())
			if err != nil {
				return nil, fmt.Errorf("reading string: %w", err)
			}
			value, err := i.heap.ReadString(args[3].I32(), args[4].I32())
			if err != nil {
				return nil, fmt.Errorf("reading bytes: %w", err)
			}

			toSet, _, err := big.ParseFloat(value, 10, 100, big.ToNearestEven)
			i.outputStore.SetMaxBigFloat(uint64(ord), key, toSet)

			return nil, nil
		},
	)

	functions["get_at"] = wasmer.NewFunction(
		i.wasmStore,
		wasmer.NewFunctionType(
			params(wasmer.I32, /* store index */
				wasmer.I64, /* ordinal */
				wasmer.I32, /* key offset */
				wasmer.I32, /* key length */
				wasmer.I32 /* return pointer */),
			returns(wasmer.I32),
		),
		func(args []wasmer.Value) ([]wasmer.Value, error) {
			storeIndex := int(args[0].I32())
			if storeIndex+1 > len(i.inputStores) {
				return nil, fmt.Errorf("'get_at' failed: invalid store index %d, %d stores declared", storeIndex, len(i.inputStores))
			}
			readStore := i.inputStores[storeIndex]
			ord := args[1].I64()
			key, err := i.heap.ReadString(args[2].I32(), args[3].I32())
			if err != nil {
				return nil, fmt.Errorf("reading string: %w", err)
			}
			value, found := readStore.GetAt(uint64(ord), key)
			if !found {
				zero := wasmer.NewI32(0)
				return []wasmer.Value{zero}, nil
			}
			outputPtr := args[4].I32()
			err = i.writeOutputToHeap(outputPtr, value)
			if err != nil {
				return nil, fmt.Errorf("writing value to output ptr %d: %w", outputPtr, err)
			}
			return []wasmer.Value{wasmer.NewI32(1)}, nil
		},
	)
	functions["get_first"] = wasmer.NewFunction(
		i.wasmStore,
		wasmer.NewFunctionType(
			params(wasmer.I32,
				wasmer.I32,
				wasmer.I32,
				wasmer.I32),
			returns(wasmer.I32),
		),
		func(args []wasmer.Value) ([]wasmer.Value, error) {
			storeIndex := int(args[0].I32())
			if storeIndex+1 > len(i.inputStores) {
				return nil, fmt.Errorf("'get_first' failed: invalid store index %d, %d stores declared", storeIndex, len(i.inputStores))
			}
			readStore := i.inputStores[storeIndex]
			key, err := i.heap.ReadString(args[1].I32(), args[2].I32())
			if err != nil {
				return nil, fmt.Errorf("reading string: %w", err)
			}
			value, found := readStore.GetFirst(key)
			if !found {
				zero := wasmer.NewI32(0)
				return []wasmer.Value{zero, zero, zero}, nil
			}
			outputPtr := args[3].I32()
			err = i.writeOutputToHeap(outputPtr, value)
			if err != nil {
				return nil, fmt.Errorf("writing value to output ptr %d: %w", outputPtr, err)
			}
			return []wasmer.Value{wasmer.NewI32(1)}, nil

		},
	)
	functions["get_last"] = wasmer.NewFunction(
		i.wasmStore,
		wasmer.NewFunctionType(
			params(wasmer.I32,
				wasmer.I32,
				wasmer.I32,
				wasmer.I32),
			returns(wasmer.I32),
		),

		func(args []wasmer.Value) ([]wasmer.Value, error) {
			storeIndex := int(args[0].I32())
			if storeIndex+1 > len(i.inputStores) {
				return nil, fmt.Errorf("'get_last' failed: invalid store index %d, %d stores declared", storeIndex, len(i.inputStores))
			}
			readStore := i.inputStores[storeIndex]
			key, err := i.heap.ReadString(args[1].I32(), args[2].I32())
			if err != nil {
				return nil, fmt.Errorf("reading string: %w", err)
			}
			value, found := readStore.GetLast(key)
			if !found {
				zero := wasmer.NewI32(0)
				return []wasmer.Value{zero, zero, zero}, nil
			}
			outputPtr := args[3].I32()
			err = i.writeOutputToHeap(outputPtr, value)
			if err != nil {
				return nil, fmt.Errorf("writing value to output ptr %d: %w", outputPtr, err)
			}
			return []wasmer.Value{wasmer.NewI32(1)}, nil
		},
	)

	imports.Register("state", functions)
}

func (i *Instance) writeOutputToHeap(outputPtr int32, value []byte) error {

	valuePtr, err := i.heap.Write(value)
	if err != nil {
		return fmt.Errorf("writting value to heap: %w", err)
	}
	returnValue := make([]byte, 8)
	binary.LittleEndian.PutUint32(returnValue[0:4], uint32(valuePtr))
	binary.LittleEndian.PutUint32(returnValue[4:], uint32(len(value)))

	_, err = i.heap.WriteAtPtr(returnValue, outputPtr)
	if err != nil {
		return fmt.Errorf("writing response at valuePtr %d: %w", valuePtr, err)
	}

	return nil
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

func (i *Instance) SetOutputStore(store *state.Builder) {
	i.outputStore = store
}

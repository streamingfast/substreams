package wasm

import (
	"context"
	"fmt"
	"math/big"

	"github.com/dustin/go-humanize"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/wasmerio/wasmer-go/wasmer"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
)

type Module struct {
	runtime *Runtime

	engine *wasmer.Engine
	module *wasmer.Module
	name   string

	wasmCode        []byte
	CurrentInstance *Instance
	imports         *wasmer.ImportObject
}

func (r *Runtime) NewModule(ctx context.Context, request *pbsubstreams.Request, wasmCode []byte, name string) (*Module, error) {
	engine := wasmer.NewUniversalEngine()
	store := wasmer.NewStore(engine)

	module, err := wasmer.NewModule(store, wasmCode)
	if err != nil {
		return nil, fmt.Errorf("loading wasm module: %w", err)
	}

	m := &Module{
		runtime:  r,
		engine:   engine,
		module:   module,
		name:     name,
		wasmCode: wasmCode,
	}
	m.imports = m.newImports(store)

	for namespace, imports := range r.extensions {
		externs := map[string]wasmer.IntoExtern{}
		for importName, f := range imports {
			externs[importName] = m.newExtensionFunction(ctx, request, store, namespace, importName, f)
		}
		//
		m.imports.Register(namespace, externs)
	}

	return m, nil
}

func (m *Module) newExtensionFunction(ctx context.Context, request *pbsubstreams.Request, store *wasmer.Store, namespace, name string, f WASMExtension) *wasmer.Function {
	return wasmer.NewFunction(
		store,
		wasmer.NewFunctionType(
			Params(wasmer.I32, wasmer.I32, wasmer.I32), // 0(READ): input bytes offset,  1(READ): input length, 2(WRITE): output bytes offset
			Returns()),
		func(args []wasmer.Value) ([]wasmer.Value, error) {

			heap := m.CurrentInstance.Heap()

			message, err := heap.ReadBytes(args[0].I32(), args[1].I32())
			if err != nil {
				return nil, fmt.Errorf("read message argument: %w", err)
			}

			out, err := f(ctx, request, m.CurrentInstance.clock, message)
			if err != nil {
				return nil, fmt.Errorf(`failed running wasm extension "%s::%s": %w`, namespace, name, err)
			}

			// It's unclear if WASMExtension implementor will correctly handle the context canceled case, as a safety
			// measure, we check if the context was canceled without being handled correctly and stop here.
			if ctx.Err() == context.Canceled {
				return nil, fmt.Errorf("running wasm extension has been stop upstream in the call stack: %w", ctx.Err())
			}

			err = m.CurrentInstance.WriteOutputToHeap(args[2].I32(), out)
			if err != nil {
				return nil, fmt.Errorf("write output to heap %w", err)
			}
			return nil, nil
		},
	)
}

func (m *Module) NewInstance(clock *pbsubstreams.Clock, functionName string, inputs []*Input) (*Instance, error) {
	// WARN: An instance needs to be created on the same thread that it is consumed.
	store := wasmer.NewStore(m.engine)

	m.CurrentInstance = &Instance{
		moduleName:   m.name,
		store:        store,
		functionName: functionName,
		clock:        clock,
	}

	vmInstance, err := wasmer.NewInstance(m.module, m.imports)
	if err != nil {
		return nil, fmt.Errorf("creating instance: %w", err)
	}
	m.CurrentInstance.vmInstance = vmInstance

	memory, err := vmInstance.Exports.GetMemory("memory")
	if err != nil {
		return nil, fmt.Errorf("getting module memory: %w", err)
	}

	alloc, err := vmInstance.Exports.GetFunction("alloc")
	if err != nil {
		return nil, fmt.Errorf("getting alloc function: %w", err)
	}
	m.CurrentInstance.memory = memory
	m.CurrentInstance.heap = NewHeap(memory, alloc)
	m.CurrentInstance.entrypoint, err = vmInstance.Exports.GetRawFunction(functionName)
	if err != nil {
		return nil, fmt.Errorf("getting wasm module function %q: %w", functionName, err)
	}

	var args []interface{}
	for _, input := range inputs {
		switch input.Type {
		case InputSource:
			ptr, err := m.CurrentInstance.heap.Write(input.StreamData)
			if err != nil {
				return nil, fmt.Errorf("writing %q to heap: %w", input.Name, err)
			}
			len := int32(len(input.StreamData))
			args = append(args, ptr, len)
		case InputStore:
			if input.Deltas {
				//todo: this maybe sub optimal when deltas are extrated from module output cache
				cnt, err := proto.Marshal(&pbsubstreams.StoreDeltas{Deltas: input.Store.Deltas})
				if err != nil {
					return nil, fmt.Errorf("marshaling store deltas: %w", err)
				}
				ptr, err := m.CurrentInstance.heap.Write(cnt)
				if err != nil {
					return nil, fmt.Errorf("writing %q (deltas=%v) to heap: %w", input.Name, input.Deltas, err)
				}

				args = append(args, ptr, int32(len(cnt)))
			} else {
				m.CurrentInstance.inputStores = append(m.CurrentInstance.inputStores, input.Store)
				args = append(args, len(m.CurrentInstance.inputStores)-1)
			}
		case OutputStore:
			m.CurrentInstance.outputStore = input.Store
			m.CurrentInstance.updatePolicy = input.UpdatePolicy
			m.CurrentInstance.valueType = input.ValueType
		}
	}
	m.CurrentInstance.args = args

	return m.CurrentInstance, nil
}

func (m *Module) newImports(store *wasmer.Store) *wasmer.ImportObject {
	imports := wasmer.NewImportObject()

	m.registerLoggerImports(imports, store)
	m.registerStateImports(imports, store)

	imports.Register("env", map[string]wasmer.IntoExtern{
		"register_panic": wasmer.NewFunction(
			store,
			wasmer.NewFunctionType(
				Params(wasmer.I32, wasmer.I32, wasmer.I32, wasmer.I32, wasmer.I32, wasmer.I32),
				Returns(),
			),
			func(args []wasmer.Value) ([]wasmer.Value, error) {
				message, err := m.CurrentInstance.heap.ReadString(args[0].I32(), args[1].I32())
				if err != nil {
					return nil, fmt.Errorf("read message argument: %w", err)
				}

				var filename string
				filenamePtr := args[2].I32()
				if filenamePtr != 0 {
					filename, err = m.CurrentInstance.heap.ReadString(args[2].I32(), args[3].I32())
					if err != nil {
						return nil, fmt.Errorf("read filename argument: %w", err)
					}
				}

				lineNumber := int(args[4].I32())
				columnNumber := int(args[5].I32())

				m.CurrentInstance.panicError = &PanicError{message, filename, lineNumber, columnNumber}

				return nil, nil
			},
		),
		"output": wasmer.NewFunction(
			store,
			wasmer.NewFunctionType(
				Params(wasmer.I32, wasmer.I32),
				Returns(),
			),
			func(args []wasmer.Value) ([]wasmer.Value, error) {
				message, err := m.CurrentInstance.heap.ReadBytes(args[0].I32(), args[1].I32())
				if err != nil {
					return nil, fmt.Errorf("reading bytes: %w", err)
				}

				m.CurrentInstance.returnValue = message

				return nil, nil
			},
		),
	})
	return imports
}

func (m *Module) registerLoggerImports(imports *wasmer.ImportObject, store *wasmer.Store) {
	imports.Register("logger", map[string]wasmer.IntoExtern{
		"println": wasmer.NewFunction(
			store,
			wasmer.NewFunctionType(
				Params(wasmer.I32, wasmer.I32),
				Returns(),
			),
			func(args []wasmer.Value) ([]wasmer.Value, error) {
				if m.CurrentInstance.ReachedLogsMaxByteCount() {
					// Early exit, we don't even need to collect the message as we would not store it anyway
					return nil, nil
				}

				length := args[1].I32()
				if length > maxLogByteCount {
					return nil, fmt.Errorf("message to log is too big, max size is %s", humanize.IBytes(uint64(length)))
				}

				message, err := m.CurrentInstance.heap.ReadString(args[0].I32(), length)
				if err != nil {
					return nil, fmt.Errorf("reading string: %w", err)
				}

				if tracer.Enabled() {
					zlog.Debug(message, zap.String("function_name", m.CurrentInstance.functionName), zap.String("wasm_file", m.CurrentInstance.moduleName))
				}

				// len(<string>) in Go count number of bytes and not characters, so we are good here
				m.CurrentInstance.LogsByteCount += uint64(len(message))

				if !m.CurrentInstance.ReachedLogsMaxByteCount() {
					m.CurrentInstance.Logs = append(m.CurrentInstance.Logs, message)
				}

				return nil, nil
			},
		),
	})
}
func (m *Module) registerStateImports(imports *wasmer.ImportObject, store *wasmer.Store) {
	functions := map[string]wasmer.IntoExtern{}
	functions["set"] = wasmer.NewFunction(
		store,
		wasmer.NewFunctionType(
			Params(wasmer.I64, wasmer.I32, wasmer.I32, wasmer.I32, wasmer.I32),
			Returns(),
		),
		func(args []wasmer.Value) ([]wasmer.Value, error) {
			if m.CurrentInstance.outputStore == nil && m.CurrentInstance.updatePolicy != pbsubstreams.Module_KindStore_UPDATE_POLICY_SET {
				return nil, fmt.Errorf("invalid store operation: 'set' only valid for stores with updatePolicy == 'replace'")
			}
			ord := args[0].I64()
			key, err := m.CurrentInstance.heap.ReadString(args[1].I32(), args[2].I32())
			if err != nil {
				return nil, fmt.Errorf("reading string: %w", err)
			}
			value, err := m.CurrentInstance.heap.ReadBytes(args[3].I32(), args[4].I32())
			if err != nil {
				return nil, fmt.Errorf("reading bytes: %w", err)
			}

			m.CurrentInstance.outputStore.SetBytes(uint64(ord), key, value)

			return nil, nil
		},
	)

	functions["set_if_not_exists"] = wasmer.NewFunction(
		store,
		wasmer.NewFunctionType(
			Params(wasmer.I64, wasmer.I32, wasmer.I32, wasmer.I32, wasmer.I32),
			Returns(),
		),
		func(args []wasmer.Value) ([]wasmer.Value, error) {
			if m.CurrentInstance.outputStore == nil && m.CurrentInstance.updatePolicy != pbsubstreams.Module_KindStore_UPDATE_POLICY_SET_IF_NOT_EXISTS {
				return nil, fmt.Errorf("invalid store operation: 'set_if_not_exists' only valid for stores with updatePolicy == 'ignore'")
			}
			ord := args[0].I64()
			key, err := m.CurrentInstance.heap.ReadString(args[1].I32(), args[2].I32())
			if err != nil {
				return nil, fmt.Errorf("reading string: %w", err)
			}
			value, err := m.CurrentInstance.heap.ReadBytes(args[3].I32(), args[4].I32())
			if err != nil {
				return nil, fmt.Errorf("reading bytes: %w", err)
			}

			m.CurrentInstance.outputStore.SetBytesIfNotExists(uint64(ord), key, value)

			return nil, nil
		},
	)
	functions["delete_prefix"] = wasmer.NewFunction(
		store,
		wasmer.NewFunctionType(
			Params(
				wasmer.I64, /* ordinal */
				wasmer.I32, /* prefix offset */
				wasmer.I32, /* prefix length */
			),
			Returns(),
		),
		func(args []wasmer.Value) ([]wasmer.Value, error) {
			prefix, err := m.CurrentInstance.heap.ReadString(args[1].I32(), args[2].I32())
			if err != nil {
				return nil, fmt.Errorf("reading prefix: %w", err)
			}
			ord := args[0].I64()
			m.CurrentInstance.outputStore.DeletePrefix(uint64(ord), prefix)
			return nil, nil
		},
	)
	functions["add_bigfloat"] = wasmer.NewFunction(
		store,
		wasmer.NewFunctionType(
			Params(wasmer.I64 /* ordinal */, wasmer.I32, wasmer.I32 /* key */, wasmer.I32, wasmer.I32 /* value */),
			Returns(),
		),
		func(args []wasmer.Value) ([]wasmer.Value, error) {
			if m.CurrentInstance.outputStore == nil && m.CurrentInstance.updatePolicy != pbsubstreams.Module_KindStore_UPDATE_POLICY_ADD && m.CurrentInstance.valueType != "bigfloat" {
				return nil, fmt.Errorf("invalid store operation: 'add_bigfloat' only valid for stores with updatePolicy == 'add' and valueType == 'bigfloat'")
			}
			ord := args[0].I64()
			key, err := m.CurrentInstance.heap.ReadString(args[1].I32(), args[2].I32())
			if err != nil {
				return nil, fmt.Errorf("reading string: %w", err)
			}
			value, err := m.CurrentInstance.heap.ReadString(args[3].I32(), args[4].I32())
			if err != nil {
				return nil, fmt.Errorf("reading bytes: %w", err)
			}

			toAdd, _, err := big.ParseFloat(value, 10, 100, big.ToNearestEven) // corresponds to SumBigFloat's read of the kv value
			if err != nil {
				return nil, fmt.Errorf("parsing bigfloat value %q: %w", value, err)
			}

			m.CurrentInstance.outputStore.SumBigFloat(uint64(ord), key, toAdd)

			return nil, nil
		},
	)

	functions["add_bigint"] = wasmer.NewFunction(
		store,
		wasmer.NewFunctionType(
			Params(wasmer.I64 /* ordinal */, wasmer.I32, wasmer.I32 /* key */, wasmer.I32, wasmer.I32 /* value */),
			Returns(),
		),
		func(args []wasmer.Value) ([]wasmer.Value, error) {
			if m.CurrentInstance.outputStore == nil && m.CurrentInstance.updatePolicy != pbsubstreams.Module_KindStore_UPDATE_POLICY_ADD && m.CurrentInstance.valueType != "bigint" {
				return nil, fmt.Errorf("invalid store operation: 'add_bigint' only valid for stores with updatePolicy == 'add' and valueType == 'bigint'")
			}
			ord := args[0].I64()
			key, err := m.CurrentInstance.heap.ReadString(args[1].I32(), args[2].I32())
			if err != nil {
				return nil, fmt.Errorf("reading string: %w", err)
			}
			value, err := m.CurrentInstance.heap.ReadString(args[3].I32(), args[4].I32())
			if err != nil {
				return nil, fmt.Errorf("reading bytes: %w", err)
			}

			toAdd, _ := new(big.Int).SetString(value, 10)
			m.CurrentInstance.outputStore.SumBigInt(uint64(ord), key, toAdd)

			return nil, nil
		},
	)

	functions["add_int64"] = wasmer.NewFunction(
		store,
		wasmer.NewFunctionType(
			Params(wasmer.I64 /* ordinal */, wasmer.I32, wasmer.I32 /* key */, wasmer.I64 /* value */),
			Returns(),
		),
		func(args []wasmer.Value) ([]wasmer.Value, error) {
			if m.CurrentInstance.outputStore == nil && m.CurrentInstance.updatePolicy != pbsubstreams.Module_KindStore_UPDATE_POLICY_ADD && m.CurrentInstance.valueType != "int64" {
				return nil, fmt.Errorf("invalid store operation: 'add_bigint' only valid for stores with updatePolicy == 'add' and valueType == 'int64'")
			}
			ord := args[0].I64()
			key, err := m.CurrentInstance.heap.ReadString(args[1].I32(), args[2].I32())
			if err != nil {
				return nil, fmt.Errorf("reading string: %w", err)
			}
			value := args[3].I64()

			m.CurrentInstance.outputStore.SumInt64(uint64(ord), key, value)

			return nil, nil
		},
	)

	functions["add_float64"] = wasmer.NewFunction(
		store,
		wasmer.NewFunctionType(
			Params(wasmer.I64 /* ordinal */, wasmer.I32, wasmer.I32 /* key */, wasmer.F64 /* value */),
			Returns(),
		),
		func(args []wasmer.Value) ([]wasmer.Value, error) {
			if m.CurrentInstance.outputStore == nil && m.CurrentInstance.updatePolicy != pbsubstreams.Module_KindStore_UPDATE_POLICY_ADD && m.CurrentInstance.valueType != "float64" {
				return nil, fmt.Errorf("invalid store operation: 'add_float64' only valid for stores with updatePolicy == 'add' and valueType == 'float64'")
			}
			ord := args[0].I64()
			key, err := m.CurrentInstance.heap.ReadString(args[1].I32(), args[2].I32())
			if err != nil {
				return nil, fmt.Errorf("reading string: %w", err)
			}

			value := args[3].F64()
			if err != nil {
				return nil, fmt.Errorf("reading bytes: %w", err)
			}
			m.CurrentInstance.outputStore.SumFloat64(uint64(ord), key, value)

			return nil, nil
		},
	)

	functions["set_min_int64"] = wasmer.NewFunction(
		store,
		wasmer.NewFunctionType(
			Params(wasmer.I64 /* ordinal */, wasmer.I32, wasmer.I32 /* key */, wasmer.I64 /* value */),
			Returns(),
		),
		func(args []wasmer.Value) ([]wasmer.Value, error) {
			if m.CurrentInstance.outputStore == nil && m.CurrentInstance.updatePolicy != pbsubstreams.Module_KindStore_UPDATE_POLICY_MIN && m.CurrentInstance.valueType != "int64" {
				return nil, fmt.Errorf("invalid store operation: 'set_min_int64' only valid for stores with updatePolicy == 'min' and valueType == 'int64'")
			}
			ord := args[0].I64()
			key, err := m.CurrentInstance.heap.ReadString(args[1].I32(), args[2].I32())
			if err != nil {
				return nil, fmt.Errorf("reading string: %w", err)
			}
			value := args[3].I64()

			m.CurrentInstance.outputStore.SetMinInt64(uint64(ord), key, value)

			return nil, nil
		},
	)

	functions["set_min_bigint"] = wasmer.NewFunction(
		store,
		wasmer.NewFunctionType(
			Params(wasmer.I64 /* ordinal */, wasmer.I32, wasmer.I32 /* key */, wasmer.I32, wasmer.I32 /* value */),
			Returns(),
		),
		func(args []wasmer.Value) ([]wasmer.Value, error) {
			if m.CurrentInstance.outputStore == nil && m.CurrentInstance.updatePolicy != pbsubstreams.Module_KindStore_UPDATE_POLICY_MIN && m.CurrentInstance.valueType != "bigfloat" {
				return nil, fmt.Errorf("invalid store operation: 'set_min_bigint' only valid for stores with updatePolicy == 'min' and valueType == 'bigint'")
			}
			ord := args[0].I64()
			key, err := m.CurrentInstance.heap.ReadString(args[1].I32(), args[2].I32())
			if err != nil {
				return nil, fmt.Errorf("reading string: %w", err)
			}
			value, err := m.CurrentInstance.heap.ReadString(args[3].I32(), args[4].I32())
			if err != nil {
				return nil, fmt.Errorf("reading bytes: %w", err)
			}

			toSet, _ := new(big.Int).SetString(value, 10)
			m.CurrentInstance.outputStore.SetMinBigInt(uint64(ord), key, toSet)

			return nil, nil
		},
	)
	functions["set_min_float64"] = wasmer.NewFunction(
		store,
		wasmer.NewFunctionType(
			Params(wasmer.I64 /* ordinal */, wasmer.I32, wasmer.I32 /* key */, wasmer.F64 /* value */),
			Returns(),
		),
		func(args []wasmer.Value) ([]wasmer.Value, error) {
			if m.CurrentInstance.outputStore == nil && m.CurrentInstance.updatePolicy != pbsubstreams.Module_KindStore_UPDATE_POLICY_MIN && m.CurrentInstance.valueType != "float" {
				return nil, fmt.Errorf("invalid store operation: 'set_min_float64' only valid for stores with updatePolicy == 'min' and valueType == 'int64'")
			}
			ord := args[0].I64()
			key, err := m.CurrentInstance.heap.ReadString(args[1].I32(), args[2].I32())
			if err != nil {
				return nil, fmt.Errorf("reading string: %w", err)
			}
			value := args[3].F64()
			m.CurrentInstance.outputStore.SetMinFloat64(uint64(ord), key, value)

			return nil, nil
		},
	)

	functions["set_min_bigfloat"] = wasmer.NewFunction(
		store,
		wasmer.NewFunctionType(
			Params(wasmer.I64 /* ordinal */, wasmer.I32, wasmer.I32 /* key */, wasmer.I32, wasmer.I32 /* value */),
			Returns(),
		),
		func(args []wasmer.Value) ([]wasmer.Value, error) {
			if m.CurrentInstance.outputStore == nil && m.CurrentInstance.updatePolicy != pbsubstreams.Module_KindStore_UPDATE_POLICY_MIN && m.CurrentInstance.valueType != "bigint" {
				return nil, fmt.Errorf("invalid store operation: 'set_min_bigfloat' only valid for stores with updatePolicy == 'min' and valueType == 'bigint'")
			}
			ord := args[0].I64()
			key, err := m.CurrentInstance.heap.ReadString(args[1].I32(), args[2].I32())
			if err != nil {
				return nil, fmt.Errorf("reading string: %w", err)
			}
			value, err := m.CurrentInstance.heap.ReadString(args[3].I32(), args[4].I32())
			if err != nil {
				return nil, fmt.Errorf("reading bytes: %w", err)
			}

			toSet, _, err := big.ParseFloat(value, 10, 100, big.ToNearestEven)
			m.CurrentInstance.outputStore.SetMinBigFloat(uint64(ord), key, toSet)

			return nil, nil
		},
	)

	functions["set_max_int64"] = wasmer.NewFunction(
		store,
		wasmer.NewFunctionType(
			Params(wasmer.I64 /* ordinal */, wasmer.I32, wasmer.I32 /* key */, wasmer.I64 /* value */),
			Returns(),
		),
		func(args []wasmer.Value) ([]wasmer.Value, error) {
			if m.CurrentInstance.outputStore == nil && m.CurrentInstance.updatePolicy != pbsubstreams.Module_KindStore_UPDATE_POLICY_MAX && m.CurrentInstance.valueType != "int64" {
				return nil, fmt.Errorf("invalid store operation: 'set_max_int64' only valid for stores with updatePolicy == 'max' and valueType == 'int64'")
			}
			ord := args[0].I64()
			key, err := m.CurrentInstance.heap.ReadString(args[1].I32(), args[2].I32())
			if err != nil {
				return nil, fmt.Errorf("reading string: %w", err)
			}
			value := args[3].I64()

			m.CurrentInstance.outputStore.SetMaxInt64(uint64(ord), key, value)

			return nil, nil
		},
	)

	functions["set_max_bigint"] = wasmer.NewFunction(
		store,
		wasmer.NewFunctionType(
			Params(wasmer.I64 /* ordinal */, wasmer.I32, wasmer.I32 /* key */, wasmer.I32, wasmer.I32 /* value */),
			Returns(),
		),
		func(args []wasmer.Value) ([]wasmer.Value, error) {
			if m.CurrentInstance.outputStore == nil && m.CurrentInstance.updatePolicy != pbsubstreams.Module_KindStore_UPDATE_POLICY_MAX && m.CurrentInstance.valueType != "bigint" {
				return nil, fmt.Errorf("invalid store operation: 'set_max_bigint' only valid for stores with updatePolicy == 'max' and valueType == 'bigint'")
			}
			ord := args[0].I64()
			key, err := m.CurrentInstance.heap.ReadString(args[1].I32(), args[2].I32())
			if err != nil {
				return nil, fmt.Errorf("reading string: %w", err)
			}
			value, err := m.CurrentInstance.heap.ReadString(args[3].I32(), args[4].I32())
			if err != nil {
				return nil, fmt.Errorf("reading bytes: %w", err)
			}

			toSet, _ := new(big.Int).SetString(value, 10)
			m.CurrentInstance.outputStore.SetMaxBigInt(uint64(ord), key, toSet)

			return nil, nil
		},
	)
	functions["set_max_float64"] = wasmer.NewFunction(
		store,
		wasmer.NewFunctionType(
			Params(wasmer.I64 /* ordinal */, wasmer.I32, wasmer.I32 /* key */, wasmer.F64 /* value */),
			Returns(),
		),
		func(args []wasmer.Value) ([]wasmer.Value, error) {
			if m.CurrentInstance.outputStore == nil && m.CurrentInstance.updatePolicy != pbsubstreams.Module_KindStore_UPDATE_POLICY_MAX && m.CurrentInstance.valueType != "float" {
				return nil, fmt.Errorf("invalid store operation: 'set_max_float64' only valid for stores with updatePolicy == 'max' and valueType == 'float64'")
			}
			ord := args[0].I64()
			key, err := m.CurrentInstance.heap.ReadString(args[1].I32(), args[2].I32())
			if err != nil {
				return nil, fmt.Errorf("reading string: %w", err)
			}
			value := args[3].F64()
			m.CurrentInstance.outputStore.SetMaxFloat64(uint64(ord), key, value)

			return nil, nil
		},
	)

	functions["set_max_bigfloat"] = wasmer.NewFunction(
		store,
		wasmer.NewFunctionType(
			Params(wasmer.I64 /* ordinal */, wasmer.I32, wasmer.I32 /* key */, wasmer.I32, wasmer.I32 /* value */),
			Returns(),
		),
		func(args []wasmer.Value) ([]wasmer.Value, error) {
			if m.CurrentInstance.outputStore == nil && m.CurrentInstance.updatePolicy != pbsubstreams.Module_KindStore_UPDATE_POLICY_MAX && m.CurrentInstance.valueType != "bigint" {
				return nil, fmt.Errorf("invalid store operation: 'set_max_bigfloat' only valid for stores with updatePolicy == 'max' and valueType == 'bigfloat'")
			}
			ord := args[0].I64()
			key, err := m.CurrentInstance.heap.ReadString(args[1].I32(), args[2].I32())
			if err != nil {
				return nil, fmt.Errorf("reading string: %w", err)
			}
			value, err := m.CurrentInstance.heap.ReadString(args[3].I32(), args[4].I32())
			if err != nil {
				return nil, fmt.Errorf("reading bytes: %w", err)
			}

			toSet, _, err := big.ParseFloat(value, 10, 100, big.ToNearestEven)
			m.CurrentInstance.outputStore.SetMaxBigFloat(uint64(ord), key, toSet)

			return nil, nil
		},
	)

	functions["get_at"] = wasmer.NewFunction(
		store,
		wasmer.NewFunctionType(
			Params(wasmer.I32, /* store index */
				wasmer.I64, /* ordinal */
				wasmer.I32, /* key offset */
				wasmer.I32, /* key length */
				wasmer.I32 /* return pointer */),
			Returns(wasmer.I32),
		),
		func(args []wasmer.Value) ([]wasmer.Value, error) {
			storeIndex := int(args[0].I32())
			if storeIndex+1 > len(m.CurrentInstance.inputStores) {
				return nil, fmt.Errorf("'get_at' failed: invalid store index %d, %d stores declared", storeIndex, len(m.CurrentInstance.inputStores))
			}
			readStore := m.CurrentInstance.inputStores[storeIndex]
			ord := args[1].I64()
			key, err := m.CurrentInstance.heap.ReadString(args[2].I32(), args[3].I32())
			if err != nil {
				return nil, fmt.Errorf("reading string: %w", err)
			}
			value, found := readStore.GetAt(uint64(ord), key)
			if !found {
				zero := wasmer.NewI32(0)
				return []wasmer.Value{zero}, nil
			}
			outputPtr := args[4].I32()
			err = m.CurrentInstance.WriteOutputToHeap(outputPtr, value)
			if err != nil {
				return nil, fmt.Errorf("writing value to output ptr %d: %w", outputPtr, err)
			}
			return []wasmer.Value{wasmer.NewI32(1)}, nil
		},
	)
	functions["get_first"] = wasmer.NewFunction(
		store,
		wasmer.NewFunctionType(
			Params(wasmer.I32,
				wasmer.I32,
				wasmer.I32,
				wasmer.I32),
			Returns(wasmer.I32),
		),
		func(args []wasmer.Value) ([]wasmer.Value, error) {
			storeIndex := int(args[0].I32())
			if storeIndex+1 > len(m.CurrentInstance.inputStores) {
				return nil, fmt.Errorf("'get_first' failed: invalid store index %d, %d stores declared", storeIndex, len(m.CurrentInstance.inputStores))
			}
			readStore := m.CurrentInstance.inputStores[storeIndex]
			key, err := m.CurrentInstance.heap.ReadString(args[1].I32(), args[2].I32())
			if err != nil {
				return nil, fmt.Errorf("reading string: %w", err)
			}
			value, found := readStore.GetFirst(key)
			if !found {
				zero := wasmer.NewI32(0)
				return []wasmer.Value{zero}, nil
			}
			outputPtr := args[3].I32()
			err = m.CurrentInstance.WriteOutputToHeap(outputPtr, value)
			if err != nil {
				return nil, fmt.Errorf("writing value to output ptr %d: %w", outputPtr, err)
			}
			return []wasmer.Value{wasmer.NewI32(1)}, nil

		},
	)
	functions["get_last"] = wasmer.NewFunction(
		store,
		wasmer.NewFunctionType(
			Params(wasmer.I32,
				wasmer.I32,
				wasmer.I32,
				wasmer.I32),
			Returns(wasmer.I32),
		),

		func(args []wasmer.Value) ([]wasmer.Value, error) {
			storeIndex := int(args[0].I32())
			if storeIndex+1 > len(m.CurrentInstance.inputStores) {
				return nil, fmt.Errorf("'get_last' failed: invalid store index %d, %d stores declared", storeIndex, len(m.CurrentInstance.inputStores))
			}
			readStore := m.CurrentInstance.inputStores[storeIndex]
			key, err := m.CurrentInstance.heap.ReadString(args[1].I32(), args[2].I32())
			if err != nil {
				return nil, fmt.Errorf("reading string: %w", err)
			}
			value, found := readStore.GetLast(key)
			if !found {
				zero := wasmer.NewI32(0)
				return []wasmer.Value{zero}, nil
			}
			outputPtr := args[3].I32()
			err = m.CurrentInstance.WriteOutputToHeap(outputPtr, value)
			if err != nil {
				return nil, fmt.Errorf("writing value to output ptr %d: %w", outputPtr, err)
			}
			return []wasmer.Value{wasmer.NewI32(1)}, nil
		},
	)

	imports.Register("state", functions)
}

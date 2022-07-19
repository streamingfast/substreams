package wasm

import (
	"context"
	"errors"
	"fmt"

	"github.com/dustin/go-humanize"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/sys"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
)

type Module struct {
	runtime *Runtime

	name string

	wasmCode        []byte
	CurrentInstance *Instance
	zeroRuntime     wazero.Runtime
	zeroModule      api.Module
}

func (r *Runtime) NewModule(ctx context.Context, request *pbsubstreams.Request, wasmCode []byte, name string) (*Module, error) {
	zeroRuntime := wazero.NewRuntime()

	m := &Module{
		runtime:     r,
		zeroRuntime: zeroRuntime,
		name:        name,
		wasmCode:    wasmCode,
	}
	if err := m.newImports(ctx); err != nil {
		return nil, fmt.Errorf("instantiating imports: %w", err)
	}
	for namespace, imports := range r.extensions {
		externs := map[string]interface{}{}
		for importName, f := range imports {
			externs[importName] = m.newExtensionFunction(request, namespace, importName, f)
		}
		//
		_, err := m.zeroRuntime.NewModuleBuilder(namespace).
			ExportFunctions(externs).Instantiate(ctx, m.zeroRuntime)
		if err != nil {
			return nil, fmt.Errorf("instantiating %s externs: %w", namespace, err)
		}
	}

	zeroModule, err := zeroRuntime.InstantiateModuleFromBinary(ctx, wasmCode)
	if err != nil {
		return nil, fmt.Errorf("instantiate zeroModule: %w", err)
	}
	m.zeroModule = zeroModule

	return m, nil
}

func (m *Module) Memory() api.Memory {
	return m.zeroModule.Memory()
}

func (m *Module) newExtensionFunction(request *pbsubstreams.Request, namespace, name string, f WASMExtension) interface{} {
	return func(ctx context.Context, apiModule api.Module, ptr, length, outputPtr uint32) {

		heap := m.CurrentInstance.Heap()

		data, err := heap.ReadBytes(ctx, apiModule.Memory(), ptr, length)
		if err != nil {
			panic(fmt.Errorf("read extension argument: %w", err))
		}

		out, err := f(ctx, request, m.CurrentInstance.clock, data)
		if err != nil {
			panic(fmt.Errorf(`running wasm extension "%s::%s": %w`, namespace, name, err))
		}

		// It's unclear if WASMExtension implementor will correctly handle the context canceled case, as a safety
		// measure, we check if the context was canceled without being handled correctly and stop here.
		if ctx.Err() == context.Canceled {
			panic(fmt.Errorf("running wasm extension has been stop upstream in the call stack: %w", ctx.Err()))
		}

		err = m.CurrentInstance.WriteOutputToHeap(ctx, apiModule.Memory(), outputPtr, out)
		if err != nil {
			panic(fmt.Errorf("write output to heap %w", err))
		}
	}
}

func (m *Module) NewInstance(ctx context.Context, clock *pbsubstreams.Clock, functionName string, inputs []*Input) (*Instance, error) {
	m.CurrentInstance = &Instance{
		moduleName:   m.name,
		functionName: functionName,
		clock:        clock,
	}
	alloc := m.zeroModule.ExportedFunction("alloc")
	dealloc := m.zeroModule.ExportedFunction("dealloc")

	if alloc == nil || dealloc == nil {
		panic("missing malloc or free")
	}

	m.CurrentInstance.heap = NewHeap(alloc, dealloc)
	m.CurrentInstance.entrypoint = m.zeroModule.ExportedFunction(functionName)
	if m.CurrentInstance.entrypoint == nil {
		return nil, fmt.Errorf("failed to get exported function %q", functionName)
	}

	var args []uint64
	for _, input := range inputs {
		switch input.Type {
		case InputSource:
			ptr, err := m.CurrentInstance.heap.Write(ctx, m.zeroModule.Memory(), input.StreamData)
			if err != nil {
				return nil, fmt.Errorf("writing %q to heap: %w", input.Name, err)
			}
			len := uint64(len(input.StreamData))
			args = append(args, uint64(ptr), len)
		case InputStore:
			if input.Deltas {
				//todo: this maybe sub optimal when deltas are extrated from zeroModule output cache
				cnt, err := proto.Marshal(&pbsubstreams.StoreDeltas{Deltas: input.Store.Deltas})
				if err != nil {
					return nil, fmt.Errorf("marshaling store deltas: %w", err)
				}
				ptr, err := m.CurrentInstance.heap.Write(ctx, m.zeroModule.Memory(), cnt)
				if err != nil {
					return nil, fmt.Errorf("writing %q (deltas=%v) to heap: %w", input.Name, input.Deltas, err)
				}

				args = append(args, uint64(ptr), uint64(len(cnt)))
			} else {
				m.CurrentInstance.inputStores = append(m.CurrentInstance.inputStores, input.Store)
				args = append(args, uint64(len(m.CurrentInstance.inputStores)-1))
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

func (m *Module) newImports(ctx context.Context) error {
	err := m.registerLoggerImports(ctx)
	if err != nil {
		return fmt.Errorf("registering logger imports: %w", err)
	}
	err = m.registerStateImports(ctx)
	if err != nil {
		return fmt.Errorf("registering state imports: %w", err)
	}

	_, err = m.zeroRuntime.NewModuleBuilder("env").
		ExportFunction("register_panic",
			func(ctx context.Context, apiModule api.Module, msgPtr, msgLength uint32, filenamePtr, filenameLength uint32, lineNumber, columnNumber uint32) {
				message, err := m.CurrentInstance.Heap().ReadString(ctx, apiModule.Memory(), msgPtr, msgLength)
				if err != nil {
					panic(fmt.Errorf("read message argument: %w", err))
				}

				var filename string
				if filenamePtr != 0 {
					filename, err = m.CurrentInstance.Heap().ReadString(ctx, apiModule.Memory(), msgPtr, msgLength)
					if err != nil {
						panic(fmt.Errorf("read filename argument: %w", err))
					}
				}

				m.CurrentInstance.panicError = &PanicError{message, filename, int(lineNumber), int(columnNumber)}
			},
		).ExportFunction("output",
		func(ctx context.Context, apiModule api.Module, ptr, length uint32) {
			message, err := m.CurrentInstance.heap.ReadBytes(ctx, apiModule.Memory(), ptr, length)
			if err != nil {
				returnError("env", fmt.Errorf("reading bytes: %w", err))
			}
			copy(m.CurrentInstance.returnValue, message)
			m.CurrentInstance.returnValue = message
		},
	).
		Instantiate(ctx, m.zeroRuntime)

	if err != nil {
		return fmt.Errorf("instantiating env zeroModule: %w", err)
	}
	return nil
}

func (m *Module) registerLoggerImports(ctx context.Context) error {
	_, err := m.zeroRuntime.NewModuleBuilder("logger").
		ExportFunction("println",
			func(ctx context.Context, apiModule api.Module, ptr uint32, length uint32) {
				if m.CurrentInstance.ReachedLogsMaxByteCount() {
					// Early exit, we don't even need to collect the message as we would not store it anyway
					return
				}

				if length > maxLogByteCount {
					panic(fmt.Errorf("message to log is too big, max size is %s", humanize.IBytes(uint64(length))))
				}

				message, err := m.CurrentInstance.heap.ReadString(ctx, apiModule.Memory(), ptr, length)
				if err != nil {
					panic(fmt.Errorf("reading string: %w", err))
				}

				if tracer.Enabled() {
					zlog.Debug(message, zap.String("function_name", m.CurrentInstance.functionName), zap.String("wasm_file", m.CurrentInstance.moduleName))
				}

				// len(<string>) in Go count number of bytes and not characters, so we are good here
				m.CurrentInstance.LogsByteCount += uint64(len(message))

				if !m.CurrentInstance.ReachedLogsMaxByteCount() {
					m.CurrentInstance.Logs = append(m.CurrentInstance.Logs, message)
				}

				return
			},
		).Instantiate(ctx, m.zeroRuntime)
	if err != nil {
		return fmt.Errorf("instantiating env zeroModule: %w", err)
	}
	return nil
}

type externError struct {
	*sys.ExitError
	cause error
}

func newExternError(moduleName string, cause error) *externError {
	return &externError{
		ExitError: sys.NewExitError(moduleName, 1),
		cause:     cause,
	}
}

func (e externError) Error() string {
	return e.ExitError.Error() + ": " + e.cause.Error()
}

func returnErrorString(moduleName, cause string) {
	panic(newExternError(moduleName, errors.New(cause)))
}
func returnError(moduleName string, cause error) {
	panic(newExternError(moduleName, cause))
}

func (m *Module) registerStateImports(ctx context.Context) error {
	functions := map[string]interface{}{}
	functions["set"] = m.set
	functions["set_if_not_exists"] = m.setIfNotExists
	functions["append"] = m.append
	functions["delete_prefix"] = m.deletePrefix
	functions["add_bigint"] = m.addBigInt
	functions["add_bigfloat"] = m.addBigFloat
	functions["add_int64"] = m.addInt64
	functions["add_float64"] = m.addFloat64
	functions["set_min_int64"] = m.setMinInt64
	functions["set_min_bigint"] = m.setMinBigint
	functions["set_min_float64"] = m.setMinfloat64
	functions["set_min_bigfloat"] = m.setMinBigfloat
	functions["set_max_int64"] = m.setMaxInt64
	functions["set_max_bigint"] = m.setMaxBigint
	functions["set_max_float64"] = m.setMaxFloat64
	functions["set_max_bigfloat"] = m.setMaxBigfloat
	functions["get_at"] = m.getAt
	functions["get_first"] = m.getFirst
	functions["get_last"] = m.getLast

	_, err := m.zeroRuntime.NewModuleBuilder("state").ExportFunctions(functions).Instantiate(ctx, m.zeroRuntime)

	if err != nil {
		return fmt.Errorf("instantiating state zeroModule: %w", err)
	}
	return nil
}

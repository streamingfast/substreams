package wasm

import (
	"context"
	"errors"
	"fmt"

	wasmtime "github.com/bytecodealliance/wasmtime-go/v4"
	"github.com/dustin/go-humanize"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"go.uber.org/zap"
)

type Module struct {
	runtime *Runtime

	name string

	wasmCode        []byte
	CurrentInstance *Instance
	entrypoint      string
	wasmInstance    *wasmtime.Instance
	wasmEngine      *wasmtime.Engine
	wasmStore       *wasmtime.Store
	wasmModule      *wasmtime.Module
	wasmLinker      *wasmtime.Linker
	Heap            *Heap
	isClosed        bool
}

func (m *Module) FreeMem() {
	m.wasmStore.FreeMem()
	m.wasmLinker.FreeMem()
	m.wasmEngine.FreeMem()
	m.isClosed = true
}

func (r *Runtime) NewModule(ctx context.Context, request *pbsubstreams.Request, wasmCode []byte, name string, entrypoint string) (*Module, error) {
	cfg := wasmtime.NewConfig()
	if r.maxFuel != 0 {
		cfg.SetConsumeFuel(true)
	}
	engine := wasmtime.NewEngineWithConfig(cfg)
	linker := wasmtime.NewLinker(engine)
	store := wasmtime.NewStore(engine)

	module, err := wasmtime.NewModule(store.Engine, wasmCode)
	if err != nil {
		return nil, fmt.Errorf("creating new module: %w", err)
	}

	m := &Module{
		runtime:    r,
		wasmEngine: engine,
		wasmLinker: linker,
		wasmStore:  store,
		wasmModule: module,
		name:       name,
		wasmCode:   wasmCode,
		entrypoint: entrypoint,
	}
	if err := m.newImports(); err != nil {
		return nil, fmt.Errorf("instantiating imports: %w", err)
	}
	for namespace, imports := range r.extensions {
		for importName, f := range imports {
			f := m.newExtensionFunction(ctx, request, namespace, importName, f)
			if err := linker.FuncWrap(namespace, importName, f); err != nil {
				return nil, fmt.Errorf("instantiating extension import, [%s@%s]: %w", namespace, name, err)
			}
		}
	}
	instance, err := m.wasmLinker.Instantiate(m.wasmStore, m.wasmModule)
	if err != nil {
		return nil, fmt.Errorf("creating new instance: %w", err)
	}
	memory := instance.GetExport(m.wasmStore, "memory").Memory()

	alloc := instance.GetExport(m.wasmStore, "alloc").Func()
	dealloc := instance.GetExport(m.wasmStore, "dealloc").Func()
	if alloc == nil || dealloc == nil {
		panic("missing malloc or free")
	}

	heap := NewHeap(memory, alloc, dealloc, m.wasmStore)
	m.Heap = heap
	m.wasmInstance = instance
	return m, nil
}

func (m *Module) NewInstance(clock *pbsubstreams.Clock, arguments []Argument) (*Instance, error) {
	if m.isClosed {
		panic("module is closed")
	}
	export := m.wasmInstance.GetExport(m.wasmStore, m.entrypoint)
	if export == nil {
		return nil, fmt.Errorf("failed to get entrypoint %q most likely does not exists", m.entrypoint)
	}
	entrypoint := export.Func()
	if entrypoint == nil {
		return nil, fmt.Errorf("failed to get exported function %q", entrypoint)
	}

	m.CurrentInstance = &Instance{
		Module:     m,
		clock:      clock,
		entrypoint: entrypoint,
	}
	if m.runtime.maxFuel != 0 {
		if remaining, _ := m.wasmStore.ConsumeFuel(m.runtime.maxFuel); remaining != 0 {
			m.wasmStore.ConsumeFuel(remaining) // don't accumulate fuel from previous executions
		}
		m.wasmStore.AddFuel(m.runtime.maxFuel)
	}

	var args []interface{}
	for _, input := range arguments {
		switch v := input.(type) {
		case *StoreWriterOutput:
			m.CurrentInstance.outputStore = v.Store
			m.CurrentInstance.updatePolicy = v.UpdatePolicy
			m.CurrentInstance.valueType = v.ValueType
		case *StoreReaderInput:
			m.CurrentInstance.inputStores = append(m.CurrentInstance.inputStores, v.Store)
			args = append(args, int32(len(m.CurrentInstance.inputStores)-1))
		case ValueArgument:
			cnt := v.Value()
			ptr, err := m.Heap.Write(cnt, input.Name())
			if err != nil {
				return nil, fmt.Errorf("writing %s to heap: %w", input.Name(), err)
			}
			length := int32(len(cnt))
			args = append(args, ptr, length)
		default:
			panic("unknown wasm argument type")
		}
	}
	m.CurrentInstance.args = args

	return m.CurrentInstance, nil
}

func (m *Module) newExtensionFunction(ctx context.Context, request *pbsubstreams.Request, namespace, name string, f WASMExtension) interface{} {
	return func(ptr, length, outputPtr int32) {
		heap := m.Heap

		data := heap.ReadBytes(ptr, length)

		out, err := f(ctx, request, m.CurrentInstance.clock, data)
		if err != nil {
			panic(fmt.Errorf(`running wasm extension "%s::%s": %w`, namespace, name, err))
		}

		// It's unclear if WASMExtension implementor will correctly handle the context canceled case, as a safety
		// measure, we check if the context was canceled without being handled correctly and stop here.
		if ctx.Err() == context.Canceled {
			panic(fmt.Errorf("running wasm %s@%s extension has been stop upstream in the call stack: %w", namespace, name, ctx.Err()))
		}

		err = m.CurrentInstance.WriteOutputToHeap(outputPtr, out, name)
		if err != nil {
			panic(fmt.Errorf("write output to heap %w", err))
		}
	}
}

func (m *Module) newImports() error {
	linker := m.wasmLinker

	err := m.registerLoggerImports(linker)
	if err != nil {
		return fmt.Errorf("registering logger imports: %w", err)
	}
	err = m.registerStateImports(linker)
	if err != nil {
		return fmt.Errorf("registering state imports: %w", err)
	}

	if err = linker.FuncWrap("env", "register_panic",
		func(msgPtr, msgLength int32, filenamePtr, filenameLength int32, lineNumber, columnNumber int32, caller *wasmtime.Caller) {
			message := m.Heap.ReadString(msgPtr, msgLength)

			var filename string
			if filenamePtr != 0 {
				filename = m.Heap.ReadString(filenamePtr, filenameLength)
			}

			m.CurrentInstance.panicError = &PanicError{message, filename, int(lineNumber), int(columnNumber)}
		},
	); err != nil {
		return fmt.Errorf("registering panic import: %w", err)
	}

	if err = linker.FuncWrap("env", "output",
		func(ptr, length int32) {
			message := m.Heap.ReadBytes(ptr, length)
			m.CurrentInstance.returnValue = make([]byte, length)
			copy(m.CurrentInstance.returnValue, message)
		},
	); err != nil {
		return fmt.Errorf("registering output import: %w", err)
	}

	return nil
}

func (m *Module) registerLoggerImports(linker *wasmtime.Linker) error {
	if err := linker.FuncWrap("logger", "println",
		func(ptr int32, length int32) {
			if m.CurrentInstance.ReachedLogsMaxByteCount() {
				// Early exit, we don't even need to collect the message as we would not store it anyway
				return
			}

			if length > maxLogByteCount {
				panic(fmt.Errorf("message to log is too big, max size is %s", humanize.IBytes(uint64(length))))
			}

			message := m.Heap.ReadString(ptr, length)
			if tracer.Enabled() {
				zlog.Debug(message, zap.String("module_name", m.CurrentInstance.Module.name), zap.String("wasm_file", m.CurrentInstance.Module.name))
			}

			// len(<string>) in Go count number of bytes and not characters, so we are good here
			m.CurrentInstance.LogsByteCount += uint64(len(message))
			if !m.CurrentInstance.ReachedLogsMaxByteCount() {
				m.CurrentInstance.Logs = append(m.CurrentInstance.Logs, message)
				m.CurrentInstance.PushExecutionStack(fmt.Sprintf("log: %s", message))
			}

			return
		},
	); err != nil {
		return fmt.Errorf("registering println import: %w", err)
	}
	return nil
}

type externError struct {
	cause error
}

func newExternError(moduleName string, cause error) *externError {
	return &externError{
		cause: cause,
	}
}

func (e externError) Error() string {
	return e.cause.Error()
}

func returnErrorString(moduleName, cause string) {
	panic(newExternError(moduleName, errors.New(cause)))
}
func returnError(moduleName string, cause error) {
	panic(newExternError(moduleName, cause))
}

func (m *Module) registerStateImports(linker *wasmtime.Linker) error {
	functions := map[string]interface{}{}
	functions["set"] = m.set
	functions["set_if_not_exists"] = m.setIfNotExists
	functions["append"] = m.append
	functions["delete_prefix"] = m.deletePrefix
	functions["add_bigint"] = m.addBigInt
	functions["add_bigdecimal"] = m.addBigDecimal
	functions["add_bigfloat"] = m.addBigDecimal
	functions["add_int64"] = m.addInt64
	functions["add_float64"] = m.addFloat64
	functions["set_min_int64"] = m.setMinInt64
	functions["set_min_bigint"] = m.setMinBigint
	functions["set_min_float64"] = m.setMinFloat64
	functions["set_min_bigdecimal"] = m.setMinBigDecimal
	functions["set_min_bigfloat"] = m.setMinBigDecimal
	functions["set_max_int64"] = m.setMaxInt64
	functions["set_max_bigint"] = m.setMaxBigInt
	functions["set_max_float64"] = m.setMaxFloat64
	functions["set_max_bigdecimal"] = m.setMaxBigDecimal
	functions["set_max_bigfloat"] = m.setMaxBigDecimal
	functions["get_at"] = m.getAt
	functions["get_first"] = m.getFirst
	functions["get_last"] = m.getLast
	functions["has_at"] = m.hasAt
	functions["has_first"] = m.hasFirst
	functions["has_last"] = m.hasLast

	for n, f := range functions {
		if err := linker.FuncWrap("state", n, f); err != nil {
			return fmt.Errorf("registering %s import: %w", n, err)
		}
	}

	return nil
}

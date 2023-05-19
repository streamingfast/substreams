package wasm

import (
	"context"
	"errors"
	"fmt"

	wasmtime "github.com/bytecodealliance/wasmtime-go/v4"
	"github.com/dustin/go-humanize"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/reqctx"
	"go.uber.org/zap"
)

type Instance struct {
	runtime *Runtime

	name string

	CurrentCall  *Call
	entrypoint   string
	wasmInstance *wasmtime.Instance
	wasmEngine   *wasmtime.Engine
	wasmStore    *wasmtime.Store
	wasmModule   *wasmtime.Module
	wasmLinker   *wasmtime.Linker
	Heap         *Heap
	isClosed     bool
}

func (i *Instance) FreeMem() {
	i.wasmStore.FreeMem()
	i.wasmLinker.FreeMem()
	i.wasmEngine.FreeMem()
	i.isClosed = true
}

func (r *Runtime) NewInstance(ctx context.Context, module *Module, name, entrypoint string) (*Instance, error) {

	linker := wasmtime.NewLinker(module.engine)
	store := wasmtime.NewStore(module.engine)

	m := &Instance{
		runtime:    r,
		wasmEngine: module.engine,
		wasmLinker: linker,
		wasmStore:  store,
		wasmModule: module.module,
		name:       name,
		entrypoint: entrypoint,
	}
	if err := m.newImports(); err != nil {
		return nil, fmt.Errorf("instantiating imports: %w", err)
	}
	for namespace, imports := range r.extensions {
		for importName, f := range imports {
			f := m.newExtensionFunction(ctx, namespace, importName, f)
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

func (i *Instance) NewCall(clock *pbsubstreams.Clock, arguments []Argument) (*Call, error) {
	if i.isClosed {
		panic("module is closed")
	}
	export := i.wasmInstance.GetExport(i.wasmStore, i.entrypoint)
	if export == nil {
		return nil, fmt.Errorf("failed to get entrypoint %q most likely does not exist", i.entrypoint)
	}
	entrypoint := export.Func()
	if entrypoint == nil {
		return nil, fmt.Errorf("failed to get exported function %q", entrypoint)
	}

	i.CurrentCall = &Call{
		instance:   i,
		clock:      clock,
		entrypoint: entrypoint,
	}
	if i.runtime.maxFuel != 0 {
		if remaining, _ := i.wasmStore.ConsumeFuel(i.runtime.maxFuel); remaining != 0 {
			i.wasmStore.ConsumeFuel(remaining) // don't accumulate fuel from previous executions
		}
		i.wasmStore.AddFuel(i.runtime.maxFuel)
	}

	var args []interface{}
	for _, input := range arguments {
		switch v := input.(type) {
		case *StoreWriterOutput:
			i.CurrentCall.outputStore = v.Store
			i.CurrentCall.updatePolicy = v.UpdatePolicy
			i.CurrentCall.valueType = v.ValueType
		case *StoreReaderInput:
			i.CurrentCall.inputStores = append(i.CurrentCall.inputStores, v.Store)
			args = append(args, int32(len(i.CurrentCall.inputStores)-1))
		case ValueArgument:
			cnt := v.Value()
			ptr, err := i.Heap.Write(cnt, input.Name())
			if err != nil {
				return nil, fmt.Errorf("writing %s to heap: %w", input.Name(), err)
			}
			length := int32(len(cnt))
			args = append(args, ptr, length)
		default:
			panic("unknown wasm argument type")
		}
	}
	i.CurrentCall.args = args

	return i.CurrentCall, nil
}

func (i *Instance) newExtensionFunction(ctx context.Context, namespace, name string, f WASMExtension) interface{} {
	return func(ptr, length, outputPtr int32) {
		heap := i.Heap

		data := heap.ReadBytes(ptr, length)

		requestID := reqctx.Details(ctx).UniqueIDString()

		out, err := f(ctx, requestID, i.CurrentCall.clock, data)
		if err != nil {
			panic(fmt.Errorf(`running wasm extension "%s::%s": %w`, namespace, name, err))
		}

		// It's unclear if WASMExtension implementor will correctly handle the context canceled case, as a safety
		// measure, we check if the context was canceled without being handled correctly and stop here.
		if ctx.Err() == context.Canceled {
			panic(fmt.Errorf("running wasm %s@%s extension has been stop upstream in the call stack: %w", namespace, name, ctx.Err()))
		}

		err = i.CurrentCall.WriteOutputToHeap(outputPtr, out, name)
		if err != nil {
			panic(fmt.Errorf("write output to heap %w", err))
		}
	}
}

func (i *Instance) newImports() error {
	linker := i.wasmLinker

	err := i.registerLoggerImports(linker)
	if err != nil {
		return fmt.Errorf("registering logger imports: %w", err)
	}
	err = i.registerStateImports(linker)
	if err != nil {
		return fmt.Errorf("registering state imports: %w", err)
	}

	if err = linker.FuncWrap("env", "register_panic",
		func(msgPtr, msgLength int32, filenamePtr, filenameLength int32, lineNumber, columnNumber int32, caller *wasmtime.Caller) {
			message := i.Heap.ReadString(msgPtr, msgLength)

			var filename string
			if filenamePtr != 0 {
				filename = i.Heap.ReadString(filenamePtr, filenameLength)
			}

			i.CurrentCall.panicError = &PanicError{message, filename, int(lineNumber), int(columnNumber)}
		},
	); err != nil {
		return fmt.Errorf("registering panic import: %w", err)
	}

	if err = linker.FuncWrap("env", "output",
		func(ptr, length int32) {
			message := i.Heap.ReadBytes(ptr, length)
			i.CurrentCall.returnValue = make([]byte, length)
			copy(i.CurrentCall.returnValue, message)
		},
	); err != nil {
		return fmt.Errorf("registering output import: %w", err)
	}

	return nil
}

func (i *Instance) registerLoggerImports(linker *wasmtime.Linker) error {
	if err := linker.FuncWrap("logger", "println",
		func(ptr int32, length int32) {
			if i.CurrentCall.ReachedLogsMaxByteCount() {
				// Early exit, we don't even need to collect the message as we would not store it anyway
				return
			}

			if length > maxLogByteCount {
				panic(fmt.Errorf("message to log is too big, max size is %s", humanize.IBytes(uint64(length))))
			}

			message := i.Heap.ReadString(ptr, length)
			if tracer.Enabled() {
				zlog.Debug(message, zap.String("module_name", i.CurrentCall.instance.name), zap.String("wasm_file", i.CurrentCall.instance.name))
			}

			// len(<string>) in Go count number of bytes and not characters, so we are good here
			i.CurrentCall.LogsByteCount += uint64(len(message))
			if !i.CurrentCall.ReachedLogsMaxByteCount() {
				i.CurrentCall.Logs = append(i.CurrentCall.Logs, message)
				i.CurrentCall.PushExecutionStack(fmt.Sprintf("log: %s", message))
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

func (i *Instance) registerStateImports(linker *wasmtime.Linker) error {
	functions := map[string]interface{}{}
	functions["set"] = i.set
	functions["set_if_not_exists"] = i.setIfNotExists
	functions["append"] = i.append
	functions["delete_prefix"] = i.deletePrefix
	functions["add_bigint"] = i.addBigInt
	functions["add_bigdecimal"] = i.addBigDecimal
	functions["add_bigfloat"] = i.addBigDecimal
	functions["add_int64"] = i.addInt64
	functions["add_float64"] = i.addFloat64
	functions["set_min_int64"] = i.setMinInt64
	functions["set_min_bigint"] = i.setMinBigint
	functions["set_min_float64"] = i.setMinFloat64
	functions["set_min_bigdecimal"] = i.setMinBigDecimal
	functions["set_min_bigfloat"] = i.setMinBigDecimal
	functions["set_max_int64"] = i.setMaxInt64
	functions["set_max_bigint"] = i.setMaxBigInt
	functions["set_max_float64"] = i.setMaxFloat64
	functions["set_max_bigdecimal"] = i.setMaxBigDecimal
	functions["set_max_bigfloat"] = i.setMaxBigDecimal
	functions["get_at"] = i.getAt
	functions["get_first"] = i.getFirst
	functions["get_last"] = i.getLast
	functions["has_at"] = i.hasAt
	functions["has_first"] = i.hasFirst
	functions["has_last"] = i.hasLast

	for n, f := range functions {
		if err := linker.FuncWrap("state", n, f); err != nil {
			return fmt.Errorf("registering %s import: %w", n, err)
		}
	}

	return nil
}

package wasmtime

import (
	"context"
	"fmt"

	wasmtime "github.com/bytecodealliance/wasmtime-go/v41"

	"github.com/streamingfast/substreams/reqctx"
	"github.com/streamingfast/substreams/wasm"
)

type instance struct {
	CurrentCall *wasm.Call

	wasmInstance *wasmtime.Instance
	wasmEngine   *wasmtime.Engine
	wasmStore    *wasmtime.Store
	wasmModule   *wasmtime.Module
	wasmLinker   *wasmtime.Linker
	Heap         *Heap
	isClosed     bool
}

func (i *instance) Close(_ context.Context) (_ error) {
	i.wasmStore.Close()
	i.wasmLinker.Close()
	i.wasmStore.Close()
	i.isClosed = true
	return
}

func (i *instance) Cleanup(_ context.Context) error {
	err := i.Heap.Clear()
	if err != nil {
		return fmt.Errorf("clearing heap: %w", err)
	}
	i.wasmStore.GC()
	return nil
}

func (i *instance) newExtensionFunction(ctx context.Context, namespace, name string, f wasm.WASMExtension) any {
	return func(ptr, length, outputPtr int32) {
		data := i.Heap.ReadBytes(ptr, length)

		extension := fmt.Sprintf("%s:%s", namespace, name)
		extStats := reqctx.WasmExtensionReqStats(ctx)

		metricID := extStats.RecordModuleWasmExternalCallBegin(i.CurrentCall.ModuleName, extension)
		out, err := f(ctx, reqctx.Details(ctx).UniqueIDString(), i.CurrentCall.Clock, data)
		// The call must be closed on every path, including errors, otherwise the in-process
		// entry leaks and keeps inflating the reported external call duration forever.
		extStats.RecordModuleWasmExternalCallEnd(i.CurrentCall.ModuleName, extension, metricID)
		if err != nil {
			panic(fmt.Errorf(`running wasm extension "%s::%s": %w`, namespace, name, err))
		}

		// It's unclear if WASMExtension implementor will correctly handle the context canceled case, as a safety
		// measure, we check if the context was canceled without being handled correctly and stop here.
		if ctx.Err() == context.Canceled {
			panic(fmt.Errorf("running wasm %s@%s extension has been stop upstream in the call stack: %w", namespace, name, ctx.Err()))
		}

		if err = writeOutputToHeap(i, outputPtr, out); err != nil {
			panic(fmt.Errorf("write output to heap %w", err))
		}
	}
}

func (i *instance) newImports() error {
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

			i.CurrentCall.SetPanicError(message, filename, int(lineNumber), int(columnNumber))
		},
	); err != nil {
		return fmt.Errorf("registering panic import: %w", err)
	}

	if err = linker.FuncWrap("env", "output",
		func(ptr, length int32) {
			message := i.Heap.ReadBytes(ptr, length)
			i.CurrentCall.SetReturnValue(message)
		},
	); err != nil {
		return fmt.Errorf("registering output import: %w", err)
	}

	if err = linker.FuncWrap("env", "skip_empty_output",
		func() {
			i.CurrentCall.SkipEmptyOutput()
		},
	); err != nil {
		return fmt.Errorf("registering output import: %w", err)
	}

	return nil
}

func (i *instance) registerLoggerImports(linker *wasmtime.Linker) error {
	if err := linker.FuncWrap("logger", "println",
		func(ptr int32, length int32) {
			message := i.Heap.ReadString(ptr, length)
			i.CurrentCall.AppendLog(message)
		},
	); err != nil {
		return fmt.Errorf("registering println import: %w", err)
	}
	return nil
}

func (i *instance) registerStateImports(linker *wasmtime.Linker) error {
	functions := map[string]any{}
	functions["set"] = i.set
	functions["set_if_not_exists"] = i.setIfNotExists
	functions["append"] = i.append
	functions["delete_prefix"] = i.deletePrefix
	functions["set_sum_int64"] = i.setSumInt64
	functions["set_sum_float64"] = i.setSumFloat64
	functions["set_sum_bigint"] = i.setSumBigInt
	functions["set_sum_bigdecimal"] = i.setSumBigDecimal
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
	functions["foundational_store_get"] = i.foundationalStoreGet
	functions["foundational_store_get_all"] = i.foundationalStoreGetAll
	functions["foundational_store_get_entries"] = i.foundationalStoreGetEntries
	functions["foundational_store_get_first_entries"] = i.foundationalStoreGetFirstEntries

	for n, f := range functions {
		if err := linker.FuncWrap("state", n, f); err != nil {
			return fmt.Errorf("registering %s import: %w", n, err)
		}
	}

	return nil
}

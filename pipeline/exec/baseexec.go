package exec

import (
	"context"
	"fmt"
	"os"

	ttrace "go.opentelemetry.io/otel/trace"

	"github.com/streamingfast/substreams/storage/execout"
	"github.com/streamingfast/substreams/wasm"
)

type BaseExecutor struct {
	ctx context.Context

	moduleName    string
	wasmModule    wasm.Module
	wasmArguments []wasm.Argument
	entrypoint    string
	tracer        ttrace.Tracer

	cachedInstance wasm.Instance

	// Results
	logs           []string
	logsTruncated  bool
	executionStack []string
}

func NewBaseExecutor(ctx context.Context, moduleName string, wasmModule wasm.Module, wasmArguments []wasm.Argument, entrypoint string, tracer ttrace.Tracer) *BaseExecutor {
	return &BaseExecutor{ctx: ctx, moduleName: moduleName, wasmModule: wasmModule, wasmArguments: wasmArguments, entrypoint: entrypoint, tracer: tracer}
}

var CACHE_ENABLED = os.Getenv("WAZERO_CACHE_ENABLED") != ""

func (e *BaseExecutor) wasmCall(outputGetter execout.ExecutionOutputGetter) (call *wasm.Call, err error) {
	e.logs = nil
	e.logsTruncated = false
	e.executionStack = nil

	hasInput := false
	for _, input := range e.wasmArguments {
		switch v := input.(type) {
		case *wasm.StoreWriterOutput:
		case *wasm.StoreReaderInput:
			hasInput = true
		case *wasm.ParamsInput:
			hasInput = true
		case wasm.ValueArgument:
			hasInput = true
			data, _, err := outputGetter.Get(v.Name())
			if err != nil {
				return nil, fmt.Errorf("input data for %q: %w", v.Name(), err)
			}
			v.SetValue(data)
		default:
			panic("unknown wasm argument type")
		}
	}
	// This allows us to skip the execution of the VM if there are no inputs.
	// This assumption should either be configurable by the manifest, or clearly documented:
	//  state builders will not be called if their input streams are 0 bytes length (and there is no
	//  state store in read mode)
	if hasInput {
		clock := outputGetter.Clock()
		var mod wasm.Instance
		call = wasm.NewCall(clock, e.moduleName, e.entrypoint, e.wasmArguments)
		mod, err = e.wasmModule.ExecuteNewCall(e.ctx, call, e.cachedInstance, e.wasmArguments)
		if panicErr := call.Err(); panicErr != nil {
			errExecutor := ErrorExecutor{
				message:    panicErr.Error(),
				stackTrace: call.ExecutionStack,
			}
			return nil, fmt.Errorf("block %d: module %q: wasm execution failed: %v", clock.Number, e.moduleName, errExecutor.Error())
		}
		if err != nil {
			return nil, fmt.Errorf("block %d: module %q: general wasm execution failed: %v", clock.Number, e.moduleName, err)
		}
		if CACHE_ENABLED {
			e.cachedInstance = mod
		} else {
			_ = mod.Close(e.ctx)
		}
		e.logs = call.Logs
		e.logsTruncated = call.ReachedLogsMaxByteCount()
		e.executionStack = call.ExecutionStack
	}
	return
}

func (e *BaseExecutor) Close() {
	if e.cachedInstance != nil {
		e.cachedInstance.Close(e.ctx)
	}
}

func (e *BaseExecutor) lastExecutionLogs() (logs []string, truncated bool) {
	return e.logs, e.logsTruncated
}
func (e *BaseExecutor) lastExecutionStack() []string {
	return e.executionStack
}

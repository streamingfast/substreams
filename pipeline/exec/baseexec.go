package exec

import (
	"errors"
	"fmt"

	"github.com/streamingfast/substreams/storage/execout"

	"github.com/streamingfast/substreams/wasm"
	ttrace "go.opentelemetry.io/otel/trace"
)

var ErrWasmDeterministicExec = errors.New("wasm execution failed deterministically")

type BaseExecutor struct {
	moduleName    string
	wasmModule    *wasm.Instance
	wasmArguments []wasm.Argument
	entrypoint    string
	tracer        ttrace.Tracer
}

func NewBaseExecutor(moduleName string, wasmModule *wasm.Instance, wasmArguments []wasm.Argument, entrypoint string, tracer ttrace.Tracer) *BaseExecutor {
	return &BaseExecutor{moduleName: moduleName, wasmModule: wasmModule, wasmArguments: wasmArguments, entrypoint: entrypoint, tracer: tracer}
}
func (e *BaseExecutor) FreeMem() { e.wasmModule.FreeMem() }

func (e *BaseExecutor) moduleLogs() (logs []string, truncated bool) {
	if instance := e.wasmModule.CurrentCall; instance != nil {
		return instance.Logs, instance.ReachedLogsMaxByteCount()
	}
	return
}
func (e *BaseExecutor) currentExecutionStack() []string {
	return e.wasmModule.CurrentCall.ExecutionStack
}

func (e *BaseExecutor) wasmCall(outputGetter execout.ExecutionOutputGetter) (instance *wasm.Call, err error) {
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
		instance, err = e.wasmModule.NewCall(clock, e.wasmArguments)
		if err != nil {
			return nil, fmt.Errorf("new wasm instance: %w", err)
		}

		if err = instance.Execute(); err != nil {
			errExecutor := &ErrorExecutor{
				message:    err.Error(),
				stackTrace: instance.ExecutionStack,
			}
			return nil, fmt.Errorf("block %d: module %q: %w: %s", clock.Number, e.moduleName, ErrWasmDeterministicExec, errExecutor.Error())
		}
		err = instance.Cleanup()

		if err != nil {
			return nil, fmt.Errorf("block %d: module %q: wasm heap clear failed: %w", clock.Number, e.moduleName, err)
		}
	}
	return
}

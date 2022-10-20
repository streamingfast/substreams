package pipeline

import (
	"bytes"
	"context"
	"fmt"
	"github.com/streamingfast/substreams/pipeline/execout"
	"github.com/streamingfast/substreams/reqctx"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/store"
	"github.com/streamingfast/substreams/wasm"
	"go.opentelemetry.io/otel/attribute"
	ttrace "go.opentelemetry.io/otel/trace"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

type ErrorExecutor struct {
	message    string
	stackTrace []string
}

func (e *ErrorExecutor) Error() string {
	b := bytes.NewBuffer(nil)

	b.WriteString(e.message)

	if len(e.stackTrace) > 0 {
		// stack trace section will also contain the logs of the execution
		b.WriteString("\n----- stack trace -----\n")
		for _, stackTraceLine := range e.stackTrace {
			b.WriteString(stackTraceLine)
			b.WriteString("\n")
		}
	}

	return b.String()
}

type ModuleExecutor interface {
	// Name returns the name of the module as defined in the manifest.
	Name() string

	// String returns the module executor representation, usually its name directly.
	String() string

	// Reset the wasm instance, avoid propagating logs.
	Reset()

	run(ctx context.Context, reader execout.ExecutionOutputGetter) (out []byte, moduleOutputData pbsubstreams.ModuleOutputData, err error)
	applyCachedOutput(value []byte) error

	moduleLogs() (logs []string, truncated bool)
	currentExecutionStack() []string
}

type BaseExecutor struct {
	moduleName    string
	wasmModule    *wasm.Module
	wasmArguments []wasm.Argument
	entrypoint    string
	tracer        ttrace.Tracer
}

func (e *BaseExecutor) moduleLogs() (logs []string, truncated bool) {
	if instance := e.wasmModule.CurrentInstance; instance != nil {
		return instance.Logs, instance.ReachedLogsMaxByteCount()
	}
	return
}
func (e *BaseExecutor) currentExecutionStack() []string {
	return e.wasmModule.CurrentInstance.ExecutionStack
}

var _ ModuleExecutor = (*MapperModuleExecutor)(nil)

type MapperModuleExecutor struct {
	BaseExecutor
	outputType string
}

var _ ModuleExecutor = (*StoreModuleExecutor)(nil)

// Name implements ModuleExecutor
func (e *MapperModuleExecutor) Name() string { return e.moduleName }

func (e *MapperModuleExecutor) String() string { return e.Name() }

func (e *MapperModuleExecutor) Reset() { e.wasmModule.CurrentInstance = nil }

func (e *MapperModuleExecutor) applyCachedOutput([]byte) error { return nil }

func (e *MapperModuleExecutor) run(ctx context.Context, reader execout.ExecutionOutputGetter) (out []byte, moduleOutput pbsubstreams.ModuleOutputData, err error) {
	ctx, span := reqctx.WithSpan(ctx, "exec_map")
	defer span.EndWithErr(&err)
	span.SetAttributes(attribute.String("module", e.moduleName))

	var instance *wasm.Instance
	if instance, err = e.wasmCall(reader); err != nil {
		return nil, nil, fmt.Errorf("maps wasm call: %w", err)
	}

	if instance != nil {
		out = instance.Output()
	}

	if out != nil {
		moduleOutput = &pbsubstreams.ModuleOutput_MapOutput{
			MapOutput: &anypb.Any{TypeUrl: "type.googleapis.com/" + e.outputType, Value: out},
		}
	}

	return out, moduleOutput, nil
}

type StoreModuleExecutor struct {
	BaseExecutor
	outputStore store.DeltaAccessor
}

func (e *StoreModuleExecutor) Name() string { return e.moduleName }

func (e *StoreModuleExecutor) String() string { return e.Name() }

func (e *StoreModuleExecutor) Reset() { e.wasmModule.CurrentInstance = nil }

func (e *StoreModuleExecutor) applyCachedOutput(value []byte) error {
	deltas := &pbsubstreams.StoreDeltas{}
	err := proto.Unmarshal(value, deltas)
	if err != nil {
		return fmt.Errorf("unmarshalling output deltas: %w", err)
	}
	e.outputStore.SetDeltas(deltas.Deltas)
	return nil

}

func (e *StoreModuleExecutor) run(ctx context.Context, reader execout.ExecutionOutputGetter) (out []byte, moduleOutput pbsubstreams.ModuleOutputData, err error) {
	ctx, span := reqctx.WithSpan(ctx, "exec_store")
	defer span.EndWithErr(&err)
	span.SetAttributes(attribute.String("module", e.moduleName))
	if _, err := e.wasmCall(reader); err != nil {
		return nil, nil, fmt.Errorf("store wasm call: %w", err)
	}

	deltas := &pbsubstreams.StoreDeltas{
		Deltas: e.outputStore.GetDeltas(),
	}

	data, err := proto.Marshal(deltas)
	if err != nil {
		return nil, nil, fmt.Errorf("caching: marshalling delta: %w", err)
	}

	moduleOutput = &pbsubstreams.ModuleOutput_StoreDeltas{
		StoreDeltas: deltas,
	}

	return data, moduleOutput, nil
}

func (e *BaseExecutor) wasmCall(outputGetter execout.ExecutionOutputGetter) (instance *wasm.Instance, err error) {
	hasInput := false
	for _, input := range e.wasmArguments {
		switch v := input.(type) {
		case *wasm.StoreWriterOutput:
		case *wasm.StoreReaderInput:
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
	//  state builders will not be called if their input streams are 0 bytes length (and there'e no
	//  state store in read mode)
	if hasInput {
		clock := outputGetter.Clock()
		instance, err = e.wasmModule.NewInstance(clock, e.wasmArguments)
		if err != nil {
			return nil, fmt.Errorf("new wasm instance: %w", err)
		}

		if err = instance.Execute(); err != nil {
			errExecutor := ErrorExecutor{
				message:    err.Error(),
				stackTrace: instance.ExecutionStack,
			}
			return nil, fmt.Errorf("block %d: module %q: wasm execution failed: %v", clock.Number, e.moduleName, errExecutor.Error())
		}
		err = instance.Module.Heap.Clear()
		if err != nil {
			return nil, fmt.Errorf("block %d: module %q: wasm heap clear failed: %w", clock.Number, e.moduleName, err)
		}
	}
	return
}

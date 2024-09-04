package exec

import (
	"context"
	"errors"
	"fmt"

	"github.com/streamingfast/substreams/reqctx"
	"github.com/streamingfast/substreams/storage/execout"
	"github.com/streamingfast/substreams/storage/index"
	"github.com/streamingfast/substreams/wasm"
	ttrace "go.opentelemetry.io/otel/trace"
)

var ErrWasmDeterministicExec = errors.New("wasm execution failed deterministically")

type BaseExecutor struct {
	ctx context.Context

	moduleName    string
	initialBlock  uint64
	wasmModule    wasm.Module
	wasmArguments []wasm.Argument
	entrypoint    string
	blockIndex    *index.BlockIndex
	tracer        ttrace.Tracer

	instanceCacheEnabled bool
	cachedInstance       wasm.Instance

	// Results
	logs           []string
	logsTruncated  bool
	executionStack []string
}

func NewBaseExecutor(ctx context.Context, moduleName string, initialBlock uint64, wasmModule wasm.Module, cacheEnabled bool, wasmArguments []wasm.Argument, blockIndex *index.BlockIndex, entrypoint string, tracer ttrace.Tracer) *BaseExecutor {
	return &BaseExecutor{
		ctx:                  ctx,
		initialBlock:         initialBlock,
		blockIndex:           blockIndex,
		moduleName:           moduleName,
		wasmModule:           wasmModule,
		instanceCacheEnabled: cacheEnabled,
		wasmArguments:        wasmArguments,
		entrypoint:           entrypoint,
		tracer:               tracer,
	}
}

// var Timer time.Duration
var ErrNoInput = errors.New("no input")
var ErrSkippedOutput = errors.New("skipped output") // willfully skipped output (through intrinsic)

// getWasmArgumentValues return the values for each argument of type wasm.ValueArgument.
// An empty value is returned as an empty byte slice, while a missing (skipped) value is returned as nil.
func getWasmArgumentValues(wasmArguments []wasm.Argument, outputGetter execout.ExecutionOutputGetter) (map[string][]byte, error) {
	out := make(map[string][]byte)
	for i, input := range wasmArguments {
		switch v := input.(type) {
		case *wasm.MapInput, *wasm.StoreDeltaInput, *wasm.SourceInput:
			val, _, err := outputGetter.Get((v.Name()))
			if err != nil {
				if errors.Is(err, execout.ErrNotFound) {
					out[v.Name()] = nil // skipped inputs are exposed to the wasm module as nil values
					break
				}
				return nil, fmt.Errorf("input data for %q, param %d: %w", v.Name(), i, err)
			}
			if val == nil {
				out[v.Name()] = []byte{} // empty inputs that are not skipped are exposed as an empty byte slice
			} else {
				out[v.Name()] = val
			}
		}
	}
	return out, nil
}

func canSkipExecution(wasmArgumentValues map[string][]byte) bool {
	if wasmArgumentValues["sf.substreams.v1.Clock"] != nil && len(wasmArgumentValues) == 1 {
		return false // never skip if the only 'ArgumentValue' input is a clock
	}

	for k, v := range wasmArgumentValues {
		if v != nil && k != "sf.substreams.v1.Clock" {
			return false
		}
	}

	// we have no input to send
	return true
}

func (e *BaseExecutor) wasmCall(outputGetter execout.ExecutionOutputGetter) (call *wasm.Call, err error) {
	e.logs = nil
	e.logsTruncated = false
	e.executionStack = nil

	argValues, err := getWasmArgumentValues(e.wasmArguments, outputGetter)
	if err != nil {
		return nil, err
	}

	if canSkipExecution(argValues) {
		return nil, ErrNoInput
	}

	clock := outputGetter.Clock()
	var inst wasm.Instance

	stats := reqctx.ReqStats(e.ctx)
	//t0 := time.Now()
	call = wasm.NewCall(clock, e.moduleName, e.entrypoint, stats, e.wasmArguments)
	inst, err = e.wasmModule.ExecuteNewCall(e.ctx, call, e.cachedInstance, e.wasmArguments, argValues)
	//Timer += time.Since(t0)
	if panicErr := call.Err(); panicErr != nil {
		errExecutor := &ErrorExecutor{
			message:    panicErr.Error(),
			stackTrace: call.ExecutionStack,
		}
		return nil, fmt.Errorf("block %d: module %q: general wasm execution panicked: %w: %s", clock.Number, e.moduleName, ErrWasmDeterministicExec, errExecutor.Error())
	}
	if err != nil {
		if ctxErr := e.ctx.Err(); ctxErr != nil {
			return nil, fmt.Errorf("block %d: module %q: general wasm execution failed: %w, %w", clock.Number, e.moduleName, err, ctxErr)
		}
		return nil, fmt.Errorf("block %d: module %q: general wasm execution failed: %w: %s", clock.Number, e.moduleName, ErrWasmDeterministicExec, err)
	}
	if e.instanceCacheEnabled {
		if err := inst.Cleanup(e.ctx); err != nil {
			return nil, fmt.Errorf("block %d: module %q: failed to cleanup module: %w", clock.Number, e.moduleName, err)
		}
		e.cachedInstance = inst
	} else {
		if err := inst.Close(e.ctx); err != nil {
			return nil, fmt.Errorf("block %d: module %q: failed to close module: %w", clock.Number, e.moduleName, err)
		}
	}
	e.logs = call.Logs
	e.logsTruncated = call.ReachedLogsMaxByteCount()
	e.executionStack = call.ExecutionStack
	return
}

func (e *BaseExecutor) BlockIndex() *index.BlockIndex {
	return e.blockIndex
}

func (e *BaseExecutor) RunsOnBlock(blockNum uint64) bool {
	return blockNum >= e.initialBlock
}

func (e *BaseExecutor) Close(ctx context.Context) error {
	if e.cachedInstance != nil {
		return e.cachedInstance.Close(ctx)
	}
	return nil
}

func (e *BaseExecutor) lastExecutionLogs() (logs []string, truncated bool) {
	return e.logs, e.logsTruncated
}
func (e *BaseExecutor) lastExecutionStack() []string {
	return e.executionStack
}

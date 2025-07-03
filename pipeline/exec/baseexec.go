package exec

import (
	"context"
	"errors"
	"fmt"

	"github.com/streamingfast/substreams/client/fstore"
	"github.com/streamingfast/substreams/metrics"
	"github.com/streamingfast/substreams/reqctx"
	"github.com/streamingfast/substreams/storage/execout"
	"github.com/streamingfast/substreams/storage/index"
	"github.com/streamingfast/substreams/wasm"
	ttrace "go.opentelemetry.io/otel/trace"
)

type BaseExecutor struct {
	ctx context.Context

	moduleName    string
	moduleHash    string
	initialBlock  uint64
	wasmModule    wasm.Module
	wasmArguments []wasm.Argument
	entrypoint    string
	blockIndex    *index.BlockIndex
	tracer        ttrace.Tracer

	instanceCacheEnabled    bool
	cachedInstance          wasm.Instance
	foundationalStoreReader fstore.FoundationalReader

	// Results
	logs          []string
	logsTruncated bool
}

func NewBaseExecutor(ctx context.Context, moduleName, moduleHash string, initialBlock uint64, wasmModule wasm.Module, cacheEnabled bool, wasmArguments []wasm.Argument, blockIndex *index.BlockIndex, entrypoint string, tracer ttrace.Tracer, foundationalStoreReader fstore.FoundationalReader) *BaseExecutor {
	return &BaseExecutor{
		ctx:                     ctx,
		initialBlock:            initialBlock,
		blockIndex:              blockIndex,
		moduleName:              moduleName,
		moduleHash:              moduleHash,
		wasmModule:              wasmModule,
		instanceCacheEnabled:    cacheEnabled,
		wasmArguments:           wasmArguments,
		entrypoint:              entrypoint,
		tracer:                  tracer,
		foundationalStoreReader: foundationalStoreReader,
	}
}

// var Timer time.Duration
var ErrNoInput = errors.New("no input")
var ErrSkippedOutput = errors.New("skipped output") // willfully skipped output (through intrinsic)

// getWasmArgumentValues return the values for each argument of type wasm.ValueArgument.
// An empty value is returned as an empty byte slice, while a missing (skipped) value is returned as nil.
func getWasmArgumentValues(wasmArguments []wasm.Argument, outputGetter execout.ExecutionOutputGetter) (out map[string][]byte, hasSingleParams bool, err error) {
	out = make(map[string][]byte)
	for i, input := range wasmArguments {
		switch v := input.(type) {
		case *wasm.MapInput, *wasm.StoreDeltaInput, *wasm.SourceInput:
			val, _, err := outputGetter.Get((v.Name()))
			if err != nil {
				if errors.Is(err, execout.ErrNotFound) {
					out[v.Name()] = nil // skipped inputs are exposed to the wasm module as nil values
					break
				}
				return nil, false, fmt.Errorf("input data for %q, param %d: %w", v.Name(), i, err)
			}
			if val == nil {
				out[v.Name()] = []byte{} // empty inputs that are not skipped are exposed as an empty byte slice
			} else {
				out[v.Name()] = val
			}

		case *wasm.ParamsInput:
			// special case for 'graph-node head tracker substreams'
			if len(wasmArguments) == 1 {
				hasSingleParams = true
			}
		}
	}
	return
}

func canSkipExecution(wasmArgumentValues map[string][]byte, hasSingleParams bool) bool {
	if hasSingleParams {
		return false
	}
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

func (e *BaseExecutor) wasmCall(outputGetter execout.ExecutionOutputGetter, canSkipEmptyOutput bool, sharedCache *SharedCache) (call *wasm.Call, err error) {
	e.logs = nil
	e.logsTruncated = false

	argValues, hasSingleParams, err := getWasmArgumentValues(e.wasmArguments, outputGetter)
	if err != nil {
		return nil, err
	}

	if canSkipExecution(argValues, hasSingleParams) {
		return nil, ErrNoInput
	}

	clock := outputGetter.Clock()
	var inst wasm.Instance

	stats := reqctx.ReqStats(e.ctx)
	//t0 := time.Now()
	call = wasm.NewCall(clock, e.moduleName, e.entrypoint, stats, e.wasmArguments, canSkipEmptyOutput, e.foundationalStoreReader)

	if sharedCache.Cachable(clock.Number) {
		err = sharedCache.Execute(e.ctx, e.wasmModule, e.moduleHash, call, e.wasmArguments, argValues)
	} else {
		inst, err = e.wasmModule.ExecuteNewCall(e.ctx, call, e.cachedInstance, e.wasmArguments, argValues)
		metrics.ExecutedWasmModules.Inc()
	}

	//Timer += time.Since(t0)
	if panicErr := call.Err(); panicErr != nil {
		errExecutor := &ErrorExecutor{
			message:    panicErr.Error(),
			stackTrace: call.Logs,
		}
		return nil, fmt.Errorf("block %d: module %q: general wasm execution panicked: %w: %s", clock.Number, e.moduleName, wasm.ErrWasmDeterministicExec, errExecutor.Error())
	}
	if err != nil {
		if ctxErr := e.ctx.Err(); ctxErr != nil {
			return nil, fmt.Errorf("block %d: module %q: general wasm execution failed: %w, %w", clock.Number, e.moduleName, err, ctxErr)
		}
		return nil, fmt.Errorf("block %d: module %q: general wasm execution failed: %w: %s", clock.Number, e.moduleName, wasm.ErrWasmDeterministicExec, err)
	}
	if inst != nil {
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
	}
	e.logs = call.Logs
	e.logsTruncated = call.ReachedLogsMaxByteCount()
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

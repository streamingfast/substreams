package exec

import (
	"context"
	"errors"
	"fmt"

	ttrace "go.opentelemetry.io/otel/trace"

	"github.com/RoaringBitmap/roaring/roaring64"
	"github.com/streamingfast/substreams/reqctx"
	"github.com/streamingfast/substreams/sqe"
	"github.com/streamingfast/substreams/storage/execout"
	"github.com/streamingfast/substreams/wasm"
)

var ErrWasmDeterministicExec = errors.New("wasm execution failed deterministically")

type BaseExecutor struct {
	ctx context.Context

	moduleName    string
	initialBlock  uint64
	wasmModule    wasm.Module
	wasmArguments []wasm.Argument
	entrypoint    string
	blockIndices  *roaring64.Bitmap // pre-applied

	blockIndexExpression sqe.Expression // applied on-the-fly, from the block index module outputs
	blockIndexModule     string
	tracer               ttrace.Tracer

	instanceCacheEnabled bool
	cachedInstance       wasm.Instance

	// Results
	logs           []string
	logsTruncated  bool
	executionStack []string
}

func NewBaseExecutor(ctx context.Context, moduleName string, initialBlock uint64, wasmModule wasm.Module, cacheEnabled bool, wasmArguments []wasm.Argument, blockIndices *roaring64.Bitmap, blockIndexExpression sqe.Expression, blockIndexModule, entrypoint string, tracer ttrace.Tracer) *BaseExecutor {
	return &BaseExecutor{
		ctx:                  ctx,
		initialBlock:         initialBlock,
		moduleName:           moduleName,
		blockIndices:         blockIndices,
		blockIndexExpression: blockIndexExpression,
		blockIndexModule:     blockIndexModule,
		wasmModule:           wasmModule,
		instanceCacheEnabled: cacheEnabled,
		wasmArguments:        wasmArguments,
		entrypoint:           entrypoint,
		tracer:               tracer,
	}
}

//var Timer time.Duration

func (e *BaseExecutor) wasmCall(outputGetter execout.ExecutionOutputGetter) (call *wasm.Call, err error) {
	e.logs = nil
	e.logsTruncated = false
	e.executionStack = nil

	hasInput := false
	for i, input := range e.wasmArguments {
		switch v := input.(type) {
		case *wasm.StoreWriterOutput:
		case *wasm.StoreReaderInput:
			hasInput = true
		case *wasm.ParamsInput:
			hasInput = true
		case wasm.ValueArgument:
			if !v.Active(outputGetter.Clock().Number) {
				break // skipping input that is not active at this block
			}
			hasInput = true
			data, _, err := outputGetter.Get(v.Name())
			if err != nil {
				return nil, fmt.Errorf("input data for %q, param %d: %w", v.Name(), i, err)
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
		var inst wasm.Instance

		stats := reqctx.ReqStats(e.ctx)
		//t0 := time.Now()
		call = wasm.NewCall(clock, e.moduleName, e.entrypoint, stats, e.wasmArguments)
		inst, err = e.wasmModule.ExecuteNewCall(e.ctx, call, e.cachedInstance, e.wasmArguments)
		//Timer += time.Since(t0)
		if panicErr := call.Err(); panicErr != nil {
			errExecutor := &ErrorExecutor{
				message:    panicErr.Error(),
				stackTrace: call.ExecutionStack,
			}
			return nil, fmt.Errorf("block %d: module %q: general wasm execution panicked: %w: %s", clock.Number, e.moduleName, ErrWasmDeterministicExec, errExecutor.Error())
		}
		if err != nil {
			if err := e.ctx.Err(); err != nil {
				return nil, fmt.Errorf("block %d: module %q: general wasm execution failed: %w", clock.Number, e.moduleName, err)
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
	}
	return
}

func (e *BaseExecutor) BlockIndices() *roaring64.Bitmap {
	return e.blockIndices
}

func (e *BaseExecutor) BlockIndexExpression() sqe.Expression {
	return e.blockIndexExpression
}

func (e *BaseExecutor) BlockIndexModule() string {
	return e.blockIndexModule
}

func (e *BaseExecutor) RunsOnBlock(blockNum uint64) bool {
	return blockNum >= e.initialBlock
}

func (e *BaseExecutor) BlockIndexExcludesAllBlocks() bool {
	if e.blockIndices != nil && e.blockIndices.IsEmpty() {
		return true
	}
	return false
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

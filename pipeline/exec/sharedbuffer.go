package exec

import (
	"context"
	"fmt"
	"sync"

	pbbstream "github.com/streamingfast/bstream/pb/sf/bstream/v1"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/reqctx"
	"github.com/streamingfast/substreams/wasm"
	"go.uber.org/atomic"
)

var GlobalSharedBuffer = NewSharedBuffer()

type SharedBuffer struct {
	sync.Mutex
	headBlock *atomic.Uint64
	size      uint64
	// module_hash -> blockHash -> callResult
	callResults map[string]map[string]*callResult
}

func NewSharedBuffer() *SharedBuffer {
	return &SharedBuffer{
		headBlock:   atomic.NewUint64(0),
		size:        10,
		callResults: make(map[string]map[string]*callResult),
	}
}

func (s *SharedBuffer) ProcessBlock(blk *pbbstream.Block, _ interface{}) error {
	s.headBlock.Store(blk.Number)
	return nil
}

type callResult struct {
	sync.Mutex
	clock          *pbsubstreams.Clock
	moduleName     string
	entrypoint     string
	logs           []string
	logsByteCount  uint64
	executionStack []string
	logsTruncated  bool
	returnValue    []byte
	executed       bool

	err      error
	panicErr *wasm.PanicError
}

func applyResult(res *callResult, call *wasm.Call) error {
	if call.Clock.Id != res.clock.Id ||
		call.Entrypoint != res.entrypoint ||
		call.ModuleName != res.moduleName {
		panic(fmt.Sprintf("invalid shared buffer data on block %s (%s) for module %s (%s)", call.Clock, res.clock, call.ModuleName, res.moduleName))
	}

	call.Logs = append([]string{}, res.logs...)
	call.LogsByteCount = res.logsByteCount
	call.PanicError = res.panicErr
	call.ExecutionStack = append([]string{}, res.executionStack...)
	call.SetReturnValue(res.returnValue)
	return res.err
}

func (res *callResult) updateFromCall(call *wasm.Call, err error) {
	res.clock = call.Clock
	res.moduleName = call.ModuleName
	res.entrypoint = call.Entrypoint
	res.err = err
	res.panicErr = call.PanicError
	res.returnValue = call.Output()
	res.logs = append([]string{}, call.Logs...)
	res.logsByteCount = call.LogsByteCount
	res.executionStack = append([]string{}, call.ExecutionStack...)
	res.executed = true
}

func newCtx(ctx context.Context) context.Context {
	details := reqctx.Details(ctx)
	return reqctx.WithRequest(context.Background(), details)
}

func (s *SharedBuffer) Execute(
	ctx context.Context,
	wasmModule wasm.Module,
	moduleHash string,
	call *wasm.Call,
	wasmArguments []wasm.Argument,
	argValues map[string][]byte,
) error {
	clock := call.Clock

	s.Lock()

	if s.callResults[clock.Id] == nil {
		s.callResults[clock.Id] = make(map[string]*callResult)
	}
	result, ok := s.callResults[clock.Id][moduleHash]
	if !ok {
		result = &callResult{}
		s.callResults[clock.Id][moduleHash] = result
	}
	result.Lock()
	defer result.Unlock()

	s.Unlock()

	if result.executed {
		return applyResult(result, call)
	}

	ctx = newCtx(ctx) // detach from executing context
	inst, err := wasmModule.ExecuteNewCall(ctx, call, nil, wasmArguments, argValues)
	inst.Close(ctx)
	result.updateFromCall(call, err)

	return err
}

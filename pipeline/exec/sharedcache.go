package exec

import (
	"context"
	"fmt"
	"sync"

	pbbstream "github.com/streamingfast/bstream/pb/sf/bstream/v1"
	"github.com/streamingfast/substreams/metrics"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/wasm"
	"go.uber.org/atomic"
	"go.uber.org/zap"
)

var GlobalSharedCache *SharedCache

type SharedCache struct {
	sync.Mutex
	headBlock *atomic.Uint64
	size      uint64
	// module_hash -> blockHash -> callEntry
	callEntries map[string]map[string]*callEntry
}

const maxUint64 uint64 = 1<<64 - 1

func NewSharedCache(size uint64) *SharedCache {
	return &SharedCache{
		headBlock:   atomic.NewUint64(maxUint64),
		size:        size,
		callEntries: make(map[string]map[string]*callEntry),
	}
}

func (s *SharedCache) ProcessBlock(blk *pbbstream.Block, _ interface{}) error {
	if tracer.Enabled() {
		zlog.Debug("shared_cache process_block", zap.Uint64("block_num", blk.Number))
	}
	s.headBlock.Store(blk.Number)
	s.cleanup(blk.Number)
	return nil
}

func (s *SharedCache) cleanup(head uint64) {
	if head < s.size {
		return
	}
	// cleanup
	lowest := head - s.size
	s.Lock()
	for clockID, hashesInClock := range s.callEntries {
		for entryID, entry := range hashesInClock {
			if entry.clock.Number < lowest {
				delete(hashesInClock, entryID)
				if tracer.Enabled() {
					zlog.Debug("deleting entry", zap.Uint64("clock_num", entry.clock.Number))
				}
			}
		}
		if len(hashesInClock) == 0 {
			delete(s.callEntries, clockID)
			if tracer.Enabled() {
				zlog.Debug("deleting clockID", zap.String("clock_id", clockID))
			}
		}

	}
	s.Unlock()
}

type callEntry struct {
	sync.RWMutex
	clock          *pbsubstreams.Clock
	moduleName     string
	entrypoint     string
	logs           []string
	logsByteCount  uint64
	executionStack []string
	returnValue    []byte
	executed       bool

	err      error
	panicErr *wasm.PanicError
}

func applyResult(res *callEntry, call *wasm.Call) error {
	if call.Clock.Id != res.clock.Id ||
		call.Entrypoint != res.entrypoint ||
		call.ModuleName != res.moduleName {
		panic(fmt.Sprintf("invalid shared cache data on block %s (%s) for module %s (%s)", call.Clock, res.clock, call.ModuleName, res.moduleName))
	}

	call.Logs = append([]string{}, res.logs...)
	call.LogsByteCount = res.logsByteCount
	call.PanicError = res.panicErr
	call.ExecutionStack = append([]string{}, res.executionStack...)
	call.SetReturnValue(res.returnValue)
	return res.err
}

func (res *callEntry) updateFromCall(call *wasm.Call, err error) {
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

func (s *SharedCache) Cachable(blockNum uint64) bool {
	resp := s != nil && blockNum+s.size > s.headBlock.Load()
	return resp
}

func (s *SharedCache) Execute(
	ctx context.Context,
	wasmModule wasm.Module,
	moduleHash string,
	call *wasm.Call,
	wasmArguments []wasm.Argument,
	argValues map[string][]byte,
) error {
	clock := call.Clock
	var resultLockOwner bool

	s.Lock()

	if s.callEntries[clock.Id] == nil {
		s.callEntries[clock.Id] = make(map[string]*callEntry)
	}
	result, ok := s.callEntries[clock.Id][moduleHash]
	if !ok {
		result = &callEntry{clock: clock}
		s.callEntries[clock.Id][moduleHash] = result
		resultLockOwner = true
		result.Lock()
		defer result.Unlock()
	}

	s.Unlock()

	if !resultLockOwner {
		result.RLock()
		defer result.RUnlock()
	}

	if result.executed {
		if tracer.Enabled() {
			zlog.Debug("getting wasm call from cache", zap.String("module_hash", moduleHash), zap.Uint64("block_num", clock.Number))
		}
		metrics.SkippedCachedWasmModules.Inc()
		return applyResult(result, call)
	}

	if tracer.Enabled() {
		zlog.Debug("executing wasm call", zap.String("module_hash", moduleHash), zap.Uint64("block_num", clock.Number))
	}
	metrics.ExecutedWasmModules.Inc()
	ctx = context.TODO()
	inst, err := wasmModule.ExecuteNewCall(ctx, call, nil, wasmArguments, argValues)
	inst.Close(ctx)
	result.updateFromCall(call, err)

	return err
}

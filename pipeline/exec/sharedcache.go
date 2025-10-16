package exec

import (
	"context"
	"fmt"
	"math"
	"sync"

	pbbstream "github.com/streamingfast/bstream/pb/sf/bstream/v1"
	"github.com/streamingfast/substreams/metrics"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/reqctx"
	"github.com/streamingfast/substreams/wasm"
	"go.uber.org/atomic"
	"go.uber.org/zap"
)

var GlobalSharedCache *SharedCache

type SharedCache struct {
	sync.Mutex
	headBlock  *atomic.Uint64
	sizeBlocks uint64
	// module_hash -> blockHash -> callEntry
	callEntries map[cacheClock]map[string]*callEntry
}

func NewSharedCache(sizeBlocks uint64) *SharedCache {
	return &SharedCache{
		headBlock:   atomic.NewUint64(math.MaxUint64),
		sizeBlocks:  sizeBlocks,
		callEntries: make(map[cacheClock]map[string]*callEntry),
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

type cacheClock struct {
	id  string
	num uint64
}

func (s *SharedCache) cleanup(head uint64) {
	if head < s.sizeBlocks {
		return
	}

	// cleanup
	lowest := head - s.sizeBlocks
	s.Lock()
	defer s.Unlock()

	for clock := range s.callEntries {
		if clock.num < lowest {
			delete(s.callEntries, clock)
			if tracer.Enabled() {
				zlog.Debug("deleting cached entries", zap.Uint64("clock_num", clock.num), zap.String("clock_id", clock.id))
			}
		}
	}
}

type callEntry struct {
	sync.RWMutex
	clock           *pbsubstreams.Clock
	moduleName      string
	entrypoint      string
	logs            []string
	logsByteCount   uint64
	executionStack  []string
	returnValue     []byte
	metricsGatherer *metrics.WasmMetricsGatherer

	err      error
	panicErr *wasm.PanicError
}

func applyResult(res *callEntry, call *wasm.Call) error {
	if call.Clock.Id != res.clock.Id || call.Entrypoint != res.entrypoint {
		panic(fmt.Sprintf(
			"invalid shared cache data on block %d id=%s call{module=%s entrypoint=%s} cached{module=%s entrypoint=%s}",
			call.Clock.Number, call.Clock.Id,
			call.ModuleName, call.Entrypoint,
			res.moduleName, res.entrypoint,
		))
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
}

func (s *SharedCache) Cachable(blockNum uint64) bool {
	return s != nil && blockNum+s.sizeBlocks > s.headBlock.Load()
}

func (s *SharedCache) Execute(
	originalContext context.Context,
	wasmModule wasm.Module,
	moduleHash string,
	call *wasm.Call,
	wasmArguments []wasm.Argument,
	argValues map[string][]byte,
	undoManager *UndoManager,
) error {
	clock := cacheClock{
		id:  call.Clock.Id,
		num: call.Clock.Number,
	}

	s.Lock() // do not return before unlocking this

	if s.callEntries[clock] == nil {
		s.callEntries[clock] = make(map[string]*callEntry)
	}
	result, found := s.callEntries[clock][moduleHash]
	if !found {
		result = &callEntry{
			clock:      call.Clock,
			moduleName: call.ModuleName,
			entrypoint: call.Entrypoint,
		}
		s.callEntries[clock][moduleHash] = result

		// this request will actually cause the WASM execution. It locks the 'result' object for writes until it populates it
		result.Lock()
		defer result.Unlock()
	}

	s.Unlock() // This lock is global, it should never wait for an execution!

	if !found {
		if tracer.Enabled() {
			// zlog.Debug("executing wasm call", zap.String("module_hash", moduleHash), zap.Uint64("block_num", clock.num))
			zlog.Info("executing wasm call",
				zap.Uint64("block_num", clock.num),
				zap.String("block_id", clock.id),
				zap.String("module_hash", moduleHash),
				zap.String("module_name", call.ModuleName),
				zap.String("entrypoint", call.Entrypoint),
			)
		}
		metrics.ExecutedWasmModules.Inc()
		result.metricsGatherer = &metrics.WasmMetricsGatherer{}

		// we create a context with just enough for the wasm module executor to work: the UniqueID, and a stats gatherer
		ctx := reqctx.WithWasmExtensionReqStats(context.TODO(), result.metricsGatherer)
		ctx = reqctx.WithRequest(ctx, &reqctx.RequestDetails{
			UniqueID: reqctx.Details(originalContext).UniqueID,
		})

		if undoManager != nil {
			var unsubscribe func()
			ctx, unsubscribe = undoManager.Subscribe(ctx, clock.id)
			defer unsubscribe()
		}

		inst, err := wasmModule.ExecuteNewCall(ctx, call, nil, wasmArguments, argValues)
		inst.Close(ctx)
		result.updateFromCall(call, err)
		result.metricsGatherer.ApplyToStats(reqctx.ReqStats(originalContext))

		return err
	}

	// the "second" request (and subsequent requests) waits here for the "first requestor" to finish the WASM execution
	result.RLock()
	defer result.RUnlock()

	if tracer.Enabled() {
		// zlog.Debug("getting wasm call from cache", zap.String("module_hash", moduleHash), zap.Uint64("block_num", clock.num))
		zlog.Info("getting wasm call from cache",
			zap.Uint64("block_num", clock.num),
			zap.String("block_id", clock.id),
			zap.String("module_hash_wasm_binary", moduleHash),
			zap.String("module_name", call.ModuleName),
			zap.String("entrypoint", call.Entrypoint),
			zap.String("cached_module_name", result.moduleName),
			zap.String("cached_entrypoint", result.entrypoint),
		)
	}
	metrics.SkippedCachedWasmModules.Inc()
	result.metricsGatherer.ApplyToStats(reqctx.ReqStats(originalContext))
	return applyResult(result, call)
}

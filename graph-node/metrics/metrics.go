package metrics

import (
	"fmt"
	"time"

	"github.com/streamingfast/bstream"
	"go.uber.org/zap/zapcore"
)

type ExecutionTime struct {
	TotalExecution   time.Duration
	WaitForBlock     time.Duration
	UnmarshalBlock   time.Duration
	BlockProc        time.Duration
	Rpc              time.Duration
	StoreFlush       time.Duration
	StoreUpdatesOnly time.Duration
	StoreInsertsOnly time.Duration
	SelectQueries    time.Duration
	FullLoadTime     time.Duration

	CacheLookupTime time.Duration
	CacheWriteTime  time.Duration

	SelectQueriesDurations map[string]time.Duration
	SelectQueriesCounts    map[string]int64

	StoreSave int64
	StoreCall int64
	Count     int64
}

func (e *ExecutionTime) Clean() {
	e.TotalExecution = 0
	e.WaitForBlock = 0
	e.UnmarshalBlock = 0
	e.BlockProc = 0
	e.Rpc = 0
	e.StoreFlush = 0
	e.StoreUpdatesOnly = 0
	e.Count = 0
	e.SelectQueries = 0
	e.StoreSave = 0
	e.StoreCall = 0

	e.SelectQueriesDurations = make(map[string]time.Duration)
	e.SelectQueriesCounts = make(map[string]int64)
}

func (e *ExecutionTime) Finalize(t time.Duration) {
	e.TotalExecution += t
	e.Count++
}

func (e *ExecutionTime) String() string {
	avgTotalExecution := time.Duration(int64(e.TotalExecution) / e.Count)
	avgWaitForBlock := time.Duration(int64(e.WaitForBlock) / e.Count)
	avgWaitForBlockRatio := float64(avgWaitForBlock) / float64(avgTotalExecution) * 100.0
	avgUnmarshalBlock := time.Duration(int64(e.UnmarshalBlock) / e.Count)
	avgUnmarshalBlockRatio := float64(avgUnmarshalBlock) / float64(avgTotalExecution) * 100.0
	avgBlockProcExecution := time.Duration(int64(e.BlockProc) / e.Count)
	avgBlockProcExecutionRatio := float64(avgBlockProcExecution) / float64(avgTotalExecution) * 100.0
	avgRpcExecution := time.Duration(int64(e.Rpc) / e.Count)
	avgRpcExecutionRatio := float64(avgRpcExecution) / float64(avgTotalExecution) * 100.0
	avgStoreFlushExecution := time.Duration(int64(e.StoreFlush) / e.Count)
	avgSelectQueriesExecution := time.Duration(int64(e.SelectQueries) / e.Count)
	avgSelectQueriesExecutionRatio := float64(avgSelectQueriesExecution) / float64(avgTotalExecution) * 100.0
	avgStoreSave := e.StoreSave / e.Count

	avgStoreFlushExecutionRatio := float64(avgStoreFlushExecution) / float64(avgTotalExecution) * 100.0
	var avgStoreUpdatesOnlyExecutionRatio float64

	if (e.StoreUpdatesOnly + e.StoreInsertsOnly) > 0 {
		avgStoreUpdatesOnlyExecutionRatio = float64(e.StoreUpdatesOnly) / float64(e.StoreUpdatesOnly+e.StoreInsertsOnly) * 100.0
	}

	allSelects := ""
	for k, v := range e.SelectQueriesDurations {
		allSelects = fmt.Sprintf("%s %s: %d (%s),", allSelects, k, e.SelectQueriesCounts[k], time.Duration(int64(v)/e.Count))
	}

	return fmt.Sprintf("Total: %s, Wait for block: %s (%% %.1f), Unmarshal block: %s (%% %.1f), processing: %s (%% %.1f), queries: %s (%% %.1f), rpc: %s (%% %.1f), store flush: %s (%% %.1f | updates: %% %.1f) [Store BatchSave count: avg %d total: %d, %d distinct calls ] [Queries: %s] [for %d blocks]",
		avgTotalExecution,
		avgWaitForBlock,
		avgWaitForBlockRatio,
		avgUnmarshalBlock,
		avgUnmarshalBlockRatio,
		avgBlockProcExecution,
		avgBlockProcExecutionRatio,
		avgSelectQueriesExecution,
		avgSelectQueriesExecutionRatio,
		avgRpcExecution,
		avgRpcExecutionRatio,
		avgStoreFlushExecution,
		avgStoreFlushExecutionRatio,
		avgStoreUpdatesOnlyExecutionRatio,
		avgStoreSave,
		e.StoreSave,
		e.StoreCall,
		allSelects,
		e.Count,
	)
}

func (e *ExecutionTime) MarshalLogObject(encoder zapcore.ObjectEncoder) error {
	avgTotalExecution := time.Duration(int64(e.TotalExecution) / e.Count)
	avgWaitForBlock := time.Duration(int64(e.WaitForBlock) / e.Count)
	avgWaitForBlockRatio := float64(avgWaitForBlock) / float64(avgTotalExecution) * 100.0
	avgUnmarshalBlock := time.Duration(int64(e.UnmarshalBlock) / e.Count)
	avgUnmarshalBlockRatio := float64(avgUnmarshalBlock) / float64(avgTotalExecution) * 100.0
	avgBlockProcExecution := time.Duration(int64(e.BlockProc) / e.Count)
	avgBlockProcExecutionRatio := float64(avgBlockProcExecution) / float64(avgTotalExecution) * 100.0
	avgRpcExecution := time.Duration(int64(e.Rpc) / e.Count)
	avgRpcExecutionRatio := float64(avgRpcExecution) / float64(avgTotalExecution) * 100.0
	avgStoreFlushExecution := time.Duration(int64(e.StoreFlush) / e.Count)
	avgSelectQueriesExecution := time.Duration(int64(e.SelectQueries) / e.Count)
	avgSelectQueriesExecutionRatio := float64(avgSelectQueriesExecution) / float64(avgTotalExecution) * 100.0
	avgStoreSave := e.StoreSave / e.Count

	avgStoreFlushExecutionRatio := float64(avgStoreFlushExecution) / float64(avgTotalExecution) * 100.0
	var avgStoreUpdatesOnlyExecutionRatio float64

	if (e.StoreUpdatesOnly + e.StoreInsertsOnly) > 0 {
		avgStoreUpdatesOnlyExecutionRatio = float64(e.StoreUpdatesOnly) / float64(e.StoreUpdatesOnly+e.StoreInsertsOnly) * 100.0
	}

	allSelects := ""
	for k, v := range e.SelectQueriesDurations {
		allSelects = fmt.Sprintf("%s %s: %d (%s),", allSelects, k, e.SelectQueriesCounts[k], time.Duration(int64(v)/e.Count))
	}

	encoder.AddDuration("total", avgTotalExecution)
	encoder.AddString("wait_for_block", fmt.Sprintf("%s (%% %.1f)", avgWaitForBlock, avgWaitForBlockRatio))
	encoder.AddString("unmarshall_block", fmt.Sprintf("%s (%% %.1f)", avgUnmarshalBlock, avgUnmarshalBlockRatio))
	encoder.AddString("processing", fmt.Sprintf("%s (%% %.1f)", avgBlockProcExecution, avgBlockProcExecutionRatio))
	encoder.AddString("queries", fmt.Sprintf("%s (%% %.1f)", avgSelectQueriesExecution, avgSelectQueriesExecutionRatio))
	encoder.AddString("rpc", fmt.Sprintf("%s (%% %.1f)", avgRpcExecution, avgRpcExecutionRatio))
	encoder.AddString("flush", fmt.Sprintf("%s (%% %.1f)", avgStoreFlushExecution, avgStoreFlushExecutionRatio))
	encoder.AddString("flush", fmt.Sprintf("%s (%% %.1f | updates: %% %.1f)", avgStoreFlushExecution, avgStoreFlushExecutionRatio, avgStoreUpdatesOnlyExecutionRatio))
	encoder.AddInt64("store_save_count_avg", avgStoreSave)
	encoder.AddInt64("store_save_count_total", e.StoreSave)
	encoder.AddInt64("store_save_count_distinct", e.StoreCall)
	encoder.AddString("queries", allSelects)
	encoder.AddInt64("block_count", e.Count)
	return nil
}

type rate struct {
	count uint64
	t0    time.Time
}

func (r *rate) Clean() {
	r.count = 0
	r.t0 = time.Now()
}

func (r *rate) Inc() {
	r.count++
}

func (r *rate) Rate() float64 {
	elapsed := time.Since(r.t0)
	return float64(r.count) / (float64(elapsed) / 1000000000.0)
}

func (r *rate) String() string {
	return fmt.Sprintf("%0.01f blocks/sec (%d total)", r.Rate(), r.count)
}

type BlockMetrics struct {
	LastBlockRef bstream.BlockRef
	BlockRate    *rate

	Exec *ExecutionTime
}

func NewBlockMetrics() *BlockMetrics {
	xec := &ExecutionTime{}
	xec.Clean()
	return &BlockMetrics{
		LastBlockRef: bstream.BlockRefEmpty,
		BlockRate:    &rate{},
		Exec:         xec,
	}
}

func (m *BlockMetrics) MarshalLogObject(encoder zapcore.ObjectEncoder) error {
	encoder.AddString("last_block", m.LastBlockRef.String())
	encoder.AddString("block", m.BlockRate.String())
	encoder.AddObject("execution", m.Exec)
	return nil
}

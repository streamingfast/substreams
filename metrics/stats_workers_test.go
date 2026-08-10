package metrics

import (
	"testing"

	pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestStats_PeakWorkersIsAHighWaterMark(t *testing.T) {
	stats := NewReqStats(&Config{}, nil, nil, zlogTest)
	stats.RecordStages([]*pbsubstreamsrpc.Stage{{Modules: []string{"mod"}}})

	first := stats.RecordNewSubrequest(0, 0, 100)
	second := stats.RecordNewSubrequest(0, 100, 200)
	third := stats.RecordNewSubrequest(0, 200, 300)
	assert.Equal(t, uint64(3), stats.workers.peak.Load())

	stats.RecordEndSubrequest(first, JobComplete)
	stats.RecordEndSubrequest(second, JobComplete)

	// The peak must not decay as jobs complete, otherwise the value read at the end of the
	// request would only reflect the tail of the request instead of its busiest moment.
	assert.Equal(t, uint64(3), stats.workers.peak.Load())

	fourth := stats.RecordNewSubrequest(0, 300, 400)
	assert.Equal(t, uint64(3), stats.workers.peak.Load())

	stats.RecordEndSubrequest(third, JobComplete)
	stats.RecordEndSubrequest(fourth, JobComplete)
	assert.Equal(t, uint64(3), stats.workers.peak.Load())
}

func TestStats_WorkersAreLoggedOnTier1(t *testing.T) {
	stats := NewReqStats(&Config{}, nil, nil, zlogTest)
	stats.RecordStages([]*pbsubstreamsrpc.Stage{{Modules: []string{"mod"}}})

	// A client asking for 300 workers on a key granted 15 of them: the log must keep both
	// numbers so the gap is explainable without reproducing the request.
	stats.SetWorkerCounts(300, 15, 15)
	stats.RecordNewSubrequest(0, 0, 100)
	stats.RecordWorkerPoolExhausted()
	stats.RecordWorkerPoolExhausted()
	stats.RecordWorkerPoolRampUpDeferred()

	fields := fieldsByKey(zapFieldsOf(stats))
	require.Contains(t, fields, "workers")

	encoder := zapcore.NewMapObjectEncoder()
	fields["workers"].AddTo(encoder)

	assert.Equal(t, map[string]any{
		"requested":                  uint64(300),
		"granted":                    uint64(15),
		"effective":                  uint64(15),
		"peak":                       uint64(1),
		"pool_exhausted_count":       uint64(2),
		"pool_rampup_deferred_count": uint64(1),
	}, encoder.Fields["workers"])
}

func TestStats_WorkersAreOmittedOnTier2(t *testing.T) {
	stats := NewReqStats(&Config{Tier2: true}, nil, nil, zlogTest)

	fields := fieldsByKey(zapFieldsOf(stats))
	assert.NotContains(t, fields, "workers", "tier2 does not schedule jobs, it has no worker of its own to report")
}

// zapFieldsOf collects the fields the final request stats log would carry. The rate counter is
// stopped first, exactly like LogAndClose does, otherwise reading its rate races with the
// background janitor goroutine dmetrics runs.
func zapFieldsOf(stats *Stats) []zap.Field {
	stats.blockRate.SyncNow()
	stats.blockRate.Stop()
	return stats.getZapFields(nil)
}

func TestStats_ProgressLogReportsLiveWorkers(t *testing.T) {
	stats := NewReqStats(&Config{}, nil, nil, zlogTest)
	stats.RecordStages([]*pbsubstreamsrpc.Stage{{Modules: []string{"mod"}}})
	stats.SetWorkerCounts(300, 15, 15)

	stats.RecordNewSubrequest(0, 0, 100)
	stats.RecordNewSubrequest(0, 100, 200)
	stats.RecordWorkerPoolExhausted()

	core, logs := observer.New(zapcore.InfoLevel)
	NewProgressLogger(stats, zap.New(core)).logProgress()

	workers := fieldMap(t, logs.All()[0])["workers"].(map[string]any)
	assert.Equal(t, uint64(300), workers["requested"])
	assert.Equal(t, uint64(15), workers["granted"])
	assert.Equal(t, uint64(15), workers["effective"])
	assert.Equal(t, uint64(2), workers["running"])
	assert.Equal(t, uint64(13), workers["idle"])
	assert.Equal(t, uint64(1), workers["pool_exhausted_5m"])
}

func TestStats_ProgressLogOmitsPoolExhaustedWhenNoneHappened(t *testing.T) {
	stats := NewReqStats(&Config{}, nil, nil, zlogTest)
	stats.SetWorkerCounts(0, 8, 8)

	core, logs := observer.New(zapcore.InfoLevel)
	NewProgressLogger(stats, zap.New(core)).logProgress()

	workers := fieldMap(t, logs.All()[0])["workers"].(map[string]any)
	assert.NotContains(t, workers, "pool_exhausted_5m", "a healthy request must not carry the field at all")
	assert.Equal(t, uint64(8), workers["idle"], "no job running means every worker is idle")
}

func TestProgressHint_WorkerPoolCapsTheRequest(t *testing.T) {
	stats := NewReqStats(&Config{}, nil, nil, zlogTest)
	stats.RecordStages([]*pbsubstreamsrpc.Stage{{Modules: []string{"mod"}}})

	// Granted 300, but jobs keep finding the pool empty while only one runs: the ceiling that
	// applies is below what this request negotiated, which is exactly what cannot be seen from
	// the request's own numbers.
	stats.SetWorkerCounts(300, 300, 300)
	stats.RecordNewSubrequest(0, 0, 100)
	for range workerPoolExhaustedToReport {
		stats.RecordWorkerPoolExhausted()
	}

	core, logs := observer.New(zapcore.InfoLevel)
	NewProgressLogger(stats, zap.New(core)).logProgress()

	hints := fieldMap(t, logs.All()[0])["hints"].([]any)
	require.Len(t, hints, 1)
	assert.Contains(t, hints[0], "no worker was free")
	assert.Contains(t, hints[0], "running 1 of the 300 workers")
}

func TestProgressHint_NoWorkerPoolHintWhenRunningAtTheLimit(t *testing.T) {
	stats := NewReqStats(&Config{}, nil, nil, zlogTest)
	stats.RecordStages([]*pbsubstreamsrpc.Stage{{Modules: []string{"mod"}}})

	// Borrows failed, but the request is already running everything it is allowed to: the pool
	// refusing a further job costs it nothing, so this is not a problem to report.
	stats.SetWorkerCounts(0, 1, 1)
	stats.RecordNewSubrequest(0, 0, 100)
	for range workerPoolExhaustedToReport * 2 {
		stats.RecordWorkerPoolExhausted()
	}

	core, logs := observer.New(zapcore.InfoLevel)
	NewProgressLogger(stats, zap.New(core)).logProgress()

	assert.Empty(t, fieldMap(t, logs.All()[0])["hints"])
}

func TestProgressHint_NoWorkerPoolHintOnAFewMisses(t *testing.T) {
	stats := NewReqStats(&Config{}, nil, nil, zlogTest)
	stats.RecordStages([]*pbsubstreamsrpc.Stage{{Modules: []string{"mod"}}})

	stats.SetWorkerCounts(0, 300, 300)
	stats.RecordNewSubrequest(0, 0, 100)
	for range workerPoolExhaustedToReport - 1 {
		stats.RecordWorkerPoolExhausted()
	}

	core, logs := observer.New(zapcore.InfoLevel)
	NewProgressLogger(stats, zap.New(core)).logProgress()

	assert.Empty(t, fieldMap(t, logs.All()[0])["hints"], "a couple of misses on a shared pool is normal")
}

func fieldsByKey(fields []zap.Field) map[string]zap.Field {
	out := make(map[string]zap.Field, len(fields))
	for _, field := range fields {
		out[field.Key] = field
	}
	return out
}

package metrics

import (
	"testing"

	pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
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

func fieldsByKey(fields []zap.Field) map[string]zap.Field {
	out := make(map[string]zap.Field, len(fields))
	for _, field := range fields {
		out[field.Key] = field
	}
	return out
}

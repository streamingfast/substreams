package backprocessing

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
)

// A window older than its duration must keep covering exactly that duration, which matters
// because the server stretches its progress interval to 5s on long requests.
func TestRateWindow_TrimsToWindow(t *testing.T) {
	at := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	w := NewRateWindow(30 * time.Second)

	for i := range 20 {
		w.Add(at, uint64(i*100))
		at = at.Add(5 * time.Second)
	}

	_, span, ok := w.PerSecond()
	require.True(t, ok)
	assert.LessOrEqual(t, span, 35*time.Second)
	assert.GreaterOrEqual(t, span, 30*time.Second)
}

func TestRateWindow_IgnoresCounterReset(t *testing.T) {
	at := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	w := NewRateWindow(30 * time.Second)

	w.Add(at, 5_000)
	at = at.Add(5 * time.Second)
	w.Add(at, 100)

	_, _, ok := w.PerSecond()
	assert.False(t, ok)
}

func TestComputeStageStats_MeasuresSquashedReadinessNotProducedRanges(t *testing.T) {
	// Every segment produced, only a third squashed. Measured from CompletedRanges this reads
	// 100%; the honest figure is the squashed frontier.
	stats := StageStats([]*pbsubstreamsrpc.Stage{{
		CompletedRanges:        []*pbsubstreamsrpc.BlockRange{{StartBlock: 1_000_000, EndBlock: 2_000_000}},
		ReadyUpToExclusive:     1_300_000,
		SquashWaitSegmentCount: 35,
	}}, nil, 2_000_000)

	require.Len(t, stats, 1)
	assert.InDelta(t, 0.3, stats[0].Ratio, 0.001)
	assert.Equal(t, uint64(35), stats[0].SquashWait)
}

// A stage that has not started reports where its modules begin, which is what anchors the low
// end of its bar — ranges and jobs say nothing at all at that point.
func TestComputeStageStats_UnstartedStageAnchorsOnReadiness(t *testing.T) {
	stats := StageStats([]*pbsubstreamsrpc.Stage{{ReadyUpToExclusive: 1_000_000}}, nil, 2_000_000)

	require.Len(t, stats, 1)
	assert.Zero(t, stats[0].Ratio)
	assert.Zero(t, stats[0].SquashWait)
}

// Zero is a legitimate readiness value for a stage whose modules and chain both start at 0, so
// it must render as 0% rather than being taken for "no data" and skipped.
func TestComputeStageStats_ZeroReadinessIsAValue(t *testing.T) {
	stats := StageStats([]*pbsubstreamsrpc.Stage{{
		ReadyUpToExclusive: 0,
		CompletedRanges:    []*pbsubstreamsrpc.BlockRange{{StartBlock: 0, EndBlock: 500}},
	}}, nil, 1_000)

	require.Len(t, stats, 1)
	assert.Zero(t, stats[0].Ratio)
}

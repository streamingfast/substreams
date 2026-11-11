package sink

import (
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
)

// TestStatsCloseStopsAllRateJanitors verifies that Stats.Close() stops every
// dmetrics rate object it owns.
//
// Each AvgRate* object spawns an internal janitor goroutine on construction,
// so a missing Stop() leaks one goroutine per Stats lifecycle. In sink.Sinker's
// continuous-mode usage this produces a linear goroutine slope over time.
func TestStatsCloseStopsAllRateJanitors(t *testing.T) {
	runtime.GC()
	time.Sleep(50 * time.Millisecond)
	before := runtime.NumGoroutine()

	const iterations = 50
	for range iterations {
		s := newStats(zlog)
		s.Close()
	}

	runtime.GC()
	time.Sleep(100 * time.Millisecond)
	after := runtime.NumGoroutine()

	delta := after - before
	if delta > 2 {
		t.Errorf("goroutine leak: started=%d ended=%d delta=%d (want ≤2); "+
			"likely a rate's Stop() is not called in Stats.Close()",
			before, after, delta)
	}
}

func TestStatsLogNow_BackprocessingProgress(t *testing.T) {
	core, logs := observer.New(zapcore.InfoLevel)
	s := newStats(zap.New(core))
	defer s.Close()

	s.RecordSession(&pbsubstreamsrpc.SessionInit{
		ResolvedStartBlock:                       2_000_000,
		LinearHandoffBlock:                       2_000_000,
		MaxParallelWorkers:                       10,
		BlocksToProcessBeforeStartBlock:          2_010_000,
		EffectiveBlocksToProcessBeforeStartBlock: 2_000_000,
		EffectiveBlocksToProcessAfterStartBlock:  500_000,
	})
	s.RecordProgress(&pbsubstreamsrpc.ModulesProgress{
		ProcessedBlocks: 400_000,
		Stages: []*pbsubstreamsrpc.Stage{
			{
				Modules:                []string{"store_a"},
				CompletedRanges:        []*pbsubstreamsrpc.BlockRange{{StartBlock: 1_000_000, EndBlock: 1_300_000}},
				ReadyUpToExclusive:     1_300_000,
				SquashWaitSegmentCount: 3,
			},
			{Modules: []string{"map_out"}, ReadyUpToExclusive: 1_000_000},
		},
		RunningJobs: []*pbsubstreamsrpc.Job{
			{Stage: 0, StartBlock: 1_300_000, ProgressBlocks: 100_000, DurationMs: 3_000},
		},
	})
	s.LogNow()

	entries := logs.FilterMessage("substreams stream stats").All()
	require.Len(t, entries, 1)

	fields := entries[0].ContextMap()
	assert.NotContains(t, fields, "is_live")

	progress, ok := fields["backprocessing_progress"].(map[string]any)
	require.True(t, ok, "got %#v", fields["backprocessing_progress"])

	assert.Equal(t, 20.0, progress["pct"])
	assert.Equal(t, "1/10", progress["jobs"])
	assert.Equal(t, uint64(2_000_000), progress["blocks_to_prepare_stores"])
	assert.Equal(t, uint64(500_000), progress["blocks_in_range"])
	assert.Equal(t, uint64(10_000), progress["blocks_cached"])

	assert.Equal(t, map[string]any{
		"s0": map[string]any{
			"pct":                30.0,
			"first_block":        uint64(1_000_000),
			"ready_up_to":        uint64(1_300_000),
			"jobs":               1,
			"oldest_job":         "3s",
			"squashing_segments": uint64(3),
		},
		"s1": map[string]any{
			"pct":         0.0,
			"first_block": uint64(1_000_000),
			"jobs":        0,
		},
		"out": map[string]any{
			"pct":         0.0,
			"first_block": uint64(1_000_000),
			"start_block": uint64(2_000_000),
		},
	}, progress["stages"])

	live := true
	s.SetLiveness(&live)
	s.LogNow()

	entries = logs.FilterMessage("substreams stream stats").All()
	require.Len(t, entries, 2)
	assert.NotContains(t, entries[1].ContextMap(), "backprocessing_progress")
	assert.Equal(t, true, entries[1].ContextMap()["is_live"])
}

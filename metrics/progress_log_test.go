package metrics

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/streamingfast/bstream"
	pbssinternal "github.com/streamingfast/substreams/pb/sf/substreams/intern/v2"
	pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func testStats(t *testing.T) *Stats {
	t.Helper()
	return NewReqStats(&Config{
		UserID:       "user-1",
		OutputModule: "map_out",
	}, nil, nil, zap.NewNop())
}

func fieldMap(t *testing.T, entry observer.LoggedEntry) map[string]interface{} {
	t.Helper()
	return entry.ContextMap()
}

func TestProgressLoggerEmitsOneLine(t *testing.T) {
	stats := testStats(t)
	core, logs := observer.New(zapcore.InfoLevel)

	stats.RecordStagesProgress([]StageProgress{
		{Stage: 0, Stores: []string{"store_a"}, Mappers: []string{"map_events"},
			PlannedFirstJobStartBlock: 1_000_000, PlannedLastJobStopBlock: 2_000_000,
			HighestContiguousBlock: 1_200_000, SegmentsReadyForSquashing: 300},
		{Stage: 1, Mappers: []string{"map_out"},
			PlannedFirstJobStartBlock: 1_000_000, PlannedLastJobStopBlock: 2_000_000,
			HighestContiguousBlock: 1_500_000},
	})
	stats.RecordLastBlockSent(nil)
	stats.lastSentBlockNum = 1_000_000

	jobIdx := stats.RecordNewSubrequest(0, 1_200_000, 1_300_000)
	stats.RecordStages([]*pbsubstreamsrpc.Stage{{Modules: []string{"store_a"}}, {Modules: []string{"map_out"}}})
	stats.RecordEndSubrequest(jobIdx, JobComplete)

	stats.RecordBlockSent(20*time.Millisecond, 10)
	stats.RecordBlockSent(80*time.Millisecond, 10)

	logger := NewProgressLogger(stats, zap.New(core))
	logger.logProgress()

	require.Equal(t, 1, logs.Len())
	entry := logs.All()[0]
	assert.Equal(t, "substreams request progress", entry.Message)

	fields := fieldMap(t, entry)
	assert.Equal(t, "parallel_processing", fields["phase"])
	assert.Equal(t, uint64(1_000_000), fields["last_sent_block"])
	// map_out is contiguously ready up to 1.5M while the consumer only got to 1M.
	assert.Equal(t, uint64(1_500_000), fields["last_block_in_cache"])

	// Modules are nested under the stage that computes them.
	stages, ok := fields["stages"].([]interface{})
	require.True(t, ok)
	require.Len(t, stages, 2)

	storeStage := stages[0].(map[string]interface{})
	assert.Equal(t, []interface{}{"store_a"}, storeStage["stores"])
	assert.Equal(t, []interface{}{"map_events"}, storeStage["mappers"])
	assert.Equal(t, uint64(1_200_000), storeStage["ready_up_to"])
	assert.Equal(t, uint64(300), storeStage["squash_wait_segments"])

	storeJobs := storeStage["jobs"].(map[string]interface{})
	assert.Equal(t, uint64(1_000_000), storeJobs["start"])
	assert.Equal(t, uint64(2_000_000), storeJobs["end"])
	assert.Equal(t, uint64(1), storeJobs["completed"])
	mapStage := stages[1].(map[string]interface{})
	assert.Equal(t, []interface{}{"map_out"}, mapStage["mappers"])
	assert.NotContains(t, mapStage, "stores")
	// The planned range is known upfront even though no job ran on that stage yet.
	assert.Equal(t, uint64(1_000_000), mapStage["jobs"].(map[string]interface{})["start"])

	send := fields["blocks_sent_5m"].(map[string]interface{})
	assert.Equal(t, uint64(20), send["blocks"])
	assert.Equal(t, "5ms", send["avg_per_block"])
	// A stall is one SendMsg call blocking on gRPC flow control, not the cost of one block.
	assert.Equal(t, "100ms", send["blocked"])
	assert.Equal(t, "80ms", send["longest_stall"])
	assert.NotContains(t, send, "messages")
	assert.NotContains(t, send, "avg_per_message")
	assert.NotContains(t, send, "min")
	assert.NotContains(t, send, "p50")
	assert.NotContains(t, send, "p90")
}

func TestProgressLoggerPhases(t *testing.T) {
	phaseOf := func(t *testing.T, setup func(*Stats)) string {
		t.Helper()
		stats := testStats(t)
		core, logs := observer.New(zapcore.InfoLevel)
		setup(stats)
		NewProgressLogger(stats, zap.New(core)).logProgress()
		return fieldMap(t, logs.All()[0])["phase"].(string)
	}

	assert.Equal(t, "parallel_processing", phaseOf(t, func(stats *Stats) {}))

	// In production mode the first mapper segment is usually not cached, so a worker streams
	// it back live: the client is receiving blocks, but not from the cache and not from the
	// linear pipeline.
	assert.Equal(t, "streaming_first_segment", phaseOf(t, func(stats *Stats) {
		stats.RecordStreamingFirstSegment(true)
	}))

	assert.Equal(t, "parallel_processing", phaseOf(t, func(stats *Stats) {
		stats.RecordStreamingFirstSegment(true)
		stats.RecordStreamingFirstSegment(false)
	}), "once that segment landed, the rest is read from the cache")

	assert.Equal(t, "linear_processing", phaseOf(t, func(stats *Stats) {
		stats.RecordStreamingFirstSegment(true)
		stats.RecordBlock(bstream.NewBlockRef("aa", 12_369_800))
	}), "a block through the linear pipeline outranks everything")
}

func TestProgressLoggerWindowSurvivesReports(t *testing.T) {
	stats := testStats(t)
	core, logs := observer.New(zapcore.InfoLevel)
	stats.RecordStages([]*pbsubstreamsrpc.Stage{{Modules: []string{"store_a"}}})

	logger := NewProgressLogger(stats, zap.New(core))

	jobIdx := stats.RecordNewSubrequest(0, 0, 1000)
	stats.RecordEndSubrequest(jobIdx, JobComplete)
	logger.logProgress()

	logger.logProgress()

	require.Equal(t, 2, logs.Len())
	first := fieldMap(t, logs.All()[0])["stages"].([]interface{})[0].(map[string]interface{})["jobs"].(map[string]interface{})
	second := fieldMap(t, logs.All()[1])["stages"].([]interface{})[0].(map[string]interface{})["jobs"].(map[string]interface{})

	assert.Equal(t, uint64(1), first["completed_5m"])
	assert.Equal(t, uint64(1), first["completed"])
	// A stage the plan says nothing about reports no planned range rather than 0-0.
	assert.NotContains(t, first, "start")

	// The window is a trailing period, not "since the previous line": emitting a report does
	// not consume it, otherwise two lines close together would each cover a different span.
	assert.Equal(t, uint64(1), second["completed_5m"], "the window must not be reset by a report")
	assert.Equal(t, uint64(1), second["completed"])
}

func TestProgressLoggerWindowAgesOut(t *testing.T) {
	stats := testStats(t)
	core, logs := observer.New(zapcore.InfoLevel)
	stats.RecordStages([]*pbsubstreamsrpc.Stage{{Modules: []string{"store_a"}}})

	// One job completed inside the window, one long before it.
	stats.stageJobStats(0).window.bucket(time.Now()).completed++
	stats.stageJobStats(0).window.bucket(time.Now().Add(-ProgressWindow-time.Minute)).completed++

	NewProgressLogger(stats, zap.New(core)).logProgress()

	jobs := fieldMap(t, logs.All()[0])["stages"].([]interface{})[0].(map[string]interface{})["jobs"].(map[string]interface{})
	assert.Equal(t, uint64(1), jobs["completed_5m"], "only what happened inside the window counts")
}

func TestProgressLoggerExternalCallsReportWindowDelta(t *testing.T) {
	stats := testStats(t)
	core, logs := observer.New(zapcore.InfoLevel)
	logger := NewProgressLogger(stats, zap.New(core))

	remote := &pbssinternal.ModuleStats{
		Name:                "map_out",
		ExternalCallMetrics: []*pbssinternal.ExternalCallMetric{{Name: "eth_call", Count: 100, TimeMs: 10_000}},
	}
	stats.completedJobsStats["map_out"] = remote

	// Calls made by tier2 jobs arrive as running totals, so the delta is measured against a
	// snapshot taken a window earlier rather than against the previous report.
	stats.sampleExternalCalls(time.Now().Add(-4 * time.Minute))

	remote.ExternalCallMetrics[0].Count = 150
	remote.ExternalCallMetrics[0].TimeMs = 25_000
	logger.logProgress()

	call := fieldMap(t, logs.All()[0])["external_calls"].([]interface{})[0].(map[string]interface{})
	assert.Equal(t, uint64(150), call["count_total"])
	assert.Equal(t, uint64(50), call["count_5m"])
	assert.Equal(t, "15s", call["time_5m"])
	assert.Equal(t, "300ms", call["avg_5m"])
}

func TestProgressLoggerExternalCallBaselineAgesOut(t *testing.T) {
	stats := testStats(t)
	core, logs := observer.New(zapcore.InfoLevel)
	logger := NewProgressLogger(stats, zap.New(core))

	remote := &pbssinternal.ModuleStats{
		Name:                "map_out",
		ExternalCallMetrics: []*pbssinternal.ExternalCallMetric{{Name: "eth_call", Count: 100, TimeMs: 10_000}},
	}
	stats.completedJobsStats["map_out"] = remote

	// A baseline older than the window must be ignored, otherwise the "last 5 minutes" would
	// silently grow into "since the request started".
	stats.sampleExternalCalls(time.Now().Add(-ProgressWindow - time.Minute))
	remote.ExternalCallMetrics[0].Count = 150
	logger.logProgress()

	call := fieldMap(t, logs.All()[0])["external_calls"].([]interface{})[0].(map[string]interface{})
	assert.Equal(t, uint64(150), call["count_5m"], "with no usable baseline the lifetime total is reported")
}

func TestProgressHints(t *testing.T) {
	t.Run("slow external calls", func(t *testing.T) {
		stats := testStats(t)
		core, logs := observer.New(zapcore.InfoLevel)
		logger := NewProgressLogger(stats, zap.New(core))

		stats.completedJobsStats["map_out"] = &pbssinternal.ModuleStats{
			Name:                "map_out",
			ExternalCallMetrics: []*pbssinternal.ExternalCallMetric{{Name: "eth_call", Count: 1000, TimeMs: 500_000}},
		}
		logger.logProgress()

		fields := fieldMap(t, logs.All()[0])
		hints := fields["hints"].([]interface{})
		require.Len(t, hints, 1)
		// 1000 calls of 500ms: slow endpoint, and nothing is known about the call volume per
		// block, so the hint must not blame it.
		assert.Contains(t, hints[0].(string), "the endpoint answering them is what limits throughput")
		assert.NotContains(t, hints[0].(string), "per block")

		call := fields["external_calls"].([]interface{})[0].(map[string]interface{})
		assert.NotContains(t, call, "calls_per_block", "0 would read as \"makes no call per block\"")
	})

	t.Run("a module making too many calls per block", func(t *testing.T) {
		stats := testStats(t)
		core, logs := observer.New(zapcore.InfoLevel)
		logger := NewProgressLogger(stats, zap.New(core))

		// 5000 fast calls over 100 blocks: the endpoint is fine, the module is not.
		stats.remoteProcessedBlockCount = 100
		stats.completedJobsStats["map_out"] = &pbssinternal.ModuleStats{
			Name:                "map_out",
			ExternalCallMetrics: []*pbssinternal.ExternalCallMetric{{Name: "eth_call", Count: 5000, TimeMs: 500}},
		}
		logger.logProgress()

		fields := fieldMap(t, logs.All()[0])
		hints := fields["hints"].([]interface{})
		require.Len(t, hints, 1)
		assert.Contains(t, hints[0].(string), "its call volume is what limits throughput")

		call := fields["external_calls"].([]interface{})[0].(map[string]interface{})
		assert.Equal(t, float64(50), call["calls_per_block"])
	})

	t.Run("an external call that never returns", func(t *testing.T) {
		stats := testStats(t)
		core, logs := observer.New(zapcore.InfoLevel)
		logger := NewProgressLogger(stats, zap.New(core))

		// Begin without End: exactly what a retrying eth_call against an unreachable endpoint
		// looks like from here — the extension retries internally, so this stays one call.
		callID := stats.RecordModuleWasmExternalCallBegin("map_out", "rpc:eth_call", 12_450_739)
		stats.modulesStats["map_out"].inprocessCallMetrics[callID] = inprocessCall{
			startTime: time.Now().Add(-3 * time.Minute),
			extension: "rpc:eth_call",
			blockNum:  12_450_739,
		}
		logger.logProgress()

		call := fieldMap(t, logs.All()[0])["external_calls"].([]interface{})[0].(map[string]interface{})
		assert.Equal(t, uint64(1), call["in_flight"])
		assert.Equal(t, "3m0s", call["oldest_in_flight"])
		// Where processing is stuck for as long as the call does not return.
		assert.Equal(t, uint64(12_450_739), call["at_block"])
		// The elapsed time of a call still running must be accounted for, otherwise a call
		// hung for minutes reports as instantaneous.
		assert.Equal(t, "3m0s", call["time_5m"])

		hints := fieldMap(t, logs.All()[0])["hints"].([]interface{})
		require.NotEmpty(t, hints)
		assert.Contains(t, hints[0].(string), "still waiting for an answer")
		assert.Contains(t, hints[0].(string), "rpc:eth_call")
	})

	t.Run("a tier2 call that never returns", func(t *testing.T) {
		stats := testStats(t)
		core, logs := observer.New(zapcore.InfoLevel)
		logger := NewProgressLogger(stats, zap.New(core))

		// A module executed by a tier2 job: the worker protocol carries counts and totals
		// only, so the open call is invisible. It shows up as accrued time on a count that
		// does not move.
		stats.startTime = time.Now().Add(-30 * time.Minute)
		remote := &pbssinternal.ModuleStats{
			Name:                "map_pools_created",
			ExternalCallMetrics: []*pbssinternal.ExternalCallMetric{{Name: "rpc:eth_call", Count: 4, TimeMs: 500_000}},
		}
		stats.completedJobsStats["map_pools_created"] = remote
		stats.sampleExternalCalls(time.Now().Add(-4 * time.Minute))

		// No new call started, yet a whole window of call time accrued.
		remote.ExternalCallMetrics[0].TimeMs += uint64(ProgressWindow.Milliseconds())
		logger.logProgress()

		call := fieldMap(t, logs.All()[0])["external_calls"].([]interface{})[0].(map[string]interface{})
		assert.Equal(t, uint64(0), call["count_5m"])
		assert.Equal(t, true, call["calls_still_running"])

		hints := fieldMap(t, logs.All()[0])["hints"].([]interface{})
		require.NotEmpty(t, hints)
		assert.Contains(t, hints[0].(string), "without a single one completing")
		assert.Contains(t, hints[0].(string), "rpc:eth_call")
	})

	t.Run("many short calls are not reported as still running", func(t *testing.T) {
		stats := testStats(t)
		core, logs := observer.New(zapcore.InfoLevel)
		logger := NewProgressLogger(stats, zap.New(core))

		stats.startTime = time.Now().Add(-30 * time.Minute)
		remote := &pbssinternal.ModuleStats{
			Name:                "map_pools_created",
			ExternalCallMetrics: []*pbssinternal.ExternalCallMetric{{Name: "rpc:eth_call", Count: 100, TimeMs: 1_000}},
		}
		stats.completedJobsStats["map_pools_created"] = remote
		stats.sampleExternalCalls(time.Now().Add(-4 * time.Minute))

		// A busy but healthy module: 900 calls totalling 9s over the window.
		remote.ExternalCallMetrics[0].Count += 900
		remote.ExternalCallMetrics[0].TimeMs += 9_000
		logger.logProgress()

		call := fieldMap(t, logs.All()[0])["external_calls"].([]interface{})[0].(map[string]interface{})
		assert.NotContains(t, call, "calls_still_running")
	})

	t.Run("a short in-flight call is not reported as stuck", func(t *testing.T) {
		stats := testStats(t)
		core, logs := observer.New(zapcore.InfoLevel)
		logger := NewProgressLogger(stats, zap.New(core))

		stats.RecordModuleWasmExternalCallBegin("map_out", "rpc:eth_call", 12_450_739)
		logger.logProgress()

		hints := fieldMap(t, logs.All()[0])["hints"].([]interface{})
		for _, hint := range hints {
			assert.NotContains(t, hint.(string), "still waiting for an answer")
		}
	})

	t.Run("the consumer is the bottleneck", func(t *testing.T) {
		stats := testStats(t)
		core, logs := observer.New(zapcore.InfoLevel)
		logger := NewProgressLogger(stats, zap.New(core))

		// Most of the window spent blocked inside SendMsg: the only direct evidence that the
		// client, and not the pipeline, is what we are waiting on.
		stats.startTime = time.Now().Add(-30 * time.Minute)
		stats.RecordStagesProgress([]StageProgress{
			{Stage: 0, Mappers: []string{"map_out"}, HighestContiguousBlock: 2_000_000},
		})
		stats.lastSentBlockNum = 1_000_000
		stats.blockSendWindow.record(time.Now(), 4*time.Minute, 1_000)
		logger.logProgress()

		hints := fieldMap(t, logs.All()[0])["hints"].([]interface{})
		require.Len(t, hints, 1)
		assert.Contains(t, hints[0].(string), "blocked writing to the consumer")
		assert.Contains(t, hints[0].(string), "1,000,000 blocks already processed and waiting in the cache")
	})

	t.Run("blocked just under and just over the threshold", func(t *testing.T) {
		blockedFor := func(t *testing.T, share float64) []interface{} {
			t.Helper()
			stats := testStats(t)
			core, logs := observer.New(zapcore.InfoLevel)
			stats.startTime = time.Now().Add(-30 * time.Minute)
			stats.blockSendWindow.record(time.Now(), time.Duration(float64(ProgressWindow)*share), 1_000)
			NewProgressLogger(stats, zap.New(core)).logProgress()
			return fieldMap(t, logs.All()[0])["hints"].([]interface{})
		}

		assert.Empty(t, blockedFor(t, sendBlockedShareToReport-0.05))
		assert.Len(t, blockedFor(t, sendBlockedShareToReport+0.05), 1)
	})

	t.Run("a full cache lead is not a slow consumer", func(t *testing.T) {
		stats := testStats(t)
		core, logs := observer.New(zapcore.InfoLevel)
		logger := NewProgressLogger(stats, zap.New(core))

		// The scheduler deliberately keeps the cache a fixed distance ahead of the consumer,
		// so a healthy request sits at that ceiling permanently. Sends are fast here.
		stats.startTime = time.Now().Add(-30 * time.Minute)
		stats.RecordStagesProgress([]StageProgress{
			{Stage: 0, Mappers: []string{"map_out"}, HighestContiguousBlock: 2_000_000},
		})
		stats.lastSentBlockNum = 1_000_000
		stats.blockSendWindow.record(time.Now(), 20*time.Millisecond, 1_000)
		logger.logProgress()

		assert.Empty(t, fieldMap(t, logs.All()[0])["hints"])
	})

	t.Run("a throttle is never a hint on its own", func(t *testing.T) {
		stats := testStats(t)
		core, logs := observer.New(zapcore.InfoLevel)
		logger := NewProgressLogger(stats, zap.New(core))

		// The first stage has no dependencies, so it runs ahead until it hits the scheduler's
		// limit and stays there: being throttled for the whole window is the healthy state of
		// a production request, whatever the consumer is doing.
		stats.startTime = time.Now().Add(-30 * time.Minute)
		stats.RecordMaxParallelJobs(10)
		stats.windowThrottled.add(time.Now(), ProgressWindow)
		logger.logProgress()

		fields := fieldMap(t, logs.All()[0])
		assert.Equal(t, "5m0s", fields["jobs_throttled_5m"], "still reported as context")
		assert.Empty(t, fields["hints"])
	})

	t.Run("nothing processed yet is not the consumer being behind", func(t *testing.T) {
		stats := testStats(t)
		core, logs := observer.New(zapcore.InfoLevel)
		logger := NewProgressLogger(stats, zap.New(core))

		// A module that has done nothing reports its own initial block as its highest
		// contiguous block. Measured against a last-sent block of 0, that used to read as
		// "12 million blocks are cached and the consumer will not take them".
		stats.RecordResolvedStartBlock(12_369_621)
		stats.RecordStagesProgress([]StageProgress{
			{Stage: 0, Mappers: []string{"map_out"}, HighestContiguousBlock: 12_369_621},
		})
		logger.logProgress()

		fields := fieldMap(t, logs.All()[0])
		assert.Equal(t, uint64(12_369_621), fields["last_block_in_cache"], "the cache stops where nothing was processed")
		assert.Empty(t, fields["hints"])
	})

	t.Run("output cached ahead before the consumer read anything", func(t *testing.T) {
		stats := testStats(t)
		core, logs := observer.New(zapcore.InfoLevel)
		logger := NewProgressLogger(stats, zap.New(core))

		// Genuinely ahead this time: processed to 12.4M from a stream starting at 12,369,621,
		// and the consumer has not taken a single block.
		stats.startTime = time.Now().Add(-30 * time.Minute)
		stats.RecordResolvedStartBlock(12_369_621)
		stats.RecordStagesProgress([]StageProgress{
			{Stage: 0, Mappers: []string{"map_out"}, HighestContiguousBlock: 12_400_000},
		})
		stats.blockSendWindow.record(time.Now(), 4*time.Minute, 10)
		logger.logProgress()

		fields := fieldMap(t, logs.All()[0])
		assert.Equal(t, uint64(12_400_000), fields["last_block_in_cache"])

		hints := fields["hints"].([]interface{})
		require.Len(t, hints, 1)
		assert.Contains(t, hints[0].(string), "blocked writing to the consumer")
		assert.Contains(t, hints[0].(string), "30,379 blocks already processed and waiting in the cache")
	})

	t.Run("a segment on track to take far too long", func(t *testing.T) {
		stats := testStats(t)
		core, logs := observer.New(zapcore.InfoLevel)
		logger := NewProgressLogger(stats, zap.New(core))

		// 33 blocks of a 1000-block segment in 53s: the segment needs ~27 minutes, and there
		// is no reason to wait 15 of them before saying so.
		id := stats.RecordNewSubrequest(1, 5_000, 6_000)
		stats.runningJobs[id].start = time.Now().Add(-53 * time.Second)
		stats.runningJobs[id].ProgressBlocks = 33
		logger.logProgress()

		hints := fieldMap(t, logs.All()[0])["hints"].([]interface{})
		require.Len(t, hints, 1)
		assert.Contains(t, hints[0].(string), "covered 33 of the 1000 blocks of segment [5000, 6000)")
		assert.Contains(t, hints[0].(string), "the segment needs about 27m")
	})

	t.Run("a segment on track to finish in time", func(t *testing.T) {
		stats := testStats(t)
		core, logs := observer.New(zapcore.InfoLevel)
		logger := NewProgressLogger(stats, zap.New(core))

		// 600 blocks in 60s: done in under two minutes.
		id := stats.RecordNewSubrequest(1, 5_000, 6_000)
		stats.runningJobs[id].start = time.Now().Add(-time.Minute)
		stats.runningJobs[id].ProgressBlocks = 600
		logger.logProgress()

		assert.Empty(t, fieldMap(t, logs.All()[0])["hints"])
	})

	t.Run("a rate is not extrapolated from the first seconds", func(t *testing.T) {
		stats := testStats(t)
		core, logs := observer.New(zapcore.InfoLevel)
		logger := NewProgressLogger(stats, zap.New(core))

		// One block in the first second projects to 16 minutes, which is noise, not a signal.
		id := stats.RecordNewSubrequest(1, 5_000, 6_000)
		stats.runningJobs[id].start = time.Now().Add(-time.Second)
		stats.runningJobs[id].ProgressBlocks = 1
		logger.logProgress()

		assert.Empty(t, fieldMap(t, logs.All()[0])["hints"])
	})

	t.Run("a job with no progress at all", func(t *testing.T) {
		stats := testStats(t)
		core, logs := observer.New(zapcore.InfoLevel)
		logger := NewProgressLogger(stats, zap.New(core))

		// Nothing to extrapolate from, so it falls back to reporting the age.
		id := stats.RecordNewSubrequest(1, 5_000, 6_000)
		stats.runningJobs[id].start = time.Now().Add(-20 * time.Minute)
		logger.logProgress()

		hints := fieldMap(t, logs.All()[0])["hints"].([]interface{})
		require.Len(t, hints, 1)
		assert.Contains(t, hints[0].(string), "has a job running for 20m0s")
	})

	t.Run("squashing is behind", func(t *testing.T) {
		stats := testStats(t)
		core, logs := observer.New(zapcore.InfoLevel)
		logger := NewProgressLogger(stats, zap.New(core))

		stats.RecordStagesProgress([]StageProgress{
			{Stage: 0, Stores: []string{"store_a"}, HighestContiguousBlock: 100_000, SegmentsReadyForSquashing: 8},
		})
		// The backlog has been there a while, so the squasher is not keeping up with it.
		stats.squashBacklogSince[0] = time.Now().Add(-3 * time.Minute)
		logger.logProgress()

		hints := fieldMap(t, logs.All()[0])["hints"].([]interface{})
		require.Len(t, hints, 1)
		assert.Contains(t, hints[0].(string), "waiting to be merged for 3m0s, 8 right now")
	})

	t.Run("a squash backlog the squasher is working off", func(t *testing.T) {
		stats := testStats(t)
		core, logs := observer.New(zapcore.InfoLevel)
		logger := NewProgressLogger(stats, zap.New(core))

		// Partials pile up faster than they are merged all the time; what matters is whether
		// the backlog holds, and this one has only just appeared.
		stats.RecordStagesProgress([]StageProgress{
			{Stage: 0, Stores: []string{"store_a"}, HighestContiguousBlock: 100_000, SegmentsReadyForSquashing: 40},
		})
		logger.logProgress()

		assert.Empty(t, fieldMap(t, logs.All()[0])["hints"])
	})

	t.Run("a squash backlog that drops below the threshold restarts the clock", func(t *testing.T) {
		stats := testStats(t)
		core, logs := observer.New(zapcore.InfoLevel)
		logger := NewProgressLogger(stats, zap.New(core))

		stats.RecordStagesProgress([]StageProgress{
			{Stage: 0, Stores: []string{"store_a"}, HighestContiguousBlock: 100_000, SegmentsReadyForSquashing: 8},
		})
		stats.squashBacklogSince[0] = time.Now().Add(-3 * time.Minute)

		// The squasher caught up, then partials piled up again: that is a new backlog, not a
		// three-minute-old one.
		stats.RecordStagesProgress([]StageProgress{
			{Stage: 0, Stores: []string{"store_a"}, HighestContiguousBlock: 100_000, SegmentsReadyForSquashing: 1},
		})
		stats.RecordStagesProgress([]StageProgress{
			{Stage: 0, Stores: []string{"store_a"}, HighestContiguousBlock: 100_000, SegmentsReadyForSquashing: 8},
		})
		logger.logProgress()

		assert.Empty(t, fieldMap(t, logs.All()[0])["hints"])
	})

	t.Run("stream stopped moving", func(t *testing.T) {
		stats := testStats(t)
		core, logs := observer.New(zapcore.InfoLevel)
		logger := NewProgressLogger(stats, zap.New(core))

		// Everything was sent longer ago than the window covers: nothing moved since.
		stats.blockSendWindow.record(time.Now().Add(-ProgressWindow-time.Minute), 2*time.Millisecond, 118)
		stats.blocksSent = 118
		stats.lastSentBlockNum = 12_369_738
		logger.logProgress()

		send := fieldMap(t, logs.All()[0])["blocks_sent_5m"].(map[string]interface{})
		assert.Equal(t, uint64(0), send["blocks"])
		// A window with nothing sent reports no timings at all rather than a wall of "0s"
		// sitting next to the lifetime counters.
		assert.NotContains(t, send, "avg_per_block")
		assert.NotContains(t, send, "longest_stall")

		hints := fieldMap(t, logs.All()[0])["hints"].([]interface{})
		require.NotEmpty(t, hints)
		assert.Contains(t, hints[0].(string), "no block was sent to the consumer")
		assert.Contains(t, hints[0].(string), "12369738")
	})

	t.Run("nothing sent yet does not count as stopped", func(t *testing.T) {
		stats := testStats(t)
		core, logs := observer.New(zapcore.InfoLevel)
		logger := NewProgressLogger(stats, zap.New(core))

		// Still backprocessing, the stream never started: not a stall.
		logger.logProgress()

		assert.Empty(t, fieldMap(t, logs.All()[0])["hints"])
	})

	t.Run("job errors caused by an unreachable rpc endpoint", func(t *testing.T) {
		stats := testStats(t)
		core, logs := observer.New(zapcore.InfoLevel)
		logger := NewProgressLogger(stats, zap.New(core))

		jobIdx := stats.RecordNewSubrequest(2, 12_369_000, 12_370_000)
		stats.RecordJobError(jobIdx, errors.New(`rpc error: code = DeadlineExceeded desc = execute modules: `+
			`deadline_exceeded: execution timed out at block #12369739: unknown error: running wasm extension `+
			`"rpc::eth_call": timeout while doing eth_call, waiting for rpc provider for 169.542µs (29 attempt(s), `+
			`last error: sending request to json_rpc endpoint: Post "http://localhost:8080/": dial tcp [::1]:8080: `+
			`connect: connection refused)`))
		stats.RecordJobRetried(jobIdx)
		logger.logProgress()

		fields := fieldMap(t, logs.All()[0])
		jobError := fields["last_job_error"].(map[string]interface{})
		assert.Equal(t, 2, jobError["stage"])
		assert.Equal(t, uint64(1), jobError["count_total"])
		assert.Equal(t, uint64(1), jobError["count_5m"])
		assert.Contains(t, jobError["error"], "connection refused")

		hints := fields["hints"].([]interface{})
		require.NotEmpty(t, hints)
		joined := fmt.Sprint(hints...)
		assert.Contains(t, joined, "points at a chain RPC endpoint")
		assert.Contains(t, joined, "connection refused")

		// A second report covers the same window, so the error still counts in it.
		logger.logProgress()
		second := fieldMap(t, logs.All()[1])["last_job_error"].(map[string]interface{})
		assert.Equal(t, uint64(1), second["count_5m"])
		assert.Equal(t, uint64(1), second["count_total"])
	})

	t.Run("a module error is not blamed on the rpc endpoint", func(t *testing.T) {
		stats := testStats(t)
		core, logs := observer.New(zapcore.InfoLevel)
		logger := NewProgressLogger(stats, zap.New(core))

		jobIdx := stats.RecordNewSubrequest(1, 0, 1000)
		stats.RecordJobError(jobIdx, errors.New("execute modules: panic in module map_out: index out of range"))
		logger.logProgress()

		fields := fieldMap(t, logs.All()[0])
		hints := fields["hints"].([]interface{})
		require.NotEmpty(t, hints)
		joined := fmt.Sprint(hints...)
		assert.Contains(t, joined, "index out of range")
		assert.NotContains(t, joined, "chain RPC endpoint")
	})

	t.Run("a cancelled request is not a job error", func(t *testing.T) {
		stats := testStats(t)
		core, logs := observer.New(zapcore.InfoLevel)
		logger := NewProgressLogger(stats, zap.New(core))

		jobIdx := stats.RecordNewSubrequest(0, 0, 1000)
		stats.RecordJobError(jobIdx, fmt.Errorf("worker gave up: %w", context.Canceled))
		logger.logProgress()

		fields := fieldMap(t, logs.All()[0])
		assert.NotContains(t, fields, "last_job_error")
		assert.Empty(t, fields["hints"])
	})

	t.Run("healthy request has no hint", func(t *testing.T) {
		stats := testStats(t)
		core, logs := observer.New(zapcore.InfoLevel)
		logger := NewProgressLogger(stats, zap.New(core))

		stats.startTime = time.Now().Add(-30 * time.Minute)
		stats.RecordStagesProgress([]StageProgress{
			{Stage: 0, Mappers: []string{"map_out"}, HighestContiguousBlock: 1_000_100},
		})
		stats.lastSentBlockNum = 1_000_000
		stats.RecordBlockSent(2*time.Millisecond, 1)
		logger.logProgress()

		assert.Empty(t, fieldMap(t, logs.All()[0])["hints"])
	})
}

func TestProgressLogDurationFromEnv(t *testing.T) {
	fallback := 5 * time.Minute

	t.Run("unset keeps the default", func(t *testing.T) {
		assert.Equal(t, fallback, progressLogDurationFromEnv("SUBSTREAMS_TEST_UNSET_VAR", fallback))
	})

	t.Run("valid duration overrides", func(t *testing.T) {
		t.Setenv(EnvProgressLogInterval, "90s")
		assert.Equal(t, 90*time.Second, progressLogDurationFromEnv(EnvProgressLogInterval, fallback))
	})

	t.Run("unparseable value panics", func(t *testing.T) {
		t.Setenv(EnvProgressLogInterval, "5 minutes")
		assert.PanicsWithError(t,
			`invalid value for env var SUBSTREAMS_PROGRESS_LOG_INTERVAL: time: unknown unit " minutes" in duration "5 minutes"`,
			func() { progressLogDurationFromEnv(EnvProgressLogInterval, fallback) })
	})

	t.Run("non-positive value panics, it would busy-loop", func(t *testing.T) {
		t.Setenv(EnvProgressLogInterval, "0s")
		assert.Panics(t, func() { progressLogDurationFromEnv(EnvProgressLogInterval, fallback) })

		t.Setenv(EnvProgressLogInterval, "-1m")
		assert.Panics(t, func() { progressLogDurationFromEnv(EnvProgressLogInterval, fallback) })
	})
}

func TestProgressLoggerUsesConfiguredIntervals(t *testing.T) {
	previousFirst, previousInterval := FirstProgressLogDelay, ProgressLogInterval
	FirstProgressLogDelay, ProgressLogInterval = 10*time.Millisecond, 20*time.Millisecond
	t.Cleanup(func() { FirstProgressLogDelay, ProgressLogInterval = previousFirst, previousInterval })

	core, logs := observer.New(zapcore.InfoLevel)
	logger := NewProgressLogger(testStats(t), zap.New(core))
	assert.Equal(t, 10*time.Millisecond, logger.firstDelay)
	assert.Equal(t, 20*time.Millisecond, logger.interval)

	ctx, cancel := context.WithCancel(context.Background())
	go logger.Run(ctx)
	require.Eventually(t, func() bool { return logs.Len() >= 3 }, 2*time.Second, 5*time.Millisecond)
	cancel()
}

func TestWindowedDurations(t *testing.T) {
	now := time.Now()
	w := &windowedDurations{}
	for i := 1; i <= 100; i++ {
		w.record(now, time.Duration(i)*time.Millisecond, 1)
	}
	snap := w.snapshot(now)

	assert.Equal(t, uint64(100), snap.count)
	assert.Equal(t, uint64(100), snap.blocks)
	assert.Equal(t, 5050*time.Millisecond, snap.total)
	assert.Equal(t, 1*time.Millisecond, snap.minimum)
	assert.Equal(t, 100*time.Millisecond, snap.maximum)
}

func TestWindowedDurationsSpanBuckets(t *testing.T) {
	now := time.Now()
	w := &windowedDurations{}

	// Spread over the window: everything still counts, and reading twice does not consume it.
	for i := 0; i < 10; i++ {
		w.record(now.Add(-time.Duration(i)*30*time.Second), 10*time.Millisecond, 2)
	}
	assert.Equal(t, uint64(10), w.snapshot(now).count)
	assert.Equal(t, uint64(20), w.snapshot(now).blocks, "a report must not consume the window")

}

func TestWindowedDurationsAgeOut(t *testing.T) {
	now := time.Now()
	w := &windowedDurations{}

	w.record(now.Add(-ProgressWindow-time.Minute), time.Second, 100)

	snap := w.snapshot(now)
	assert.Equal(t, uint64(0), snap.count, "a sample older than the window must not be reported")
	assert.Equal(t, time.Duration(0), snap.maximum)
}

func TestWindowedCounterAgesOut(t *testing.T) {
	now := time.Now()
	var c windowedCounter

	c.add(now, 3)
	c.add(now.Add(-2*time.Minute), 4)
	c.add(now.Add(-ProgressWindow-time.Minute), 100)

	assert.Equal(t, uint64(7), c.sum(now))
}

// A slot is addressed by absolute time, so a stale bucket must be overwritten rather than
// summed when its slot comes back around a full cycle later.
func TestWindowedCounterReusesStaleSlot(t *testing.T) {
	now := time.Now()
	var c windowedCounter

	old := now.Add(-time.Duration(windowBuckets) * windowBucketDuration)
	c.add(old, 100)
	c.add(now, 1)

	assert.Equal(t, uint64(1), c.sum(now))
}

package tui

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
)

// Run with UPDATE_GOLDEN=true to rewrite the expectations after an intentional change, then
// read the diff: these files are the actual thing the user sees.
func assertGolden(t *testing.T, name, actual string) {
	t.Helper()

	path := filepath.Join("testdata", name+".txt")
	if os.Getenv("UPDATE_GOLDEN") == "true" {
		require.NoError(t, os.MkdirAll("testdata", 0755))
		require.NoError(t, os.WriteFile(path, []byte(actual), 0644))
		return
	}

	expected, err := os.ReadFile(path)
	require.NoError(t, err, "missing golden file, run with UPDATE_GOLDEN=true to create it")
	assert.Equal(t, string(expected), actual)
}

const (
	backprocessingTarget = 1_500_000
	backprocessingStart  = 1_000_000
)

func nominalSession() *pbsubstreamsrpc.SessionInit {
	return &pbsubstreamsrpc.SessionInit{
		TraceId:                                  "8bf2e35f5a387311a74c4d08f0678b52",
		ResolvedStartBlock:                       backprocessingTarget,
		LinearHandoffBlock:                       backprocessingTarget,
		MaxParallelWorkers:                       10,
		ChainHead:                                3_412_884,
		BlocksToProcessBeforeStartBlock:          3_000_000,
		EffectiveBlocksToProcessBeforeStartBlock: 2_000_000,
	}
}

// nominalProgress describes a request halfway through a two stage backprocess. Stage 1 has a
// gap in its completed ranges, so its coverage and its contiguous output frontier differ —
// which is the whole reason the `out` row exists next to the stage rows.
func nominalProgress(processed uint64, moduleMs, moduleBlocks uint64) *pbsubstreamsrpc.ModulesProgress {
	return &pbsubstreamsrpc.ModulesProgress{
		ProcessedBlocks: processed,
		Stages: []*pbsubstreamsrpc.Stage{
			{
				Modules:            []string{"uni:store_pools"},
				CompletedRanges:    []*pbsubstreamsrpc.BlockRange{{StartBlock: backprocessingStart, EndBlock: backprocessingTarget}},
				ReadyUpToExclusive: backprocessingTarget,
			},
			{
				Modules: []string{"uni:map_pools_created"},
				CompletedRanges: []*pbsubstreamsrpc.BlockRange{
					{StartBlock: backprocessingStart, EndBlock: 1_150_000},
					{StartBlock: 1_300_000, EndBlock: 1_400_000},
				},
				// The gap means readiness stops at the end of the first contiguous run, even
				// though a later range has been produced.
				ReadyUpToExclusive: 1_150_000,
			},
		},
		RunningJobs: []*pbsubstreamsrpc.Job{
			{Stage: 1, StartBlock: 1_150_000, StopBlock: 1_160_000, ProgressBlocks: 4_000, DurationMs: 41_000},
			{Stage: 1, StartBlock: 1_160_000, StopBlock: 1_170_000, ProgressBlocks: 2_000, DurationMs: 12_000},
			{Stage: 1, StartBlock: 1_170_000, StopBlock: 1_180_000, ProgressBlocks: 1_000, DurationMs: 6_000},
			{Stage: 1, StartBlock: 1_180_000, StopBlock: 1_190_000, ProgressBlocks: 500, DurationMs: 2_000},
		},
		ModulesStats: []*pbsubstreamsrpc.ModuleStats{
			{
				Name:                     "uni:map_pools_created",
				TotalProcessedBlockCount: moduleBlocks,
				TotalProcessingTimeMs:    moduleMs,
				ExternalCallMetrics: []*pbsubstreamsrpc.ExternalCallMetric{
					{Name: "eth_call", Count: 4, TimeMs: moduleMs * 61 / 100},
				},
			},
			{
				Name:                        "uni:store_pools",
				TotalProcessedBlockCount:    500_000,
				TotalProcessingTimeMs:       6_000_000,
				TotalStoreOperationTimeMs:   1_800_000,
				TotalStoreReadCount:         1_500_000,
				TotalStoreWriteCount:        500_000,
				TotalStoreDeleteprefixCount: 0,
			},
		},
	}
}

// nominalModel drives two progress messages 20s apart so the rate window, the ETA and the
// recent module costs are all populated, as they are on any request older than ten seconds.
func nominalModel(t *testing.T, width int) model {
	t.Helper()

	clock := newTestClock()
	m := newTestModel(clock)
	m.Width = width

	// The module got 10s slower per block between the two samples: 1.2M ms over 100k blocks
	// on average, but the 250k ms it spent on the last 10k blocks is 25ms per block.
	m = update(t, m, nominalSession(), nominalProgress(900_000, 1_200_000, 100_000))
	clock.advance(20 * time.Second)
	m = update(t, m, nominalProgress(933_500, 1_450_000, 110_000))

	return m
}

func TestGolden_Backprocessing(t *testing.T) {
	assertGolden(t, "backprocessing", nominalModel(t, 120).View())
}

func TestGolden_BackprocessingNarrowTerminal(t *testing.T) {
	assertGolden(t, "backprocessing_narrow", nominalModel(t, 70).View())
}

// Squashing is the state that used to read as a stall: every segment has been produced, so
// completed_ranges is full and no job is running, but tier1 is still merging the partials into
// the store and processed_blocks does not move. Measured from completed_ranges the stage would
// show 100% at a rate of zero.
func TestGolden_Squashing(t *testing.T) {
	m := newTestModel(newTestClock())
	m.Width = 120

	m = update(t, m, nominalSession(), &pbsubstreamsrpc.ModulesProgress{
		ProcessedBlocks: 900_000,
		Stages: []*pbsubstreamsrpc.Stage{
			{
				Modules:         []string{"uni:store_pools"},
				CompletedRanges: []*pbsubstreamsrpc.BlockRange{{StartBlock: backprocessingStart, EndBlock: backprocessingTarget}},
				// Produced all the way to the target, squashed only a third of the way.
				ReadyUpToExclusive:     1_150_000,
				SquashWaitSegmentCount: 35,
			},
		},
	})

	assertGolden(t, "squashing", m.View())
}

func TestGolden_NothingToDo(t *testing.T) {
	m := update(t, newTestModel(newTestClock()), &pbsubstreamsrpc.SessionInit{
		TraceId:                         "8bf2e35f5a387311a74c4d08f0678b52",
		ResolvedStartBlock:              3_385_000,
		BlocksToProcessBeforeStartBlock: 3_385_000,
		// Everything was cached by a previous run, so nothing is left to process.
		EffectiveBlocksToProcessBeforeStartBlock: 0,
	})

	assertGolden(t, "nothing_to_do", m.View())
}

func TestGolden_Starting(t *testing.T) {
	m := update(t, newTestModel(newTestClock()), nominalSession())
	assertGolden(t, "starting", m.View())
}

func TestGolden_Complete(t *testing.T) {
	clock := newTestClock()
	m := update(t, newTestModel(clock), nominalSession())
	clock.advance(4*time.Minute + 12*time.Second)
	m = update(t, m, &pbsubstreamsrpc.ModulesProgress{ProcessedBlocks: 2_000_000})

	assertGolden(t, "complete", m.View())
}

// A single stage has nothing to compare itself against, so the output frontier folds into its
// row rather than restating the same bar underneath it.
func TestGolden_SingleStage(t *testing.T) {
	m := update(t, newTestModel(newTestClock()),
		&pbsubstreamsrpc.SessionInit{
			TraceId:                                 "8bf2e35f5a387311a74c4d08f0678b52",
			ResolvedStartBlock:                      backprocessingTarget,
			LinearHandoffBlock:                      backprocessingTarget,
			MaxParallelWorkers:                      10,
			BlocksToProcessAfterStartBlock:          500_000,
			EffectiveBlocksToProcessAfterStartBlock: 500_000,
		},
		&pbsubstreamsrpc.ModulesProgress{
			ProcessedBlocks: 150_000,
			Stages: []*pbsubstreamsrpc.Stage{{
				Modules:            []string{"map_events"},
				ReadyUpToExclusive: 1_150_000,
				CompletedRanges:    []*pbsubstreamsrpc.BlockRange{{StartBlock: backprocessingStart, EndBlock: 1_150_000}},
			}},
			RunningJobs: []*pbsubstreamsrpc.Job{{Stage: 0, StartBlock: 1_150_000, StopBlock: 1_160_000, DurationMs: 3_000}},
		},
	)

	assertGolden(t, "single_stage", m.View())
}

// Past four stages the finished ones at the front are the least interesting rows on screen.
func TestGolden_ManyStagesCollapse(t *testing.T) {
	stages := make([]*pbsubstreamsrpc.Stage, 0, 8)
	for i := range 8 {
		end := uint64(backprocessingTarget)
		if i >= 5 {
			end = 1_200_000
		}
		stages = append(stages, &pbsubstreamsrpc.Stage{
			Modules:            []string{"mod_" + string(rune('a'+i))},
			CompletedRanges:    []*pbsubstreamsrpc.BlockRange{{StartBlock: backprocessingStart, EndBlock: end}},
			ReadyUpToExclusive: end,
		})
	}

	m := update(t, newTestModel(newTestClock()),
		nominalSession(),
		&pbsubstreamsrpc.ModulesProgress{
			ProcessedBlocks: 800_000,
			Stages:          stages,
			RunningJobs:     []*pbsubstreamsrpc.Job{{Stage: 7, StartBlock: 1_200_000, StopBlock: 1_210_000, DurationMs: 9_000}},
		},
	)

	assertGolden(t, "many_stages", m.View())
}

// Early in a run only the first stage has reported anything. The out row must still be there,
// at 0%: rows appearing out of nowhere later make a live region hard to read.
func TestGolden_LastStageNotStartedYet(t *testing.T) {
	m := update(t, newTestModel(newTestClock()),
		nominalSession(),
		&pbsubstreamsrpc.ModulesProgress{
			ProcessedBlocks: 50_000,
			Stages: []*pbsubstreamsrpc.Stage{
				{Modules: []string{"uni:store_pools"}, CompletedRanges: []*pbsubstreamsrpc.BlockRange{{StartBlock: backprocessingStart, EndBlock: 1_050_000}}, ReadyUpToExclusive: 1_050_000},
				// Not started: readiness reports where the stage begins, which anchors its bar.
				{Modules: []string{"uni:map_pools_created"}, ReadyUpToExclusive: backprocessingStart},
			},
			RunningJobs: []*pbsubstreamsrpc.Job{{Stage: 0, StartBlock: 1_050_000, StopBlock: 1_060_000, DurationMs: 2_000}},
		},
	)

	assertGolden(t, "last_stage_not_started", m.View())
}

// When nothing is finished there is nothing to collapse, so rows have to be dropped. The
// frontier says more about the run than its head does, so the front goes — and says so.
func TestGolden_ManyStagesNoneFinished(t *testing.T) {
	stages := make([]*pbsubstreamsrpc.Stage, 0, 6)
	for i := range 6 {
		stages = append(stages, &pbsubstreamsrpc.Stage{
			Modules:            []string{"mod_" + string(rune('a'+i))},
			CompletedRanges:    []*pbsubstreamsrpc.BlockRange{{StartBlock: backprocessingStart, EndBlock: 1_100_000}},
			ReadyUpToExclusive: 1_100_000,
		})
	}

	m := update(t, newTestModel(newTestClock()),
		nominalSession(),
		&pbsubstreamsrpc.ModulesProgress{
			ProcessedBlocks: 600_000,
			Stages:          stages,
			RunningJobs:     []*pbsubstreamsrpc.Job{{Stage: 5, StartBlock: 1_100_000, StopBlock: 1_110_000, DurationMs: 4_000}},
		},
	)

	assertGolden(t, "many_stages_none_finished", m.View())
}

// Two imported modules can share a leaf name, in which case the qualified name is the only
// way to tell them apart and shortening has to back off.
func TestGolden_ModuleNameCollision(t *testing.T) {
	m := update(t, newTestModel(newTestClock()),
		nominalSession(),
		&pbsubstreamsrpc.ModulesProgress{
			ProcessedBlocks: 100_000,
			Stages: []*pbsubstreamsrpc.Stage{
				{Modules: []string{"alpha:map_events"}, CompletedRanges: []*pbsubstreamsrpc.BlockRange{{StartBlock: backprocessingStart, EndBlock: 1_100_000}}, ReadyUpToExclusive: 1_100_000},
				{Modules: []string{"beta:map_events", "gamma:map_transfers"}, CompletedRanges: []*pbsubstreamsrpc.BlockRange{{StartBlock: backprocessingStart, EndBlock: 1_050_000}}, ReadyUpToExclusive: 1_050_000},
			},
			ModulesStats: []*pbsubstreamsrpc.ModuleStats{
				{Name: "alpha:map_events", TotalProcessedBlockCount: 1_000, TotalProcessingTimeMs: 50_000},
				{Name: "beta:map_events", TotalProcessedBlockCount: 1_000, TotalProcessingTimeMs: 30_000},
				{Name: "gamma:map_transfers", TotalProcessedBlockCount: 1_000, TotalProcessingTimeMs: 20_000},
			},
		},
	)

	assertGolden(t, "module_name_collision", m.View())
}

// A module that is merely present must not earn a row: a diagnostic that fires on a healthy
// run is worse than no diagnostic at all.
func TestGolden_CheapModulesAreHidden(t *testing.T) {
	m := update(t, newTestModel(newTestClock()),
		nominalSession(),
		&pbsubstreamsrpc.ModulesProgress{
			ProcessedBlocks: 100_000,
			Stages: []*pbsubstreamsrpc.Stage{
				{Modules: []string{"map_cheap"}, CompletedRanges: []*pbsubstreamsrpc.BlockRange{{StartBlock: backprocessingStart, EndBlock: 1_100_000}}, ReadyUpToExclusive: 1_100_000},
			},
			ModulesStats: []*pbsubstreamsrpc.ModuleStats{
				{Name: "map_cheap", TotalProcessedBlockCount: 100_000, TotalProcessingTimeMs: 100_000},
			},
		},
	)

	assertGolden(t, "cheap_modules_hidden", m.View())
}

func TestGolden_Preamble(t *testing.T) {
	actual := formatSessionPreamble(nominalSession(), sessionContext{
		Endpoint:       "hoodi",
		OutputModule:   "metadata_to_foundational_store",
		ProductionMode: true,
		Stages:         2,
	})

	assertGolden(t, "preamble", actual+"\n")
}

// Development mode against a bare server: no head block, no worker count, a single stage.
func TestGolden_PreambleMinimal(t *testing.T) {
	actual := formatSessionPreamble(&pbsubstreamsrpc.SessionInit{
		TraceId:                                 "8bf2e35f5a387311a74c4d08f0678b52",
		ResolvedStartBlock:                      100,
		BlocksToProcessAfterStartBlock:          60,
		EffectiveBlocksToProcessAfterStartBlock: 60,
	}, sessionContext{
		Endpoint:     "localhost:9000",
		OutputModule: "map_events",
		Stages:       1,
	})

	assertGolden(t, "preamble_minimal", actual+"\n")
}

// A second run of the same request finds everything cached. "0 prepare stores" is not an
// amount of work, so the line has to say what actually happened.
func TestGolden_PreambleFullyCached(t *testing.T) {
	actual := formatSessionPreamble(&pbsubstreamsrpc.SessionInit{
		TraceId:                                  "a9ba0e51af0f494eb8036d60c731a6c8",
		ResolvedStartBlock:                       6_000,
		MaxParallelWorkers:                       10,
		BlocksToProcessBeforeStartBlock:          5_900,
		EffectiveBlocksToProcessBeforeStartBlock: 0,
		BlocksToProcessAfterStartBlock:           200,
		EffectiveBlocksToProcessAfterStartBlock:  0,
	}, sessionContext{
		Endpoint:       "localhost:54168",
		OutputModule:   "map_tx_counter_summary",
		ProductionMode: true,
		Stages:         2,
	})

	assertGolden(t, "preamble_fully_cached", actual+"\n")
}

func TestEffectiveStageCount(t *testing.T) {
	// The server collapses the graph to a single stage outside production mode, the preamble
	// must not claim otherwise.
	assert.Equal(t, 1, effectiveStageCount(4, false))
	assert.Equal(t, 4, effectiveStageCount(4, true))
}

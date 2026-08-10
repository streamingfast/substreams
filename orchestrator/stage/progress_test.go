package stage

import (
	"context"
	"testing"

	"github.com/streamingfast/bstream"
	"github.com/streamingfast/substreams/metrics"
	"github.com/streamingfast/substreams/orchestrator/plan"
	"github.com/streamingfast/substreams/pipeline/exec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stagesForProgress builds 3 stages (store, store, map) over blocks [5, 50) with a segment
// interval of 10. Note that the test graph leaves every module's initial block at 0.
func stagesForProgress(t *testing.T, firstStreamableBlock uint64) *Stages {
	t.Helper()

	previous := bstream.GetProtocolFirstStreamableBlock
	bstream.GetProtocolFirstStreamableBlock = firstStreamableBlock
	t.Cleanup(func() { bstream.GetProtocolFirstStreamableBlock = previous })

	reqPlan, err := plan.BuildTier1RequestPlan(true, 10, 5, 5, 5, 50, 50, true)
	require.NoError(t, err)

	return NewStages(context.Background(), exec.TestGraphStagedModules(5, 5, 5, 5, 5), reqPlan, nil, nil)
}

func setStates(t *testing.T, stages *Stages, states map[Unit]UnitState) {
	t.Helper()
	for u := range states {
		stages.allocSegments(u.Segment)
	}
	for u, state := range states {
		stages.forceTransition(u.Segment, u.Stage, state)
	}
}

func progressByStage(progress []metrics.StageProgress) map[int]metrics.StageProgress {
	out := make(map[int]metrics.StageProgress)
	for _, p := range progress {
		out[p.Stage] = p
	}
	return out
}

func TestModulesProgressNothingDone(t *testing.T) {
	// The chain does not start before block 3, so no module can claim anything below it.
	stages := stagesForProgress(t, 3)

	progress := progressByStage(stages.stagesProgress(stages.computeStageStats()))
	require.Len(t, progress, 3)

	// Nothing processed yet: every stage reports the block its modules start from.
	for stage, p := range progress {
		assert.Equal(t, uint64(3), p.HighestContiguousBlock, "stage %d", stage)
		assert.Equal(t, uint64(0), p.SegmentsReadyForSquashing, "stage %d", stage)
	}
}

func TestStagesProgressPlannedRangeComesFromThePlan(t *testing.T) {
	stages := stagesForProgress(t, 0)

	// Nothing was scheduled at all, yet each stage already knows the whole span of jobs it
	// has to run for this request — it comes from the plan's segmenter, not from what the
	// scheduler picked up. The test graph leaves init blocks at 0, hence [0, 50).
	progress := progressByStage(stages.stagesProgress(stages.computeStageStats()))
	for stage, p := range progress {
		segmenter := stages.stages[stage].segmenter
		assert.Equal(t, segmenter.InitialBlock(), p.PlannedFirstJobStartBlock, "stage %d", stage)
		assert.Equal(t, segmenter.ExclusiveEndBlock(), p.PlannedLastJobStopBlock, "stage %d", stage)
		assert.Equal(t, uint64(0), p.PlannedFirstJobStartBlock, "stage %d", stage)
		assert.Equal(t, uint64(50), p.PlannedLastJobStopBlock, "stage %d", stage)
	}
}

func TestModulesProgressStoreExcludesUnsquashedPartials(t *testing.T) {
	stages := stagesForProgress(t, 0)

	// Stage 0: segments 0 and 1 are squashed, 2 and 3 only have their partial on disk.
	setStates(t, stages, map[Unit]UnitState{
		unit(0, 0): UnitCompleted,
		unit(1, 0): UnitCompleted,
		unit(2, 0): UnitPartialPresent,
		unit(3, 0): UnitMerging,
	})

	progress := progressByStage(stages.stagesProgress(stages.computeStageStats()))

	// Contiguous stops at the end of segment 1, the two partials above are reported apart.
	assert.Equal(t, uint64(20), progress[0].HighestContiguousBlock)
	assert.Equal(t, uint64(2), progress[0].SegmentsReadyForSquashing)
}

func TestModulesProgressStoreIgnoresPartialsBehindAHole(t *testing.T) {
	stages := stagesForProgress(t, 0)

	// A hole at segment 1 must not let segment 2 count as contiguous.
	setStates(t, stages, map[Unit]UnitState{
		unit(0, 0): UnitCompleted,
		unit(2, 0): UnitCompleted,
	})

	progress := progressByStage(stages.stagesProgress(stages.computeStageStats()))
	assert.Equal(t, uint64(10), progress[0].HighestContiguousBlock)
}

func TestModulesProgressMapCountsPartials(t *testing.T) {
	stages := stagesForProgress(t, 0)

	// Mapper output is read straight from its partial exec-out files, so a partial counts
	// as ready, unlike for a store.
	setStates(t, stages, map[Unit]UnitState{
		unit(0, 2): UnitCompleted,
		unit(1, 2): UnitPartialPresent,
		unit(2, 2): UnitPartialPresent,
	})

	progress := progressByStage(stages.stagesProgress(stages.computeStageStats()))
	assert.Equal(t, uint64(30), progress[2].HighestContiguousBlock)
	assert.Equal(t, uint64(0), progress[2].SegmentsReadyForSquashing, "maps are never squashed")
}

func TestComputeStageStatsKeepsCompletedRanges(t *testing.T) {
	stages := stagesForProgress(t, 0)
	setStates(t, stages, map[Unit]UnitState{
		unit(0, 0): UnitCompleted,
		unit(1, 0): UnitCompleted,
		unit(2, 0): UnitPartialPresent,
	})

	stats := stages.computeStageStats()
	merged := stats.ranges[0].Merged()
	require.Len(t, merged, 1)
	assert.Equal(t, uint64(0), merged[0].StartBlock)
	assert.Equal(t, uint64(30), merged[0].ExclusiveEndBlock)
}

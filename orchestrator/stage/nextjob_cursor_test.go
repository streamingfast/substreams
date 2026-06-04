package stage

import (
	"context"
	"math"
	"testing"

	"github.com/streamingfast/substreams/orchestrator/plan"
	"github.com/streamingfast/substreams/pipeline/exec"
	"github.com/stretchr/testify/require"
)

// drainSchedule runs a full backprocessing to completion the same way the real
// scheduler does (NextJob -> MarkJobSuccess -> merge stores), recording the exact
// sequence of jobs NextJob hands out. When resetCursor is true it forces NextJob
// to rescan from the first segment every call (the pre-optimization behaviour),
// so we can prove the cursor doesn't change which jobs are scheduled.
func drainSchedule(t *testing.T, stages *Stages, resetCursor bool) []Unit {
	t.Helper()

	mergeStores := func() {
		for _, stage := range stages.stages {
			if stage.kind != KindStore {
				continue
			}
			for {
				mu := stage.nextUnit()
				if mu.Segment > stage.segmenter.LastIndex() {
					break
				}
				if stages.getState(mu) != UnitPartialPresent {
					break
				}
				if !stages.previousUnitComplete(mu) {
					break
				}
				stages.MarkSegmentMerging(mu)
				stages.MergeCompleted(mu)
			}
		}
	}

	var jobs []Unit
	for i := 0; i < 100000; i++ {
		if resetCursor {
			stages.nextJobCursor = stages.globalSegmenter.FirstIndex()
		}
		u, r, _ := stages.NextJob(math.MaxInt)
		if r == nil {
			break
		}
		jobs = append(jobs, u)
		stages.MarkJobSuccess(u)
		mergeStores()
	}
	return jobs
}

func buildDrainStages(t *testing.T) *Stages {
	t.Helper()
	reqPlan, err := plan.BuildTier1RequestPlan(true, 10, 0, 0, 0, 200, 200, true)
	require.NoError(t, err)
	return NewStages(
		context.Background(),
		exec.TestGraphStagedModules(0, 0, 0, 0, 0),
		reqPlan,
		nil,
		nil,
	)
}

// TestNextJobCursorEquivalence proves that scanning from the advancing cursor
// produces exactly the same job stream as always rescanning from the start.
func TestNextJobCursorEquivalence(t *testing.T) {
	withCursor := drainSchedule(t, buildDrainStages(t), false)
	resetEveryCall := drainSchedule(t, buildDrainStages(t), true)

	require.NotEmpty(t, withCursor)
	require.Equal(t, resetEveryCall, withCursor,
		"the scan cursor must not change which jobs are scheduled vs. a full rescan")
}

// TestNextJobCursorAdvances proves the optimization actually kicks in: after a
// full drain the cursor has moved well past the first segment.
func TestNextJobCursorAdvances(t *testing.T) {
	stages := buildDrainStages(t)
	first := stages.globalSegmenter.FirstIndex()
	last := stages.globalSegmenter.LastIndex()

	drainSchedule(t, stages, false)

	require.Greater(t, stages.nextJobCursor, first,
		"cursor should advance past the finished prefix")
	require.GreaterOrEqual(t, stages.nextJobCursor, last,
		"after a full drain the cursor should reach the end")
}

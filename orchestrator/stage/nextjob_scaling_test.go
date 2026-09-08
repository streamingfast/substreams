package stage

import (
	"context"
	"math"
	"testing"

	"github.com/streamingfast/substreams/orchestrator/plan"
	"github.com/streamingfast/substreams/pipeline/exec"
	"github.com/stretchr/testify/require"
)

// scheduleRun drives Stages the way the scheduler does, without workers or a
// store: up to `workers` jobs in flight, each job succeeding in the order it was
// handed out, and one squash per `squashEvery` job completions so that the
// squasher lags far behind the jobs. It returns how many NextJob calls it took.
func scheduleRun(t testing.TB, segments, workers, squashEvery int) (stages *Stages, nextJobCalls int) {
	t.Helper()
	interval := uint64(10)
	end := uint64(segments) * interval
	reqPlan, err := plan.BuildTier1RequestPlan(true, interval, 0, 0, 0, end, end, true)
	require.NoError(t, err)
	stages = NewStages(context.Background(), exec.TestGraphStagedModules(0, 0, 0, 0, 0), reqPlan, nil, nil)

	squashOne := func() bool {
		for idx, stage := range stages.stages {
			if stage.kind != KindStore {
				continue
			}
			u := stage.nextUnit()
			if u.Segment > stage.segmenter.LastIndex() || stages.getState(u) != UnitPartialPresent || !stages.previousUnitComplete(u) {
				continue
			}
			stages.MarkSegmentMerging(u)
			stages.MergeCompleted(u)
			_ = idx
			return true
		}
		return false
	}

	var inflight []Unit
	completions := 0
	for {
		for len(inflight) < workers {
			nextJobCalls++
			u, r, _ := stages.NextJob(math.MaxInt)
			if r == nil {
				break
			}
			inflight = append(inflight, u)
		}
		if len(inflight) == 0 {
			if !squashOne() {
				break
			}
			continue
		}
		u := inflight[0]
		inflight = inflight[1:]
		stages.MarkJobSuccess(u)
		completions++
		if completions%squashEvery == 0 {
			squashOne()
		}
	}
	require.True(t, stages.AllStoresCompleted(), "every store segment squashed")
	require.True(t, stages.LastStageCompleted(), "every map segment produced")
	return stages, nextJobCalls
}

func TestNextJob_CursorFollowsTheJobFrontierNotTheSquasher(t *testing.T) {
	interval := uint64(10)
	segments := 200
	end := uint64(segments) * interval
	reqPlan, err := plan.BuildTier1RequestPlan(true, interval, 0, 0, 0, end, end, true)
	require.NoError(t, err)
	stages := NewStages(context.Background(), exec.TestGraphStagedModules(0, 0, 0, 0, 0), reqPlan, nil, nil)

	// Run every stage-0 job to completion without squashing anything: the whole
	// first stage is PartialPresent, waiting on the squasher.
	for {
		u, r, _ := stages.NextJob(math.MaxInt)
		if r == nil {
			break
		}
		stages.MarkJobSuccess(u)
	}
	require.Equal(t, UnitPartialPresent, stages.getState(Unit{Segment: segments - 1, Stage: 0}))

	// The next call has nothing to hand out. It must not rescan the backlog: the
	// first stage's pending cursor sits at the frontier, not at segment 0 where the
	// squasher is, and the higher stages cannot reach past the squasher.
	_, r, _ := stages.NextJob(math.MaxInt)
	require.Nil(t, r)
	require.GreaterOrEqual(t, stages.pendingFrom[0], segments-1, "stage 0 cursor stuck behind the unsquashed backlog")
	require.LessOrEqual(t, len(stages.candidateSegments(stages.nextJobCursor, segments-1)), len(stages.stages)*2, "a call visits a bounded number of segments")

	// A squash that finds no partial sends the unit back to Pending, and the cursor
	// must come back with it or the job would never be re-scheduled.
	stages.MarkSegmentMerging(Unit{Segment: 0, Stage: 0})
	stages.MarkSegmentPending(Unit{Segment: 0, Stage: 0})
	u, r, _ := stages.NextJob(math.MaxInt)
	require.NotNil(t, r)
	require.Equal(t, Unit{Segment: 0, Stage: 0}, u)
}

func TestNextJob_FullRunCompletes(t *testing.T) {
	scheduleRun(t, 300, 4, 3)
}

// An execout file found missing long after its segment was done sends the map unit
// back to Pending. The cursors have all moved to the end of the range by then, and
// the job must still come back out of NextJob.
func TestNextJob_ReschedulesAMapSegmentWhoseFileWentMissing(t *testing.T) {
	stages, _ := scheduleRun(t, 300, 4, 3)
	_, r, _ := stages.NextJob(math.MaxInt)
	require.Nil(t, r, "everything is done")

	mapStage := len(stages.stages) - 1
	require.True(t, stages.ReprocessMapSegment(150))

	u, r, _ := stages.NextJob(math.MaxInt)
	require.NotNil(t, r)
	require.Equal(t, Unit{Segment: 150, Stage: mapStage}, u)

	stages.MarkJobSuccess(u)
	_, r, _ = stages.NextJob(math.MaxInt)
	require.Nil(t, r)
	require.True(t, stages.LastStageCompleted())
}

func BenchmarkNextJob_SlowSquasher(b *testing.B) {
	for _, segments := range []int{1000, 2000, 4000, 8000} {
		b.Run(itoa(segments), func(b *testing.B) {
			var calls int
			for i := 0; i < b.N; i++ {
				_, calls = scheduleRun(b, segments, 4, 3)
			}
			b.ReportMetric(float64(calls), "nextjob_calls")
		})
	}
}

func itoa(i int) string {
	out := ""
	for i > 0 {
		out = string(rune('0'+i%10)) + out
		i /= 10
	}
	return out
}

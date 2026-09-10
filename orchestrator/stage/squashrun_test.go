package stage

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/streamingfast/substreams/orchestrator/loop"
	"github.com/streamingfast/substreams/orchestrator/plan"
	"github.com/streamingfast/substreams/pipeline/exec"
)

// newMergeTestStages returns stages with store segments 0 to 4 on stages 0 and 1, and
// segment 0 of stage 0 already merged.
func newMergeTestStages(t *testing.T) *Stages {
	t.Helper()
	reqPlan, err := plan.BuildTier1RequestPlan(true, 10, 5, 5, 5, 50, 50, true)
	require.NoError(t, err)
	s := NewStages(context.Background(), exec.TestGraphStagedModules(5, 5, 5, 5, 5), reqPlan, nil, nil)
	s.allocSegments(4)

	s.forceTransition(0, 0, UnitMerging)
	s.MergeCompleted(unit(0, 0))
	return s
}

func TestClaimMergeRun(t *testing.T) {
	t.Run("stops at the first segment without a partial", func(t *testing.T) {
		s := newMergeTestStages(t)
		s.forceTransition(1, 0, UnitPartialPresent)
		s.forceTransition(2, 0, UnitPartialPresent)
		s.forceTransition(4, 0, UnitPartialPresent)

		s.MarkSegmentMerging(unit(1, 0))
		run := s.claimMergeRun(s.stages[0], unit(1, 0), 10)

		assert.Equal(t, []Unit{unit(1, 0), unit(2, 0)}, run)
		assert.Equal(t, UnitMerging, s.getState(unit(2, 0)))
		assert.Equal(t, UnitPending, s.getState(unit(3, 0)))
		assert.Equal(t, UnitPartialPresent, s.getState(unit(4, 0)))
	})

	t.Run("stops at the limit", func(t *testing.T) {
		s := newMergeTestStages(t)
		for seg := 1; seg <= 4; seg++ {
			s.forceTransition(seg, 0, UnitPartialPresent)
		}

		s.MarkSegmentMerging(unit(1, 0))
		run := s.claimMergeRun(s.stages[0], unit(1, 0), 2)

		assert.Equal(t, []Unit{unit(1, 0), unit(2, 0)}, run)
		assert.Equal(t, UnitPartialPresent, s.getState(unit(3, 0)))
	})

	t.Run("stops at the last segment of the stage", func(t *testing.T) {
		s := newMergeTestStages(t)
		for seg := 1; seg <= 4; seg++ {
			s.forceTransition(seg, 0, UnitPartialPresent)
		}

		s.MarkSegmentMerging(unit(1, 0))
		run := s.claimMergeRun(s.stages[0], unit(1, 0), 100)

		assert.Equal(t, []Unit{unit(1, 0), unit(2, 0), unit(3, 0), unit(4, 0)}, run)
	})
}

func TestCmdTryMergeClaimsRun(t *testing.T) {
	s := newMergeTestStages(t)
	s.forceTransition(1, 0, UnitPartialPresent)
	s.forceTransition(2, 0, UnitPartialPresent)

	require.NotNil(t, s.CmdTryMerge(0))
	assert.Equal(t, UnitMerging, s.getState(unit(1, 0)))
	assert.Equal(t, UnitMerging, s.getState(unit(2, 0)))

	// the run is in flight: nothing more to merge on that stage until it reports back
	msg := s.CmdTryMerge(0)()
	assert.IsType(t, MsgMergeNotReady{}, msg)
}

func TestMergeRunCompleted(t *testing.T) {
	s := newMergeTestStages(t)
	for seg := 1; seg <= 3; seg++ {
		s.forceTransition(seg, 0, UnitPartialPresent)
	}
	s.MarkSegmentMerging(unit(1, 0))
	run := s.claimMergeRun(s.stages[0], unit(1, 0), 10)
	require.Len(t, run, 3)

	s.MergeRunCompleted(0, run[:1], run[1:])

	assert.Equal(t, UnitCompleted, s.getState(unit(1, 0)))
	assert.Equal(t, UnitPartialPresent, s.getState(unit(2, 0)))
	assert.Equal(t, UnitPartialPresent, s.getState(unit(3, 0)))
	assert.Equal(t, unit(2, 0), s.stages[0].nextUnit(), "the next run starts at the first unmerged unit")
}

func TestSquashRun(t *testing.T) {
	run := []Unit{unit(1, 0), unit(2, 0), unit(3, 0)}

	t.Run("squashes every unit in order", func(t *testing.T) {
		var squashed []Unit
		merged, unmerged, err := squashRun(run, time.Hour, func(u Unit) error {
			squashed = append(squashed, u)
			return nil
		})

		require.NoError(t, err)
		assert.Equal(t, run, squashed)
		assert.Equal(t, run, merged)
		assert.Empty(t, unmerged)
	})

	t.Run("squashes the first unit even with no budget left", func(t *testing.T) {
		merged, unmerged, err := squashRun(run, 0, func(Unit) error { return nil })

		require.NoError(t, err)
		assert.Equal(t, run[:1], merged)
		assert.Equal(t, run[1:], unmerged)
	})

	t.Run("stops at the first error", func(t *testing.T) {
		boom := errors.New("boom")
		merged, _, err := squashRun(run, time.Hour, func(u Unit) error {
			if u == unit(2, 0) {
				return boom
			}
			return nil
		})

		assert.ErrorIs(t, err, boom)
		assert.Equal(t, run[:1], merged)
	})
}

func TestCloseWaitsForSquashRun(t *testing.T) {
	s := newMergeTestStages(t)

	squashing := make(chan struct{})
	release := make(chan struct{})
	squashDone := make(chan loop.Msg, 1)
	go func() {
		squashDone <- s.guardSquash(func() loop.Msg {
			close(squashing)
			<-release
			return MsgMergeFinished{}
		})
	}()
	<-squashing

	closeDone := make(chan struct{})
	go func() {
		s.Close()
		close(closeDone)
	}()

	select {
	case <-closeDone:
		t.Fatal("Close returned while a squash run was in flight")
	case <-time.After(50 * time.Millisecond):
	}
	assert.Error(t, s.ctx.Err(), "Close cancels the stages context before waiting")

	close(release)
	assert.IsType(t, MsgMergeFinished{}, <-squashDone)
	select {
	case <-closeDone:
	case <-time.After(time.Second):
		t.Fatal("Close did not return once the squash run finished")
	}
}

func TestSquashSkippedAfterClose(t *testing.T) {
	s := newMergeTestStages(t)
	s.Close()

	ran := false
	msg := s.guardSquash(func() loop.Msg {
		ran = true
		return MsgMergeFinished{}
	})

	assert.False(t, ran)
	assert.IsType(t, MsgMergeFailed{}, msg)
}

func TestSquashRunStopsOnceContextCancelled(t *testing.T) {
	s := newMergeTestStages(t)
	for seg := 1; seg <= 3; seg++ {
		s.forceTransition(seg, 0, UnitPartialPresent)
	}
	s.MarkSegmentMerging(unit(1, 0))
	run := s.claimMergeRun(s.stages[0], unit(1, 0), 10)

	var squashed []Unit
	merged, _, err := squashRun(run, time.Hour, func(u Unit) error {
		if err := s.ctx.Err(); err != nil {
			return err
		}
		squashed = append(squashed, u)
		s.cancel()
		return nil
	})

	assert.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, run[:1], squashed, "the unit in progress finishes, the next one is not started")
	assert.Equal(t, run[:1], merged)
}

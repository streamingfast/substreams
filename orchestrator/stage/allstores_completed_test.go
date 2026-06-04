package stage

import (
	"testing"

	"github.com/streamingfast/substreams/block"
	"github.com/stretchr/testify/require"
)

// refAllStoresCompleted is the original, un-optimized full-scan implementation,
// kept here as the correctness oracle for the cursor-based AllStoresCompleted.
func refAllStoresCompleted(s *Stages) bool {
	if s.storeSegmenter == nil {
		return true
	}
	if s.storeSegmenter.ExclusiveEndBlock() == s.storeSegmenter.InitialBlock() {
		return true
	}
	lastSegment := s.storeSegmenter.LastIndex()
	for idx, stage := range s.stages {
		if stage.kind != KindStore {
			continue
		}
		for seg := s.storeSegmenter.FirstIndex(); seg <= lastSegment; seg++ {
			state := s.getState(Unit{Segment: seg, Stage: idx})
			if state != UnitCompleted && state != UnitNoOp {
				return false
			}
		}
	}
	return true
}

func newStoreStagesForTest(segCount, stageCount int) *Stages {
	seg := block.NewSegmenter(10, 0, uint64(segCount*10))
	stages := make([]*Stage, stageCount)
	for i := range stages {
		stages[i] = &Stage{kind: KindStore, segmenter: seg}
	}
	s := &Stages{
		storeSegmenter: seg,
		segmentOffset:  0,
		stages:         stages,
		segmentStates:  make([]stageStates, segCount),
	}
	for i := range s.segmentStates {
		s.segmentStates[i] = make(stageStates, stageCount)
	}
	return s
}

// TestAllStoresCompletedCursorMatchesReference drives a monotonic sequence of
// segment completions (Completed/NoOp are terminal in production) and checks that
// the cursor-based AllStoresCompleted agrees with the full-scan reference at every
// step, and only flips to true once everything is terminal.
func TestAllStoresCompletedCursorMatchesReference(t *testing.T) {
	const segCount, stageCount = 8, 3
	s := newStoreStagesForTest(segCount, stageCount)

	require.False(t, s.AllStoresCompleted())
	require.Equal(t, refAllStoresCompleted(s), s.AllStoresCompleted())

	// Complete units in a deliberately non-front-to-back, but monotonic, order to
	// make sure the cursor never advances past a still-pending segment.
	type cell struct{ seg, stage int }
	order := []cell{
		{0, 0}, {0, 1}, {1, 0}, {0, 2}, {2, 0}, {1, 1}, {1, 2},
		{3, 0}, {2, 1}, {4, 0}, {2, 2}, {3, 1}, {3, 2}, {5, 0},
		{4, 1}, {4, 2}, {6, 0}, {5, 1}, {5, 2}, {7, 0}, {6, 1},
		{6, 2}, {7, 1}, {7, 2},
	}
	// Mark a couple of early segments NoOp instead of Completed (a stage whose
	// modules start later) — also terminal, must be treated the same.
	noop := map[cell]bool{{0, 2}: true, {1, 2}: true}

	for i, c := range order {
		state := UnitCompleted
		if noop[c] {
			state = UnitNoOp
		}
		s.segmentStates[c.seg][c.stage] = state

		got := s.AllStoresCompleted()
		want := refAllStoresCompleted(s)
		require.Equalf(t, want, got, "step %d after marking seg=%d stage=%d", i, c.seg, c.stage)

		// Only the very last mutation may make it true.
		if i < len(order)-1 {
			require.Falsef(t, got, "completed too early at step %d", i)
		}
	}
	require.True(t, s.AllStoresCompleted())
	// Latches and stays true.
	require.True(t, s.AllStoresCompleted())
}

// TestAllStoresCompletedNonStoreStagesIgnored ensures map (non-store) stages are
// ignored, matching the reference.
func TestAllStoresCompletedNonStoreStagesIgnored(t *testing.T) {
	const segCount = 4
	seg := block.NewSegmenter(10, 0, segCount*10)
	s := &Stages{
		storeSegmenter: seg,
		stages: []*Stage{
			{kind: KindStore, segmenter: seg},
			{kind: KindMap, segmenter: seg}, // map stage stays PartialPresent forever
		},
		segmentStates: make([]stageStates, segCount),
	}
	for i := range s.segmentStates {
		s.segmentStates[i] = make(stageStates, 2)
		s.segmentStates[i][1] = UnitPartialPresent // map: never Completed
	}

	require.False(t, s.AllStoresCompleted())
	for i := 0; i < segCount; i++ {
		s.segmentStates[i][0] = UnitCompleted
	}
	require.Equal(t, refAllStoresCompleted(s), s.AllStoresCompleted())
	require.True(t, s.AllStoresCompleted())
}

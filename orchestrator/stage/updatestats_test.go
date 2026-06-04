package stage

import (
	"context"
	"sort"
	"testing"

	"github.com/streamingfast/substreams/block"
	"github.com/streamingfast/substreams/orchestrator/plan"
	"github.com/streamingfast/substreams/pipeline/exec"
	"github.com/stretchr/testify/require"
)

// refStatsMergedRanges reproduces the original UpdateStats range computation
// (per-stage map keyed by start block, collected, sorted, merged) as the oracle
// for the optimized single-pass statsRangesByStage.
func refStatsMergedRanges(s *Stages) []block.Ranges {
	out := make([]block.Ranges, len(s.stages))
	for stgIdx := range s.stages {
		br := make(map[uint64]*block.Range)
		for segmentIdx, segment := range s.segmentStates {
			state := segment[stgIdx]
			segmenter := s.stages[stgIdx].storeModuleStates[0].segmenter
			if state == UnitCompleted || state == UnitPartialPresent || state == UnitMerging {
				if rng := segmenter.Range(segmentIdx + s.segmentOffset); rng != nil {
					br[rng.StartBlock] = rng
				}
			}
		}
		blockRanges := block.Ranges(make([]*block.Range, len(br)))
		i := 0
		for _, v := range br {
			blockRanges[i] = v
			i++
		}
		sort.Sort(blockRanges)
		out[stgIdx] = blockRanges.Merged()
	}
	return out
}

func mergedAll(in []block.Ranges) []block.Ranges {
	out := make([]block.Ranges, len(in))
	for i := range in {
		out[i] = in[i].Merged()
	}
	return out
}

// TestStatsRangesByStageMatchesReference fills the segment matrix with a mix of
// states (including gaps and non-counted states) and checks the optimized
// single-pass range computation matches the original map+sort version exactly.
func TestStatsRangesByStageMatchesReference(t *testing.T) {
	reqPlan, err := plan.BuildTier1RequestPlan(true, 10, 0, 0, 0, 200, 200, true)
	require.NoError(t, err)
	s := NewStages(
		context.Background(),
		exec.TestGraphStagedModules(0, 0, 0, 0, 0),
		reqPlan,
		nil,
		nil,
	)

	last := s.globalSegmenter.LastIndex()
	s.allocSegments(last)

	// A deterministic mix: completed prefix, a gap, partials/merging, and some
	// non-counted states (Pending/Scheduled/NoOp/Shadowed) that must be excluded.
	states := []UnitState{
		UnitCompleted, UnitPartialPresent, UnitMerging, UnitPending,
		UnitScheduled, UnitNoOp, UnitShadowed,
	}
	for seg := 0; seg <= last; seg++ {
		for stg := range s.stages {
			s.segmentStates[seg-s.segmentOffset][stg] = states[(seg*3+stg)%len(states)]
		}
	}

	require.Equal(t, refStatsMergedRanges(s), mergedAll(s.statsRangesByStage()))
}

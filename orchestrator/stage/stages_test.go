package stage

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/test-go/testify/require"

	"github.com/streamingfast/substreams/block"
	"github.com/streamingfast/substreams/orchestrator/plan"
	"github.com/streamingfast/substreams/pipeline/exec"
)

func TestNewStages(t *testing.T) {
	//seg := block.NewSegmenter(10, 5, 75)
	reqPlan, err := plan.BuildTier1RequestPlan(true, 10, 5, 5, 5, 75, 75, true)
	assert.NoError(t, err)

	stages := NewStages(
		context.Background(),
		exec.TestGraphStagedModules(5, 7, 12, 22, 25),
		reqPlan,
		nil,
		nil,
	)

	assert.Equal(t, 8, stages.globalSegmenter.Count()) // from 5 to 75
	assert.Equal(t, true, stages.storeSegmenter.EndsOnInterval(6))
	assert.Equal(t, false, stages.globalSegmenter.EndsOnInterval(7))
	assert.Equal(t, 6, stages.storeSegmenter.IndexForStartBlock(60), "index in range")
	assert.Equal(t, 8, stages.storeSegmenter.IndexForStartBlock(80), "index out of range still returned here")
	assert.Nil(t, stages.storeSegmenter.Range(8), "out of range")

	assert.Equal(t, block.MustParseRange("5-10"), stages.storeSegmenter.Range(0))
	assert.Equal(t, block.MustParseRange("10-20"), stages.storeSegmenter.Range(1))
	assert.Equal(t, block.MustParseRange("70-75"), stages.storeSegmenter.Range(7))
	assert.Equal(t, block.MustParseRange("70-75"), stages.globalSegmenter.Range(7))
}

func TestNewStagesDepsInFuture(t *testing.T) {
	//  productionMode bool, segmentInterval, lowestInitialBlock, lowestStoreInitialBlock, resolvedStartBlock, linearHandoffBlock, exclusiveEndBlock uint64, scheduleStores bool
	reqPlan, err := plan.BuildTier1RequestPlan(true,
		10,   // segmentInterval
		0,    // lowestInitialBlock
		1000, // lowestStoreInitialBlock
		5,    // resolvedStartBlock
		100,  // linearHandoffBlock
		200,  // exclusiveEndBlock
		true)
	assert.NoError(t, err)

	stages := NewStages(
		context.Background(),
		exec.TestGraphStagedModules(1000, 1000, 1000, 1000, 0),
		reqPlan,
		nil,
		nil,
	)

	/*
				// All stores are NOOP because they start in the future
			    S:NNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNN
		        S:NNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNN
		        M:....................................................................................................
	*/
	assert.Equal(t, 10, stages.globalSegmenter.Count()) // 0 to 100 here we check that the linearHandoff block is the boundary for the global segmenter
	assert.Equal(t, 0, stages.storeSegmenter.Count())

	assert.Equal(t, 100, stages.storeSegmenter.FirstIndex())
	assert.Equal(t, -1, stages.storeSegmenter.LastIndex()) // segmenter is empty, there is "no" last index

	//	assert.Equal(t, 100, stages.storeSegmenter.Count()) // 0 to 100 here we check that the linearHandoff block is the boundary for the global segmenter

	//assert.Equal(t, true, stages.storeSegmenter.EndsOnInterval(6))
	//assert.Equal(t, false, stages.globalSegmenter.EndsOnInterval(7))
	//assert.Equal(t, 6, stages.storeSegmenter.IndexForStartBlock(60), "index in range")
	//assert.Equal(t, 8, stages.storeSegmenter.IndexForStartBlock(80), "index out of range still returned here")
	//assert.Nil(t, stages.storeSegmenter.Range(8), "out of range")

	//assert.Equal(t, block.MustParseRange("5-10"), stages.storeSegmenter.Range(0))
	//assert.Equal(t, block.MustParseRange("10-20"), stages.storeSegmenter.Range(1))
	//assert.Equal(t, block.MustParseRange("70-75"), stages.storeSegmenter.Range(7))
	//assert.Equal(t, block.MustParseRange("70-75"), stages.globalSegmenter.Range(7))
}

func unit(seg, stage int) Unit {
	return Unit{Segment: seg, Stage: stage}
}

func TestNewStageNextJobs(t *testing.T) {
	reqPlan, err := plan.BuildTier1RequestPlan(true, 10, 5, 5, 5, 50, 50, true)
	assert.NoError(t, err)
	assert.Equal(t, "interval=10, stores=[5, 50), map_write=[5, 50), map_read=[5, 50), linear=[nil)", reqPlan.String())
	stages := NewStages(
		context.Background(),
		exec.TestGraphStagedModules(5, 5, 5, 5, 5),
		reqPlan,
		nil,
		nil,
	)

	noNextJob := func() {
		_, r, _ := stages.NextJob(math.MaxInt)
		assert.Nil(t, r)
	}

	nextJob := func() Unit {
		j, r, _ := stages.NextJob(math.MaxInt)
		if r == nil {
			t.Error("no next job")
		}
		return j
	}

	merge := func(u Unit) {
		stages.forceTransition(u.Segment, u.Stage, UnitMerging)
		stages.MergeCompleted(u)
	}

	assert.Equal(t, unit(0, 2), nextJob())

	segmentStateEquals(t, stages, `
		S:Z
		S:Z
		M:S`)

	assert.Equal(t, unit(1, 0), nextJob())
	segmentStateEquals(t, stages, `
		S:ZS
		S:ZZ
		M:S.`)

	assert.Equal(t, unit(2, 0), nextJob())
	segmentStateEquals(t, stages, `
		S:ZSS
		S:ZZ.
		M:S..`)

	stages.MarkJobSuccess(unit(0, 2))
	segmentStateEquals(t, stages, `
		S:PSS
		S:PZ.
		M:P..`)

	merge(unit(0, 0))
	merge(unit(0, 1))

	assert.Equal(t, unit(3, 0), nextJob())
	segmentStateEquals(t, stages, `
		S:CSSS
		S:CZ..
		M:P...`)

	stages.MarkJobSuccess(unit(1, 0))
	merge(unit(1, 0))
	segmentStateEquals(t, stages, `
		S:CCSS
		S:CZ..
		M:P...`)

	assert.Equal(t, unit(1, 2), nextJob())
	segmentStateEquals(t, stages, `
		S:CCSS
		S:CZ..
		M:PS..`)

	assert.Equal(t, unit(4, 0), nextJob())
	segmentStateEquals(t, stages, `
		S:CCSSS
		S:CZ...
		M:PS...`)

	noNextJob()

	stages.MarkJobSuccess(unit(2, 0))
	merge(unit(2, 0))
	stages.MarkJobSuccess(unit(3, 0))
	merge(unit(3, 0))

	segmentStateEquals(t, stages, `
		S:CCCCS
		S:CZ...
		M:PS...`)

	assert.Equal(t, unit(2, 1), nextJob())
	assert.Equal(t, unit(3, 1), nextJob())
	segmentStateEquals(t, stages, `
		S:CCCCS
		S:CZSS.
		M:PS...`)

	stages.MarkJobSuccess(unit(2, 1))
	segmentStateEquals(t, stages, `
		S:CCCCS
		S:CZPS.
		M:PS...`)

	noNextJob()

	stages.MarkJobSuccess(unit(1, 2))
	segmentStateEquals(t, stages, `
		S:CCCCS
		S:CPPS.
		M:PP...`)

	noNextJob()

	stages.MarkJobSuccess(unit(4, 0))

	assert.Equal(t, unit(4, 1), nextJob())
	segmentStateEquals(t, stages, `
		S:CCCCP
		S:CPPSS
		M:PP...`)

	merge(unit(1, 1))
	merge(unit(2, 1))
	segmentStateEquals(t, stages, `
		S:CCCCP
		S:CCCSS
		M:PP...`)

	assert.Equal(t, unit(2, 2), nextJob())
	segmentStateEquals(t, stages, `
		S:CCCCP
		S:CCCSS
		M:PPS..`)

	stages.MarkJobSuccess(unit(3, 1))
	stages.MarkJobSuccess(unit(4, 1))
	assert.Equal(t, unit(3, 2), nextJob())

	segmentStateEquals(t, stages, `
		S:CCCCP
		S:CCCPP
		M:PPSS.`)

	noNextJob()

	merge(unit(3, 1))
	assert.Equal(t, unit(4, 2), nextJob())
	segmentStateEquals(t, stages, `
		S:CCCCP
		S:CCCCP
		M:PPSSS`)
}

func assertNoNextJob(t *testing.T, stages *Stages) {
	_, r, _ := stages.NextJob(math.MaxInt)
	assert.Nil(t, r)
}

func nextJob(t *testing.T, stages *Stages) Unit {
	j, r, _ := stages.NextJob(math.MaxInt)
	if r == nil {
		t.Error("no next job")
	}
	return j
}

func TestShadowSimple(t *testing.T) {

	reqPlan, err := plan.BuildTier1RequestPlan(
		true, // production mode
		10,   // segmentInterval
		5,    // lowestInitialBlock
		5,    // lowestStoreInitialBlock
		5,    // resolvedStartBlock
		50,   //linearHandoffBlock
		50,   //effectiveEndBlock
		true, // scheduleStores
	)
	assert.NoError(t, err)
	assert.Equal(t, "interval=10, stores=[5, 50), map_write=[5, 50), map_read=[5, 50), linear=[nil)", reqPlan.String())

	stages := NewStages(
		context.Background(),
		exec.TestGraphStagedModules(5, 5, 5, 5, 5),
		reqPlan,
		nil,
		nil,
	)

	segmentStateEquals(t, stages, `
		S:
		S:
		M:`)

	stages.shadowableSegment = 0
	stages.markShadowedUnits(0)
	segmentStateEquals(t, stages, `
		S:Z
		S:Z
		M:.`)

	assert.Equal(t, unit(0, 2), nextJob(t, stages))
	segmentStateEquals(t, stages, `
		S:Z
		S:Z
		M:S`)

	assert.Equal(t, unit(1, 0), nextJob(t, stages))
	assert.Equal(t, unit(2, 0), nextJob(t, stages))
	segmentStateEquals(t, stages, `
		S:ZSS
		S:ZZ.
		M:S..`)
}

// A unit whose upper neighbour is merging a partial found on disk must not be shadowed:
// no job will run for that upper unit, so nothing would ever un-shadow it and the stage
// would wait forever.
func TestShadowNotUnderMergeFromDisk(t *testing.T) {
	reqPlan, err := plan.BuildTier1RequestPlan(true, 10, 5, 5, 5, 50, 50, true)
	assert.NoError(t, err)

	stages := NewStages(
		context.Background(),
		exec.TestGraphStagedModules(5, 5, 5, 5, 5),
		reqPlan,
		nil,
		nil,
	)
	stages.allocSegments(0)
	stages.forceTransition(0, 0, UnitCompleted)
	stages.forceTransition(0, 1, UnitPending)
	stages.forceTransition(0, 2, UnitMerging)
	stages.shadowableSegment = 0

	stages.markShadowedUnits(0)
	segmentStateEquals(t, stages, `
		S:C
		S:.
		M:M`)

	stages.forceTransition(0, 2, UnitPartialPresent)
	stages.markShadowedUnits(0)
	segmentStateEquals(t, stages, `
		S:C
		S:.
		M:P`)

	stages.forceTransition(0, 2, UnitPending)
	stages.markShadowedUnits(0)
	segmentStateEquals(t, stages, `
		S:C
		S:Z
		M:.`)
}

func TestShadowStartAfter(t *testing.T) {
	reqPlan, err := plan.BuildTier1RequestPlan(true, 10, 5, 5, 30, 90, 90, true)
	assert.NoError(t, err)
	assert.Equal(t, "interval=10, stores=[5, 90), map_write=[30, 90), map_read=[30, 90), linear=[nil)", reqPlan.String())

	stages := NewStages(
		context.Background(),
		exec.TestGraphStagedModules(5, 5, 5, 5, 5),
		reqPlan,
		nil,
		nil,
	)

	stages.markSegmentCompleted(unit(0, 0))
	stages.markSegmentCompleted(unit(0, 1))
	stages.markSegmentCompleted(unit(1, 0))
	stages.markSegmentCompleted(unit(1, 1))

	segmentStateEquals(t, stages, `
		S:CC.
		S:CC.
		M:NNN`)

	stages.setShadowableSegment(3)
	require.Equal(t, 2, stages.shadowableSegment) // 4 is the first segment that has all its dependencies
	for i := 0; i <= 3; i++ {
		stages.markShadowedUnits(i) // called from 'offsetSegment` (0) to `startSegment` (4)
	}
	segmentStateEquals(t, stages, `
	        	            S:CCZ.
        	            	S:CC.Z
        	            	M:NNN.`)

	assert.Equal(t, unit(2, 1), nextJob(t, stages))
	assert.Equal(t, unit(3, 0), nextJob(t, stages))
	assert.Equal(t, unit(4, 0), nextJob(t, stages))
}

func TestShadowStartAfter2(t *testing.T) {
	reqPlan, err := plan.BuildTier1RequestPlan(true, 10, 5, 5, 40, 90, 90, true)
	assert.NoError(t, err)
	assert.Equal(t, "interval=10, stores=[5, 90), map_write=[40, 90), map_read=[40, 90), linear=[nil)", reqPlan.String())

	stages := NewStages(
		context.Background(),
		exec.TestGraphStagedModules(5, 5, 5, 5, 5),
		reqPlan,
		nil,
		nil,
	)

	stages.markSegmentCompleted(unit(0, 0))
	stages.markSegmentCompleted(unit(0, 1))
	stages.markSegmentCompleted(unit(1, 0))
	stages.markSegmentCompleted(unit(1, 1))

	segmentStateEquals(t, stages, `
		S:CC..
		S:CC..
		M:NNNN`)

	stages.setShadowableSegment(4)
	require.Equal(t, 2, stages.shadowableSegment) // 4 is the first segment that has all its dependencies
	for i := 0; i <= 4; i++ {
		stages.markShadowedUnits(i) // called from 'offsetSegment` (0) to `startSegment` (4)
	}
	segmentStateEquals(t, stages, `
	        	            S:CCZ..
        	            	S:CC...
        	            	M:NNNN.`)

	assert.Equal(t, unit(2, 1), nextJob(t, stages))
	assert.Equal(t, unit(3, 0), nextJob(t, stages))
	assert.Equal(t, unit(4, 0), nextJob(t, stages))
}

func segmentStateEquals(t *testing.T, s *Stages, segments string) {
	t.Helper()

	out := s.StatesString()

	lines := strings.FieldsFunc(segments, func(c rune) bool { return c == '\n' || c == '\r' })
	for i := range lines {
		lines[i] = strings.TrimSpace(lines[i])
	}
	canon := strings.Join(lines, "\n")

	assert.Equal(t, canon, strings.TrimSpace(out))
}

func TestStages_previousUnitComplete(t *testing.T) {
	s := Stages{
		storeSegmenter: block.NewSegmenter(10, 100, 200),
		segmentOffset:  10,
		segmentStates: []stageStates{
			{UnitPending, UnitPending},
			{UnitPending, UnitPending},
		},
	}
	u00 := Unit{Stage: 0, Segment: 10}
	u01 := Unit{Stage: 0, Segment: 11}
	assert.True(t, s.previousUnitComplete(u00))  // because of first boundary
	assert.False(t, s.previousUnitComplete(u01)) // u00 not complete
	s.setState(u00, UnitCompleted)
	assert.True(t, s.previousUnitComplete(u01)) // u00 is now complete
}

func TestStages_setShadowableSegment(t *testing.T) {
	tests := []struct {
		startSegment     int
		stages           []stageStates
		expectShadowable int
	}{
		{
			startSegment: 10,
			stages: []stageStates{
				{UnitCompleted, UnitCompleted},
			},
			expectShadowable: 10,
		},
		{
			startSegment:     11,
			stages:           []stageStates{},
			expectShadowable: 10,
		},
		{
			startSegment: 11,
			stages: []stageStates{
				{UnitCompleted, UnitCompleted},
				{UnitPending, UnitPending},
			},
			expectShadowable: 11,
		},
		{
			startSegment: 20,
			stages: []stageStates{
				{UnitCompleted, UnitCompleted},
				{UnitCompleted, UnitCompleted},
				{UnitPartialPresent, UnitCompleted},
				{UnitPartialPresent, UnitPending},
			},
			expectShadowable: 12,
		},
		{
			startSegment: 20,
			stages: []stageStates{
				{UnitCompleted, UnitCompleted},
				{UnitCompleted, UnitCompleted},
				{UnitCompleted, UnitCompleted},
				{UnitCompleted, UnitCompleted},
				{UnitCompleted, UnitCompleted},
				{UnitCompleted, UnitCompleted},
				{UnitCompleted, UnitCompleted},
				{UnitCompleted, UnitCompleted},
				{UnitCompleted, UnitCompleted},
				{UnitCompleted, UnitCompleted},
			},
			expectShadowable: 20,
		},
	}

	for i, tt := range tests {
		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
			s := Stages{
				storeSegmenter: block.NewSegmenter(10, 100, 200),
				segmentOffset:  10,
				segmentStates:  tt.stages,
				stages:         make([]*Stage, 2),
			}
			for i := 0; i < 2; i++ {
				s.stages[i] = &Stage{
					segmenter: s.storeSegmenter,
				}
			}
			s.setShadowableSegment(tt.startSegment)
			assert.Equal(t, tt.expectShadowable, s.shadowableSegment)
		})
	}
}

func TestStages_allocSegments(t *testing.T) {
	tests := []struct {
		offset       int
		allocSegment int
		expectLen    int
	}{
		{10, 11, 2},
		{0, 11, 12},
		{5, 1, 0},
		{5, 5, 1},
		{1, 5, 5},
	}
	for idx, tt := range tests {
		t.Run(strconv.Itoa(idx), func(t *testing.T) {
			s := &Stages{
				segmentOffset: tt.offset,
			}
			s.allocSegments(tt.allocSegment)
			assert.Len(t, s.segmentStates, tt.expectLen)
		})
	}
}

func TestStages_dependenciesCompleted(t *testing.T) {
	// makeDepsStages constructs a minimal Stages for testing dependenciesCompleted.
	// segmentOffset: the base index subtracted when indexing into segmentStates.
	// stageCount: number of stages (all sharing one segmenter for simplicity).
	// firstIndex: segment index where the segmenter starts
	//   (initialBlock = firstIndex * 10), so getState returns UnitNoOp for segments below it.
	// segStates: the state matrix indexed as [segmentIdx - segmentOffset][stageIdx].
	makeDepsStages := func(segmentOffset, stageCount, firstIndex int, segStates []stageStates) *Stages {
		seg := block.NewSegmenter(10, uint64(firstIndex)*10, uint64(firstIndex*10+10000))
		stages := make([]*Stage, stageCount)
		for i := 0; i < stageCount; i++ {
			stages[i] = &Stage{segmenter: seg}
		}
		return &Stages{
			segmentOffset: segmentOffset,
			segmentStates: segStates,
			stages:        stages,
		}
	}

	tests := []struct {
		name string
		s    *Stages
		unit Unit
		want bool
	}{
		// Stage 0 has no prior stages — always true.
		{
			name: "stage 0 always true regardless of other states",
			s:    makeDepsStages(0, 2, 0, []stageStates{{UnitPending, UnitPending}}),
			unit: Unit{Segment: 0, Stage: 0},
			want: true,
		},

		// --- Stage 1: each possible state for the single prior stage ---
		{
			name: "stage 1 parent completed",
			s:    makeDepsStages(0, 2, 0, []stageStates{{UnitCompleted, UnitPending}}),
			unit: Unit{Segment: 0, Stage: 1},
			want: true,
		},
		{
			name: "stage 1 parent noop",
			s:    makeDepsStages(0, 2, 0, []stageStates{{UnitNoOp, UnitPending}}),
			unit: Unit{Segment: 0, Stage: 1},
			want: true,
		},
		{
			name: "stage 1 parent pending",
			s:    makeDepsStages(0, 2, 0, []stageStates{{UnitPending, UnitPending}}),
			unit: Unit{Segment: 0, Stage: 1},
			want: false,
		},
		{
			name: "stage 1 parent scheduled",
			s:    makeDepsStages(0, 2, 0, []stageStates{{UnitScheduled, UnitPending}}),
			unit: Unit{Segment: 0, Stage: 1},
			want: false,
		},
		{
			name: "stage 1 parent merging",
			s:    makeDepsStages(0, 2, 0, []stageStates{{UnitMerging, UnitPending}}),
			unit: Unit{Segment: 0, Stage: 1},
			want: false,
		},
		{
			name: "stage 1 parent partial present",
			s:    makeDepsStages(0, 2, 0, []stageStates{{UnitMerging, UnitPartialPresent}}),
			unit: Unit{Segment: 0, Stage: 1},
			want: false,
		},

		// --- Completed parent whose previous segment is not done: its fullKV at the
		// segment start block is not guaranteed to exist (snapshots may be pruned) ---
		{
			name: "stage 1 parent completed but previous segment parent pending",
			s: makeDepsStages(0, 2, 0, []stageStates{
				{UnitPending, UnitPending},
				{UnitCompleted, UnitPending},
			}),
			unit: Unit{Segment: 1, Stage: 1},
			want: false,
		},
		{
			name: "stage 1 parent completed and previous segment parent noop",
			s: makeDepsStages(0, 2, 0, []stageStates{
				{UnitNoOp, UnitPending},
				{UnitCompleted, UnitPending},
			}),
			unit: Unit{Segment: 1, Stage: 1},
			want: true,
		},

		// --- Shadowed parent: allowed only when the previous segment's parent is done ---
		{
			name: "stage 1 parent shadowed prev-segment-parent completed",
			s: makeDepsStages(0, 2, 0, []stageStates{
				{UnitCompleted, UnitPending},
				{UnitShadowed, UnitPending},
			}),
			unit: Unit{Segment: 1, Stage: 1},
			want: true,
		},
		{
			// segmentOffset=1 means segment 0 is before the offset; getState returns UnitNoOp for it.
			name: "stage 1 parent shadowed prev-segment-parent noop (segment before offset)",
			s:    makeDepsStages(1, 2, 0, []stageStates{{UnitShadowed, UnitPending}}),
			unit: Unit{Segment: 1, Stage: 1},
			want: true,
		},
		{
			name: "stage 1 parent shadowed prev-segment-parent pending",
			s: makeDepsStages(0, 2, 0, []stageStates{
				{UnitPending, UnitPending},
				{UnitShadowed, UnitPending},
			}),
			unit: Unit{Segment: 1, Stage: 1},
			want: false,
		},

		// --- PartialPresent parent: same rule as Shadowed ---
		{
			name: "stage 1 parent partial-present prev-segment-parent completed",
			s: makeDepsStages(0, 2, 0, []stageStates{
				{UnitCompleted, UnitPending},
				{UnitPartialPresent, UnitPending},
			}),
			unit: Unit{Segment: 1, Stage: 1},
			want: true,
		},
		{
			name: "stage 1 parent partial-present prev-segment-parent noop (segment before offset)",
			s:    makeDepsStages(1, 2, 0, []stageStates{{UnitPartialPresent, UnitPending}}),
			unit: Unit{Segment: 1, Stage: 1},
			want: true,
		},
		{
			name: "stage 1 parent partial-present prev-segment-parent pending",
			s: makeDepsStages(0, 2, 0, []stageStates{
				{UnitPending, UnitPending},
				{UnitPartialPresent, UnitPending},
			}),
			unit: Unit{Segment: 1, Stage: 1},
			want: false,
		},

		// --- Multi-stage (stage 2 has two prior stages that must both be satisfied) ---
		{
			name: "stage 2 all prior stages completed",
			s:    makeDepsStages(0, 3, 0, []stageStates{{UnitCompleted, UnitCompleted, UnitPending}}),
			unit: Unit{Segment: 0, Stage: 2},
			want: true,
		},
		{
			name: "stage 2 first prior stage pending",
			s:    makeDepsStages(0, 3, 0, []stageStates{{UnitPending, UnitCompleted, UnitPending}}),
			unit: Unit{Segment: 0, Stage: 2},
			want: false,
		},
		{
			name: "stage 2 second prior stage pending",
			s:    makeDepsStages(0, 3, 0, []stageStates{{UnitCompleted, UnitPending, UnitPending}}),
			unit: Unit{Segment: 0, Stage: 2},
			want: false,
		},
		{
			// previousSegmentParent = getState({seg 0, stage 1}) = UnitCompleted → shadowed stage 1 is OK
			name: "stage 2 intermediate stage shadowed prev-segment-parent completed",
			s: makeDepsStages(0, 3, 0, []stageStates{
				{UnitCompleted, UnitCompleted, UnitPending},
				{UnitCompleted, UnitShadowed, UnitPending},
			}),
			unit: Unit{Segment: 1, Stage: 2},
			want: true,
		},
		{
			// previousSegmentParent = getState({seg 0, stage 1}) = UnitPending → shadowed stage 1 blocks
			name: "stage 2 intermediate stage shadowed prev-segment-parent pending",
			s: makeDepsStages(0, 3, 0, []stageStates{
				{UnitCompleted, UnitPending, UnitPending},
				{UnitCompleted, UnitShadowed, UnitPending},
			}),
			unit: Unit{Segment: 1, Stage: 2},
			want: false,
		},

		{
			name: "pending parent at the first segment of the segmenter must return false",
			s:    makeDepsStages(5, 2, 5, []stageStates{{UnitPending, UnitPending}}),
			unit: Unit{Segment: 5, Stage: 1},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.s.dependenciesCompleted(tt.unit))
		})
	}
}

func TestReprocessMapSegment(t *testing.T) {
	reqPlan, err := plan.BuildTier1RequestPlan(true, 10, 5, 5, 5, 50, 50, true)
	assert.NoError(t, err)
	stages := NewStages(context.Background(), exec.TestGraphStagedModules(5, 5, 5, 5, 5), reqPlan, nil, nil)

	stages.allocSegments(3)
	stages.forceTransition(3, 2, UnitCompleted)
	stages.nextJobCursor = 4

	assert.True(t, stages.ReprocessMapSegment(3))
	assert.Equal(t, UnitPending, stages.getState(Unit{Stage: 2, Segment: 3}))
	assert.Equal(t, 3, stages.nextJobCursor)

	assert.False(t, stages.ReprocessMapSegment(3), "already pending")
	stages.forceTransition(3, 2, UnitScheduled)
	assert.False(t, stages.ReprocessMapSegment(3), "job running")
}

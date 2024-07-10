package stage

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/streamingfast/substreams/block"
	"github.com/streamingfast/substreams/orchestrator/plan"
	"github.com/streamingfast/substreams/pipeline/exec"
)

func TestNewStages(t *testing.T) {
	//seg := block.NewSegmenter(10, 5, 75)
	reqPlan, err := plan.BuildTier1RequestPlan(true, 10, 5, 5, 75, 75, true)
	assert.NoError(t, err)

	stages := NewStages(
		context.Background(),
		exec.TestGraphStagedModules(5, 7, 12, 22, 25),
		reqPlan,
		nil,
	)

	assert.Equal(t, 8, stages.globalSegmenter.Count()) // from 5 to 75
	assert.Equal(t, true, stages.storeSegmenter.EndsOnInterval(6))
	assert.Equal(t, false, stages.globalSegmenter.EndsOnInterval(7))
	assert.Equal(t, 6, stages.storeSegmenter.IndexForStartBlock(60), "index in range")
	assert.Equal(t, 8, stages.storeSegmenter.IndexForStartBlock(80), "index out of range still returned here")
	assert.Nil(t, stages.storeSegmenter.Range(8), "out of range")

	assert.Equal(t, block.ParseRange("5-10"), stages.storeSegmenter.Range(0))
	assert.Equal(t, block.ParseRange("10-20"), stages.storeSegmenter.Range(1))
	assert.Equal(t, block.ParseRange("70-75"), stages.storeSegmenter.Range(7))
	assert.Equal(t, block.ParseRange("70-75"), stages.globalSegmenter.Range(7))
}

func unit(seg, stage int) Unit {
	return Unit{Segment: seg, Stage: stage}
}

func TestNewStageNextJobs(t *testing.T) {
	reqPlan, err := plan.BuildTier1RequestPlan(true, 10, 5, 5, 50, 50, true)
	assert.NoError(t, err)
	assert.Equal(t, "interval=10, stores=[5, 50), map_write=[5, 50), map_read=[5, 50), linear=[nil)", reqPlan.String())
	stages := NewStages(
		context.Background(),
		exec.TestGraphStagedModules(5, 5, 5, 5, 5),
		reqPlan,
		nil,
	)

	noNextJob := func() {
		_, r := stages.NextJob()
		assert.Nil(t, r)
	}

	nextJob := func() Unit {
		j, r := stages.NextJob()
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

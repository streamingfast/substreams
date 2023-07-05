package stage

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/streamingfast/substreams/block"
	"github.com/streamingfast/substreams/orchestrator/plan"
	"github.com/streamingfast/substreams/pipeline/outputmodules"
)

func TestNewStages(t *testing.T) {
	//seg := block.NewSegmenter(10, 5, 75)
	reqPlan := plan.BuildTier1RequestPlan(true, 10, 5, 5, 75, 75)

	stages := NewStages(
		context.Background(),
		outputmodules.TestGraphStagedModules(5, 7, 12, 22, 25),
		reqPlan,
		nil,
		"trace",
	)

	assert.Equal(t, 8, stages.storeSegmenter.Count()) // from 5 to 75
	assert.Equal(t, false, stages.storeSegmenter.EndsOnInterval(7))
	assert.Equal(t, 6, stages.storeSegmenter.IndexForStartBlock(60), "index in range")
	assert.Equal(t, 8, stages.storeSegmenter.IndexForStartBlock(80), "index out of range still returned here")
	assert.Nil(t, stages.storeSegmenter.Range(8), "out of range")

	assert.Equal(t, block.ParseRange("5-10"), stages.storeSegmenter.Range(0))
	assert.Equal(t, block.ParseRange("10-20"), stages.storeSegmenter.Range(1))
	assert.Equal(t, block.ParseRange("70-75"), stages.storeSegmenter.Range(7))
}

func TestNewStagesMapNotAlignedWithStoreEndBlock(t *testing.T) {
	reqPlan := plan.BuildTier1RequestPlan(true, 10, 5, 5, 75, 75)
	assert.Equal(t, "", reqPlan.String())

	stages := NewStages(
		context.Background(),
		outputmodules.TestGraphStagedModules(5, 7, 12, 22, 25),
		reqPlan,
		nil,
		"trace",
	)

	assert.Equal(t, 8, stages.storeSegmenter.Count()) // from 5 to 75
	assert.Equal(t, false, stages.storeSegmenter.EndsOnInterval(7))
	assert.Equal(t, 6, stages.storeSegmenter.IndexForStartBlock(60), "index in range")
	assert.Equal(t, 8, stages.storeSegmenter.IndexForStartBlock(80), "index out of range still returned here")
	assert.Nil(t, stages.storeSegmenter.Range(8), "out of range")

	assert.Equal(t, block.ParseRange("5-10"), stages.storeSegmenter.Range(0))
	assert.Equal(t, block.ParseRange("10-20"), stages.storeSegmenter.Range(1))
	assert.Equal(t, block.ParseRange("70-75"), stages.storeSegmenter.Range(7))
}

func TestNewStagesNextJobs(t *testing.T) {
	//seg := block.NewSegmenter(10, 5, 50)
	reqPlan := plan.BuildTier1RequestPlan(true, 10, 5, 5, 50, 50)
	stages := NewStages(
		context.Background(),
		outputmodules.TestGraphStagedModules(5, 5, 5, 5, 5),
		reqPlan,
		nil,
		"trace",
	)

	stages.allocSegments(0)
	stages.setState(Unit{Stage: 2, Segment: 0}, UnitNoOp)

	segmentStateEquals(t, stages, `
S:..
S:..
M:N.`)

	j1, _ := stages.NextJob()
	assert.Equal(t, 1, j1.Stage)
	assert.Equal(t, 0, j1.Segment)

	segmentStateEquals(t, stages, `
S:..
S:S.
M:N.`)

	stages.forceTransition(0, 1, UnitCompleted)

	segmentStateEquals(t, stages, `
S:..
S:C.
M:N.`)

	stages.NextJob()

	segmentStateEquals(t, stages, `
S:S.
S:C.
M:N.`)

	stages.NextJob()

	segmentStateEquals(t, stages, `
S:SS
S:C.
M:N.`)

	stages.forceTransition(0, 0, UnitCompleted)
	stages.NextJob()

	segmentStateEquals(t, stages, `
S:CS
S:C.
M:NS`)

	stages.forceTransition(1, 0, UnitCompleted)
	stages.NextJob()

	segmentStateEquals(t, stages, `
S:CC
S:CS
M:NS`)

	stages.NextJob()

	segmentStateEquals(t, stages, `
S:CC..
S:CSS.
M:NS..`)

	stages.MarkSegmentPartialPresent(id(1, 2))

	segmentStateEquals(t, stages, `
S:CC..
S:CSS.
M:NP..`)

	stages.MarkSegmentMerging(id(1, 2))

	segmentStateEquals(t, stages, `
S:CC..
S:CSS.
M:NM..`)

	stages.markSegmentCompleted(id(1, 2))
	stages.NextJob()

	segmentStateEquals(t, stages, `
S:CCS.
S:CSS.
M:NC..`)

	stages.NextJob()

	segmentStateEquals(t, stages, `
S:CCSS
S:CSS.
M:NC..`)

	stages.NextJob()

	segmentStateEquals(t, stages, `
S:CCSSS...
S:CSS.....
M:NC......`)

	stages.NextJob()

	segmentStateEquals(t, stages, `
S:CCSSSS..
S:CSS.....
M:NC......`)

	_, r := stages.NextJob()
	assert.Nil(t, r)
	stages.MarkSegmentPartialPresent(id(2, 0))

	segmentStateEquals(t, stages, `
S:CCPSSS..
S:CSS.....
M:NC......`)

	_, r = stages.NextJob()
	assert.Nil(t, r)
	stages.MarkSegmentMerging(id(2, 0))

	segmentStateEquals(t, stages, `
S:CCMSSS..
S:CSS.....
M:NC......`)

	_, r = stages.NextJob()
	assert.Nil(t, r)
	stages.markSegmentCompleted(id(2, 0))

	segmentStateEquals(t, stages, `
S:CCCSSS..
S:CSS.....
M:NC......`)

	stages.NextJob()

	segmentStateEquals(t, stages, `
S:CCCSSS..
S:CSSS....
M:NC......`)

	stages.forceTransition(1, 1, UnitCompleted)
	stages.NextJob()

	segmentStateEquals(t, stages, `
S:CCCSSS..
S:CCSS....
M:NCS.....`)

}

func id(segment, stage int) Unit {
	return Unit{Stage: stage, Segment: segment}
}

func segmentStateEquals(t *testing.T, s *Stages, segments string) {
	t.Helper()

	out := s.StatesString()

	assert.Equal(t, strings.TrimSpace(segments), strings.TrimSpace(out))
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

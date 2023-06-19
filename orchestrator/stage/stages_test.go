package stage

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/streamingfast/substreams/block"
	"github.com/streamingfast/substreams/pipeline/outputmodules"
)

func TestNewStages(t *testing.T) {
	seg := block.NewSegmenter(10, 5, 75)
	stages := NewStages(
		context.Background(),
		outputmodules.TestGraphStagedModules(5, 7, 12, 22, 25),
		seg,
		nil,
	)

	assert.Equal(t, 8, stages.segmenter.Count()) // from 5 to 75
	assert.Equal(t, true, stages.segmenter.IsPartial(7))
	assert.Equal(t, 6, stages.segmenter.IndexForStartBlock(60), "index in range")
	assert.Equal(t, 8, stages.segmenter.IndexForStartBlock(80), "index out of range still returned here")
	assert.Nil(t, stages.segmenter.Range(8), "out of range")

	assert.Equal(t, block.ParseRange("5-10"), stages.segmenter.Range(0))
	assert.Equal(t, block.ParseRange("10-20"), stages.segmenter.Range(1))
	assert.Equal(t, block.ParseRange("70-75"), stages.segmenter.Range(7))
}

func TestNewStagesNextJobs(t *testing.T) {
	seg := block.NewSegmenter(10, 5, 50)
	stages := NewStages(
		context.Background(),
		outputmodules.TestGraphStagedModules(5, 5, 5, 5, 5),
		seg,
		nil,
	)

	j1, _ := stages.NextJob()
	assert.Equal(t, 2, j1.Stage)
	assert.Equal(t, 0, j1.Segment)

	segmentStateEquals(t, stages, `
..
..
S.`)

	stages.forceTransition(0, 2, UnitCompleted)
	stages.NextJob()

	segmentStateEquals(t, stages, `
..
S.
C.`)

	stages.forceTransition(0, 1, UnitCompleted)

	segmentStateEquals(t, stages, `
..
C.
C.`)

	stages.NextJob()

	segmentStateEquals(t, stages, `
S.
C.
C.`)

	stages.NextJob()

	segmentStateEquals(t, stages, `
SS
C.
C.`)

	stages.forceTransition(0, 0, UnitCompleted)
	stages.NextJob()

	segmentStateEquals(t, stages, `
CS
C.
CS`)

	stages.forceTransition(1, 0, UnitCompleted)
	stages.NextJob()

	segmentStateEquals(t, stages, `
CC
CS
CS`)

	stages.NextJob()

	segmentStateEquals(t, stages, `
CC..
CSS.
CS..`)

	stages.MarkSegmentPartialPresent(id(1, 2))

	segmentStateEquals(t, stages, `
CC..
CSS.
CP..`)

	stages.MarkSegmentMerging(id(1, 2))

	segmentStateEquals(t, stages, `
CC..
CSS.
CM..`)

	stages.markSegmentCompleted(id(1, 2))
	stages.NextJob()

	segmentStateEquals(t, stages, `
CCS.
CSS.
CC..`)

	stages.NextJob()

	segmentStateEquals(t, stages, `
CCSS
CSS.
CC..`)

	stages.NextJob()

	segmentStateEquals(t, stages, `
CCSSS...
CSS.....
CC......`)

	stages.NextJob()

	segmentStateEquals(t, stages, `
CCSSSS..
CSS.....
CC......`)

	_, r := stages.NextJob()
	assert.Nil(t, r)
	stages.MarkSegmentPartialPresent(id(2, 0))

	segmentStateEquals(t, stages, `
CCPSSS..
CSS.....
CC......`)

	_, r = stages.NextJob()
	assert.Nil(t, r)
	stages.MarkSegmentMerging(id(2, 0))

	segmentStateEquals(t, stages, `
CCMSSS..
CSS.....
CC......`)

	_, r = stages.NextJob()
	assert.Nil(t, r)
	stages.markSegmentCompleted(id(2, 0))

	segmentStateEquals(t, stages, `
CCCSSS..
CSS.....
CC......`)

	stages.NextJob()

	segmentStateEquals(t, stages, `
CCCSSS..
CSSS....
CC......`)

	stages.forceTransition(1, 1, UnitCompleted)
	stages.NextJob()

	segmentStateEquals(t, stages, `
CCCSSS..
CCSS....
CCS.....`)

}

func id(segment, stage int) Unit {
	return Unit{Stage: stage, Segment: segment}
}

func segmentStateEquals(t *testing.T, s *Stages, segments string) {
	t.Helper()

	out := strings.Builder{}
	for i := 0; i < len(s.stages); i++ {
		for _, segment := range s.segmentStates {
			out.WriteString(map[UnitState]string{
				UnitPending:        ".",
				UnitPartialPresent: "P",
				UnitScheduled:      "S",
				UnitMerging:        "M",
				UnitCompleted:      "C",
			}[segment[i]])
		}
		out.WriteString("\n")
	}

	assert.Equal(t, strings.TrimSpace(segments), strings.TrimSpace(out.String()))
}

func TestStages_previousUnitComplete(t *testing.T) {
	s := Stages{
		segmenter:     block.NewSegmenter(10, 100, 200),
		segmentOffset: 10,
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

package stage

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/streamingfast/substreams/block"
	"github.com/streamingfast/substreams/pipeline/outputmodules"
)

func TestStages(t *testing.T) {
	s := &Stages{
		stages: []*Stage{
			&Stage{kind: KindStore},
			&Stage{kind: KindStore},
			&Stage{kind: KindMap},
		},
		Segmenter: block.NewSegmenter(10, 5, 35),
	}

	assert.Equal(t, true, s.dependenciesCompleted(0, 1))
	segID := s.NextJob()
	require.NotNil(t, segID)
	assert.Equal(t, 1, segID.Stage)
	assert.Equal(t, 2, segID.Segment)
	assert.Equal(t, block.ParseRange("10-20"), segID.Range)
}

func TestNewStages(t *testing.T) {
	stages := NewStages(outputmodules.TestGraphStagedModules(5, 7, 12, 22, 25), 10, 75)
	assert.Equal(t, 8, stages.Count()) // from 5 to 75
	assert.Equal(t, true, stages.IsPartial(7))
	assert.Equal(t, 6, stages.IndexForBlock(60))
	assert.Equal(t, 6, stages.IndexForBlock(60))
	assert.Panics(t, func() { stages.IndexForBlock(80) })
	assert.Equal(t, block.ParseRange("5-10"), stages.Range(0))
	assert.Equal(t, block.ParseRange("10-20"), stages.Range(1))
	assert.Equal(t, block.ParseRange("70-75"), stages.Range(7))
	assert.Panics(t, func() { stages.Range(8) })
	assert.Equal(t, 0, stages.completedSegments)
}

func TestNewStagesNextJobs(t *testing.T) {
	stages := NewStages(outputmodules.TestGraphStagedModules(5, 5, 5, 5, 5), 10, 50)

	j1 := stages.NextJob()
	assert.Equal(t, 2, j1.Stage)
	assert.Equal(t, 0, j1.Segment)
	assert.Equal(t, block.ParseRange("5-10"), j1.Range)

	segmentStateEquals(t, stages, `
PP
PP
SP`)

	stages.MarkJobCompleted(0, 2)
	stages.NextJob()

	segmentStateEquals(t, stages, `
PP
SP
CP`)

	stages.MarkJobCompleted(0, 1)

	segmentStateEquals(t, stages, `
PP
CP
CP`)

	stages.NextJob()

	segmentStateEquals(t, stages, `
SP
CP
CP`)

	stages.NextJob()

	segmentStateEquals(t, stages, `
SS
CP
CP`)

	stages.MarkJobCompleted(0, 0)
	stages.NextJob()

	segmentStateEquals(t, stages, `
CS
CP
CS`)

	stages.MarkJobCompleted(1, 0)
	stages.NextJob()

	segmentStateEquals(t, stages, `
CC
CS
CS`)

	stages.NextJob()

	segmentStateEquals(t, stages, `
CCPP
CSSP
CSPP`)

	stages.MarkJobCompleted(1, 2)
	stages.NextJob()

	segmentStateEquals(t, stages, `
CCSP
CSSP
CCPP`)

	stages.NextJob()

	segmentStateEquals(t, stages, `
CCSS
CSSP
CCPP`)

	stages.NextJob()

	segmentStateEquals(t, stages, `
CCSSSPPP
CSSPPPPP
CCPPPPPP`)

	stages.NextJob()

	segmentStateEquals(t, stages, `
CCSSSSPP
CSSPPPPP
CCPPPPPP`)

	assert.Nil(t, stages.NextJob())

	stages.MarkJobCompleted(2, 0)
	stages.NextJob()

	segmentStateEquals(t, stages, `
CCCSSSPP
CSSSPPPP
CCPPPPPP`)

	stages.MarkJobCompleted(1, 1)
	stages.NextJob()

	segmentStateEquals(t, stages, `
CCCSSSPP
CCSSPPPP
CCSPPPPP`)

}

func segmentStateEquals(t *testing.T, s *Stages, segments string) {
	t.Helper()

	out := strings.Builder{}
	for i := 0; i < len(s.stages); i++ {
		for _, segment := range s.state {
			out.WriteString(map[SegmentState]string{
				SegmentPending:   "P",
				SegmentScheduled: "S",
				SegmentCompleted: "C",
			}[segment[i]])
		}
		out.WriteString("\n")
	}

	assert.Equal(t, strings.TrimSpace(segments), strings.TrimSpace(out.String()))
}

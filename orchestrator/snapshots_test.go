package orchestrator

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSnapshots_LastCompleted(t *testing.T) {
	assert.Equal(t, 300, int((&Snapshots{
		Completes: parseRanges("100-200,100-300"),
		Partials:  parseRanges("300-400"),
	}).LastCompletedBlock()))

	assert.Equal(t, 0, int((&Snapshots{
		Completes: parseRanges(""),
		Partials:  parseRanges("200-300"),
	}).LastCompletedBlock()))
}

func TestSnapshots_LastCompleteBefore(t *testing.T) {
	s := &Snapshots{
		Completes: parseRanges("10-20,10-50,10-1000"),
	}

	assert.Nil(t, s.LastCompleteSnapshotBefore(0))
	assert.Nil(t, s.LastCompleteSnapshotBefore(5))
	assert.Nil(t, s.LastCompleteSnapshotBefore(19))
	assert.Equal(t, 20, int(s.LastCompleteSnapshotBefore(20).ExclusiveEndBlock))
	assert.Equal(t, 20, int(s.LastCompleteSnapshotBefore(21).ExclusiveEndBlock))
	assert.Equal(t, 20, int(s.LastCompleteSnapshotBefore(49).ExclusiveEndBlock))
	assert.Equal(t, 50, int(s.LastCompleteSnapshotBefore(50).ExclusiveEndBlock))
	assert.Equal(t, 50, int(s.LastCompleteSnapshotBefore(51).ExclusiveEndBlock))
	assert.Equal(t, 50, int(s.LastCompleteSnapshotBefore(999).ExclusiveEndBlock))
	assert.Equal(t, 1000, int(s.LastCompleteSnapshotBefore(1000).ExclusiveEndBlock))
	assert.Equal(t, 1000, int(s.LastCompleteSnapshotBefore(1001).ExclusiveEndBlock))
	assert.Equal(t, 1000, int(s.LastCompleteSnapshotBefore(10000).ExclusiveEndBlock))

	s = &Snapshots{
		Completes: parseRanges(""),
	}
	assert.Nil(t, s.LastCompleteSnapshotBefore(0))
	assert.Nil(t, s.LastCompleteSnapshotBefore(5))
}

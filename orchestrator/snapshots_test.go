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

	assert.Equal(t, 0, int(s.LastCompleteBefore(0)))
	assert.Equal(t, 0, int(s.LastCompleteBefore(5)))
	assert.Equal(t, 0, int(s.LastCompleteBefore(19)))
	assert.Equal(t, 20, int(s.LastCompleteBefore(20)))
	assert.Equal(t, 20, int(s.LastCompleteBefore(21)))
	assert.Equal(t, 20, int(s.LastCompleteBefore(49)))
	assert.Equal(t, 50, int(s.LastCompleteBefore(50)))
	assert.Equal(t, 50, int(s.LastCompleteBefore(51)))
	assert.Equal(t, 50, int(s.LastCompleteBefore(999)))
	assert.Equal(t, 1000, int(s.LastCompleteBefore(1000)))
	assert.Equal(t, 1000, int(s.LastCompleteBefore(1001)))
	assert.Equal(t, 1000, int(s.LastCompleteBefore(10000)))

	s = &Snapshots{
		Completes: parseRanges(""),
	}
	assert.Equal(t, 0, int(s.LastCompleteBefore(0)))
	assert.Equal(t, 0, int(s.LastCompleteBefore(5)))
}

package scheduler

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSched2_JobFinished(t *testing.T) {
	s := &Scheduler{}
	TestRunNone(s.Update(JobFinished{JobID: "123"}))
	assert.Equal(t, 1, len(s.cmds))
}

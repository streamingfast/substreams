package scheduler

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/streamingfast/substreams/orchestrator/execout"
)

func TestSched2_JobFinished(t *testing.T) {
	s := &Scheduler{}
	s.Update(execout.MsgFileDownloaded{})
	assert.Equal(t, 1, len(s.cmds))
}

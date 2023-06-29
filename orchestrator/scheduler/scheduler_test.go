package scheduler

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/streamingfast/substreams/orchestrator/execout"
	"github.com/streamingfast/substreams/orchestrator/loop"
)

func TestSched2_JobFinished(t *testing.T) {
	s := &Scheduler{
		ExecOutWalker: newTestWalker(),
	}
	cmd := s.Update(execout.MsgFileDownloaded{})
	msg := cmd()
	assert.Equal(t, 1, len(msg.(loop.BatchMsg)))
}

func newTestWalker() Walker {

}

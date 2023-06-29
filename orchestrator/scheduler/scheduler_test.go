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
	// TODO: implement interfaces on the Scheduler, so it can be easily tested
	// here and there.

	// Stages:
	//  * MergeCompleted
	//  * MarkSegmentPartialPresent
	//  * NextJob
	//  * CmdTryMerge
	//  * WaitAsyncWork
	//  * CmdStartMerge  (outside of Update loop though)
	//  * FinalStoreMap  (outside of Update loop though)
	// WorkerPool:  - No need to interface-ize
	//  * Borrow / Return
	// Worker returned here:
	//  * Work()
	// ExecOutWalker:
	//  * CmdDownloadCurrentSegment()
	//  * NextSegment()

}

func newTestWalker() Walker {

}

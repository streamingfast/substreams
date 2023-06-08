package work

import (
	"github.com/streamingfast/substreams/orchestrator/loop"
	"github.com/streamingfast/substreams/orchestrator/stage"
	"github.com/streamingfast/substreams/storage/store"
)

// Messages

type MsgJobFailed struct {
	SegmentID *stage.SegmentID

	Error error
}

type MsgJobSucceeded struct {
	SegmentID *stage.SegmentID

	Files store.FileInfos
}

type MsgScheduleNextJob struct{}

type MsgWorkerFreed struct {
	Worker Worker
}

//
//// Other types
//
//type JobID string
//
//func (j JobID) JobID() string {
//	return string(j)
//}
//
//type JobIDer interface {
//	JobID() string
//}

// Commands

func CmdScheduleNextJob() loop.Cmd {
	return func() loop.Msg {
		return MsgScheduleNextJob{}
	}
}

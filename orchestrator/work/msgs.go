package work

import "github.com/streamingfast/substreams/orchestrator/loop"

// Messages

type MsgJobFailed struct {
	JobID   string
	Stage   int
	Segment int
}

type MsgJobSucceeded struct {
	JobID   string
	Stage   int
	Segment int
}

type MsgJobStarted struct {
	JobID   string
	Stage   int
	Segment int
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

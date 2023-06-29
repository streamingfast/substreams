package work

import (
	"github.com/streamingfast/substreams/orchestrator/loop"
	"github.com/streamingfast/substreams/orchestrator/stage"
)

// Messages

type MsgJobFailed struct {
	Unit  stage.Unit
	Error error
}

type MsgJobSucceeded struct {
	Unit   stage.Unit
	Worker Worker
}

type MsgScheduleNextJob struct{}

func CmdScheduleNextJob() loop.Cmd {
	return func() loop.Msg {
		return MsgScheduleNextJob{}
	}
}

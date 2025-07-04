package work

import (
	"github.com/streamingfast/substreams/orchestrator/loop"
	"github.com/streamingfast/substreams/orchestrator/stage"
)

// Messages

type MsgJobFailed struct {
	loop.IsMsg
	Unit   stage.Unit
	Worker Worker
	Error  error
}

type MsgJobSucceeded struct {
	loop.IsMsg
	Unit     stage.Unit
	Worker   Worker
	Streamed bool
}

type MsgPendingShutdown struct {
	loop.IsMsg
}

type MsgScheduleNextJob struct {
	loop.IsMsg
	TriggerBy string
}

func CmdScheduleNextJob(triggerBy string) loop.Cmd {
	return func() loop.Msg {
		return MsgScheduleNextJob{
			TriggerBy: triggerBy,
		}
	}
}

type DelayedMsgScheduleNextJob struct {
	loop.IsMsg
	TriggerBy string
}

func CmdDelayedScheduleNextJob(triggerBy string) loop.Cmd {
	return func() loop.Msg {
		return DelayedMsgScheduleNextJob{
			TriggerBy: triggerBy,
		}
	}
}

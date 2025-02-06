package work

import (
	"time"

	"github.com/streamingfast/substreams/orchestrator/loop"
	"github.com/streamingfast/substreams/orchestrator/stage"
)

// Messages

type MsgJobFailed struct {
	loop.IsMsg
	Unit  stage.Unit
	Error error
}

type MsgJobSucceeded struct {
	loop.IsMsg
	Unit   stage.Unit
	Worker Worker
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
	Delay     time.Duration
}

func CmdDelayedScheduleNextJob(triggerBy string, delay time.Duration) loop.Cmd {
	return func() loop.Msg {
		return DelayedMsgScheduleNextJob{
			TriggerBy: triggerBy,
			Delay:     delay,
		}
	}
}

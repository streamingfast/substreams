package stage

import "github.com/streamingfast/substreams/orchestrator/loop"

// This means that this single Store has completed its full sync, up to the target block
type MsgAllStoresCompleted struct {
	loop.IsMsg
	Unit
}

func CmdAllStoresCompleted() loop.Cmd {
	return func() loop.Msg {
		return MsgAllStoresCompleted{}
	}
}

type MsgMergeFinished struct {
	loop.IsMsg
	Unit
} // A single partial store was successfully merged into the full store.

type MsgMergeFailed struct {
	loop.IsMsg
	Unit
	Error error
}

type MsgMergeNotReady struct {
	loop.IsMsg
	Reason   string
	NextUnit Unit
}

func CmdMergeNotReady(nextUnit Unit, reason string) loop.Cmd {
	return func() loop.Msg {
		return MsgMergeNotReady{NextUnit: nextUnit, Reason: reason}
	}
}

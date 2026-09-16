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

// MsgMergeFinished reports a squash run on one stage: the Merged units were squashed into
// the full stores, in order, and the Unmerged ones were claimed by the run but left for
// the next one.
type MsgMergeFinished struct {
	loop.IsMsg
	Stage    int
	Merged   []Unit
	Unmerged []Unit
}

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

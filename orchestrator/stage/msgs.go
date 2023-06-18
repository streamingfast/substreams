package stage

import "github.com/streamingfast/substreams/orchestrator/loop"

// This means that this single Store has completed its full sync, up to the target block
type MsgStoresCompleted struct {
	Unit
}

type MsgMergeFinished struct {
	Unit
} // A single partial store was successfully merged into the full store.

type MsgMergeFailed struct {
	Unit
	Error error
}

type MsgMergeStage struct {
	Unit
}

func CmdMergeStage(u Unit) loop.Cmd {
	return func() loop.Msg {
		return MsgMergeStage{u}
	}
}

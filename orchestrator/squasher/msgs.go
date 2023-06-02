package squasher

type MsgMergeStarted struct{}

type MsgMergeFinished struct{}

type MsgMergeFailed struct{}

type MsgMergeStage struct {
	Stage   int
	Segment int
}

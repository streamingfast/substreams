package squasher

// This means that this single Store has completed its full sync, up to the target block
type MsgStoreCompleted struct{}

type MsgMergeFinished struct {
	ModuleName string
} // A single partial store was successfully merged into the full store.

type MsgMergeFailed struct{}

type MsgMergeStage struct {
	Stage   int
	Segment int
}

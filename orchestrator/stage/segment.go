package stage

// State for a given StageSegment in a given Stage
type StageSegment struct {
	state SegmentState
}

type SegmentState int

const (
	SegmentPending SegmentState = iota
	SegmentScheduled
	SegmentCompleted
)

// SegmentID can be used as a key, and points to the respective indexes of
// Stages::segments[SegmentID.Segment][SegmentID.Stage]
type SegmentID struct {
	Segment int
	Stage   int
}

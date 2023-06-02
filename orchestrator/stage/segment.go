package stage

import "github.com/streamingfast/substreams/block"

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
	Range   *block.Range
}

package stage

import (
	"github.com/streamingfast/substreams/block"
	pbssinternal "github.com/streamingfast/substreams/pb/sf/substreams/intern/v2"
	"github.com/streamingfast/substreams/reqctx"
)

type UnitState int

const (
	UnitPending UnitState = iota // The job needs to be scheduled, no complete store exists at the end of its Range, nor any partial store for the end of this segment.
	UnitPartialPresent
	UnitScheduled // Means the job was scheduled for execution
	UnitMerging   // A partial is being merged
	UnitCompleted // End state. A store has been snapshot for this segment, and we have gone over in the per-request squasher
)

// Unit can be used as a key, and points to the respective indexes of
// Stages::unitStates[Unit.Segment][Unit.Stage]
type Unit struct {
	Segment int
	Stage   int
	Range   *block.Range
}

func (i Unit) NewRequest(req *reqctx.RequestDetails) *pbssinternal.ProcessRangeRequest {
	return &pbssinternal.ProcessRangeRequest{
		StartBlockNum: i.Range.StartBlock,
		StopBlockNum:  i.Range.ExclusiveEndBlock,
		Modules:       req.Modules,
		OutputModule:  req.OutputModule,
		Stage:         uint32(i.Stage),
	}
}

func (s UnitState) String() string {
	switch s {
	case UnitPending:
		return "Pending"
	case UnitPartialPresent:
		return "PartialPresent"
	case UnitScheduled:
		return "Scheduled"
	case UnitMerging:
		return "Merging"
	case UnitCompleted:
		return "Completed"
	default:
		return "Unknown"
	}
}

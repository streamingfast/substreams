package stage

import (
	"go.uber.org/zap/zapcore"
)

type UnitState int

const (
	UnitPending UnitState = iota // The job needs to be scheduled, no complete store exists at the end of its Range, nor any partial store for the end of this segment.
	UnitPartialPresent
	UnitScheduled // Means the job was scheduled for execution
	UnitMerging   // A partial is being merged
	UnitCompleted // End state. A store has been snapshot for this segment, and we have gone over in the per-request squasher
	UnitNoOp      // State given to a unit that does not need scheduling. Mostly for map segments where we know in advance we won't consume the output.
)

// Unit can be used as a key, and points to the respective indexes of
// Stages.getState(unit)
type Unit struct {
	Segment int
	Stage   int
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
	case UnitNoOp:
		return "NoOp"
	default:
		return "Unknown"
	}
}

func (u Unit) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	enc.AddInt("segment", u.Segment)
	enc.AddInt("stage", u.Stage)
	return nil
}

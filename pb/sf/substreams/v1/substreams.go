package pbsubstreams

import "github.com/streamingfast/bstream"

func StepToProto(step bstream.StepType) ForkStep {
	switch step {
	case bstream.StepNew:
		return ForkStep_STEP_NEW
	case bstream.StepUndo:
		return ForkStep_STEP_UNDO
	case bstream.StepIrreversible:
		return ForkStep_STEP_IRREVERSIBLE
	}
	return ForkStep_STEP_UNKNOWN
}

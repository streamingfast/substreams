package pbsubstreams

import (
	"fmt"

	"github.com/streamingfast/bstream"
)

// GetOutputModuleName is a helper to retrieve the output module name as we transition to a single output
// the assumption is that the Request has been validated before hence we can assume to there is 1 element in outputModules
func (x *Request) GetOutputModuleName() string {
	if x.OutputModule != "" {
		return x.OutputModule
	}
	return x.OutputModules[0]
}

func StepToProto(step bstream.StepType, finalBlocksOnly bool) (out ForkStep, skip bool) {
	if finalBlocksOnly {
		if step.Matches(bstream.StepIrreversible) {
			return ForkStep_STEP_IRREVERSIBLE, false
		}
		return ForkStep_STEP_UNKNOWN, true
	}

	if step.Matches(bstream.StepNew) {
		return ForkStep_STEP_NEW, false
	}
	if step.Matches(bstream.StepUndo) {
		return ForkStep_STEP_UNDO, false
	}
	return ForkStep_STEP_UNKNOWN, true // simply skip irreversible or stalled here
}

type ModuleOutputData interface {
	isModuleOutput_Data()
}

func ValidateRequest(req *Request) error {
	allMods := map[string]bool{}
	seenStores := map[string]bool{}

	if req.OutputModule == "" && len(req.OutputModules) > 1 {
		return fmt.Errorf("multiple output modules is not accepted")
	}

	for _, mod := range req.Modules.Modules {
		allMods[mod.Name] = true
		if _, ok := mod.Kind.(*Module_KindStore_); ok {
			seenStores[mod.Name] = true
		}

	}

	//TODO: should we remove this
	for _, outMod := range req.InitialStoreSnapshotForModules {
		if !seenStores[outMod] {
			return fmt.Errorf("initial store snapshots for module: %q: no such 'store' module defined modules graph", outMod)
		}
	}
	return nil
}

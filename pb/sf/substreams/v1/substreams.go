package pbsubstreams

import (
	"fmt"

	"github.com/streamingfast/bstream"
)

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
	seenMods := map[string]bool{}
	seenStores := map[string]bool{}
	for _, mod := range req.Modules.Modules {
		seenMods[mod.Name] = true
		if _, ok := mod.Kind.(*Module_KindStore_); ok {
			seenStores[mod.Name] = true
		}
	}

	for _, outMod := range req.OutputModules {
		if !seenMods[outMod] {
			return fmt.Errorf("output module %q requested but not defined modules graph", outMod)
		}
	}

	for _, outMod := range req.InitialStoreSnapshotForModules {
		if !seenStores[outMod] {
			if seenMods[outMod] {
				return fmt.Errorf("initial store snapshots for modules: %q: not a 'store' module", outMod)
			}
			return fmt.Errorf("initial store snapshots for module: %q: not defined modules graph", outMod)
		}
	}

	return nil
}

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
	outMods := map[string]bool{}
	allMods := map[string]bool{}
	seenStores := map[string]bool{}
	for _, mod := range req.Modules.Modules {
		allMods[mod.Name] = true
		if _, ok := mod.Kind.(*Module_KindStore_); ok {
			seenStores[mod.Name] = true
		}
	}
	for _, outMod := range req.OutputModules {
		outMods[outMod] = true
		if !allMods[outMod] {
			return fmt.Errorf("output module %q requested but not defined modules graph", outMod)
		}
	}

	for _, outMod := range req.InitialStoreSnapshotForModules {
		if !seenStores[outMod] {
			return fmt.Errorf("initial store snapshots for module: %q: no such 'store' module defined modules graph", outMod)
		}
	}

	if req.ProductionMode {
		if err := validateProductionMode(req); err != nil {
			return fmt.Errorf("production_mode: %w", err)
		}
	}

	return nil
}

func validateProductionMode(req *Request) error {
	if len(req.OutputModules) != 1 {
		return fmt.Errorf("output_modules need to be a single map module")
	}

	outModName := req.OutputModules[0]
	for _, mod := range req.Modules.Modules {
		if outModName == mod.Name {
			if _, ok := mod.Kind.(*Module_KindMap_); !ok {
				return fmt.Errorf("the single output_modules specified needs to be of kind 'map'")
			}
		}
	}
	return nil
}

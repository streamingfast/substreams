package pbsubstreams

import (
	"fmt"

	"github.com/streamingfast/bstream"
)

// GetOutputModuleName is a helper to retrieve the output module name as we transition to a single output
// from either `OutputModule` (priority if non-empty) or `OutputModules`. If an output module was found,
// return `<name>, true` otherwise returns `"", false`.
func (x *Request) GetOutputModuleName() (string, bool) {
	if x.OutputModule != "" {
		return x.OutputModule, true
	}

	if len(x.OutputModules) > 0 {
		return x.OutputModules[0], true
	}

	return "", false
}

// MustGetOutputModuleName is like #GetOutputModuleName but panics if no output module is found.
func (x *Request) MustGetOutputModuleName() string {
	outputModule, found := x.GetOutputModuleName()
	if !found {
		panic(fmt.Errorf("no output module provided in request"))
	}

	return outputModule
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

func ValidateRequest(req *Request, isSubRequest bool) error {
	seenStores := map[string]bool{}

	if req.StartBlockNum < 0 {
		// TODO(abourget): remove this check once we support StartBlockNum being negative
		return fmt.Errorf("negative start block %d is not accepted", req.StartBlockNum)
	}

	if req.Modules == nil {
		return fmt.Errorf("no modules found in request")
	}

	if err := validateOutputModule(req); err != nil {
		return fmt.Errorf("output module: %w", err)
	}

	if req.DebugInitialStoreSnapshotForModules != nil && req.ProductionMode {
		return fmt.Errorf("cannot set 'debug-modules-initial-snapshot' in 'production-mode'")
	}

	outputModule, found := req.GetOutputModuleName()
	if !found {
		return fmt.Errorf("no valid output module defined")
	}

	outputModuleFound := false
	for _, mod := range req.Modules.Modules {
		if _, ok := mod.Kind.(*Module_KindStore_); ok {
			seenStores[mod.Name] = true
		}
		if mod.Name == outputModule {
			if !isSubRequest {
				if _, ok := mod.Kind.(*Module_KindStore_); ok {
					return fmt.Errorf("output module must be of kind 'map'")
				}
			}
			outputModuleFound = true
		}
	}
	if !outputModuleFound {
		return fmt.Errorf("output module %q not found in modules", outputModule)
	}

	for _, storeSnapshot := range req.DebugInitialStoreSnapshotForModules {
		if !seenStores[storeSnapshot] {
			return fmt.Errorf("initial store snapshots for module: %q: no such 'store' module defined modules graph", storeSnapshot)
		}
	}
	return nil
}

func validateOutputModule(req *Request) error {
	if req.OutputModule != "" {
		return nil
	}
	outputCount := len(req.OutputModules)
	if outputCount == 0 {
		return fmt.Errorf("no output module found in request")
	}
	if outputCount > 1 {
		return fmt.Errorf("multiple output modules is not accepted")
	}
	return nil
}

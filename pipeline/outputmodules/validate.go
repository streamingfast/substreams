package outputmodules

import (
	"fmt"

	"github.com/streamingfast/substreams/manifest"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

// ValidateRequest is run by the server code.
func ValidateRequest(request *pbsubstreams.Request, blockType string, isSubRequest bool) error {
	if request.ProductionMode {
		if request.DebugInitialStoreSnapshotForModules != nil && len(request.DebugInitialStoreSnapshotForModules) > 0 {
			return fmt.Errorf("debug initial store snapshot feature is not supported in production mode")
		}
	}

	if err := manifest.ValidateModules(request.Modules); err != nil {
		return fmt.Errorf("modules validation failed: %w", err)
	}

	if err := pbsubstreams.ValidateRequest(request, isSubRequest); err != nil {
		return fmt.Errorf("validate request: %s", err)
	}

	for _, binary := range request.Modules.Binaries {
		if binary.Type != "wasm/rust-v1" {
			return fmt.Errorf(`unsupported binary type: %q, please use "wasm/rust-v1"`, binary.Type)
		}
	}

	graph, err := manifest.NewModuleGraph(request.Modules.Modules)
	if err != nil {
		return fmt.Errorf("should have been able to derive modules graph: %w", err)
	}

	// Already validated by `ValidateRequest` above, so we can use the `Must...` version
	outputModule := request.MustGetOutputModuleName()
	ancestors, err := graph.AncestorsOf(outputModule)
	if err != nil {
		return fmt.Errorf("computing ancestors of %q: %w", outputModule, err)
	}

	// We must only validate the input source against module that we are going to actually run. A Substreams
	// could provide modules for multiple chain while executing only one of them in which case only the one
	// run (and its dependencies transitively) should be checked.
	for _, mod := range ancestors {
		for _, input := range mod.Inputs {
			if src := input.GetSource(); src != nil {
				if src.Type != blockType && src.Type != "sf.substreams.v1.Clock" {
					return fmt.Errorf("input source %q not supported, only %q and 'sf.substreams.v1.Clock' are valid", src, blockType)
				}
			}
		}
	}
	return nil
}

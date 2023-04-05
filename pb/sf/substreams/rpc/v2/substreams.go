package pbsubstreamsrpc

import (
	"fmt"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

func (bd *BlockScopedData) AllMapModuleOutputs() []*MapModuleOutput {
	return append([]*MapModuleOutput{bd.Output}, bd.DebugMapOutputs...)
}

func (req *Request) Validate() error {
	seenStores := map[string]bool{}

	if req.StartBlockNum < 0 {
		// TODO(abourget): remove this check once we support StartBlockNum being negative
		return fmt.Errorf("negative start block %d is not accepted", req.StartBlockNum)
	}

	if req.Modules == nil {
		return fmt.Errorf("no modules found in request")
	}

	if req.OutputModule == "" {
		return fmt.Errorf("no output module defined in request")
	}

	if req.DebugInitialStoreSnapshotForModules != nil && req.ProductionMode {
		return fmt.Errorf("cannot set 'debug-modules-initial-snapshot' in 'production-mode'")
	}

	outputModuleFound := false
	for _, mod := range req.Modules.Modules {
		if _, ok := mod.Kind.(*pbsubstreams.Module_KindStore_); ok {
			seenStores[mod.Name] = true
		}
		if mod.Name == req.OutputModule {
			if _, ok := mod.Kind.(*pbsubstreams.Module_KindStore_); ok {
				return fmt.Errorf("output module must be of kind 'map'")
			}
			outputModuleFound = true
		}
	}
	if !outputModuleFound {
		return fmt.Errorf("output module %q not found in modules", req.OutputModule)
	}

	for _, storeSnapshot := range req.DebugInitialStoreSnapshotForModules {
		if !seenStores[storeSnapshot] {
			return fmt.Errorf("initial store snapshots for module: %q: no such 'store' module defined modules graph", storeSnapshot)
		}
	}
	return nil
}

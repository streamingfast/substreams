package pbsubstreamsrpc

import (
	"fmt"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

type AnyModuleOutput struct {
	MapOutput   *MapModuleOutput
	StoreOutput *StoreModuleOutput
}

func (a *AnyModuleOutput) IsMap() bool {
	return a.MapOutput != nil
}

func (a *AnyModuleOutput) IsStore() bool {
	return a.StoreOutput != nil
}

func (a *AnyModuleOutput) Name() string {
	if a.MapOutput != nil {
		return a.MapOutput.Name
	}
	return a.StoreOutput.Name
}

func (a *AnyModuleOutput) DebugInfo() *OutputDebugInfo {
	if a.MapOutput != nil {
		return a.MapOutput.DebugInfo
	}
	return a.StoreOutput.DebugInfo
}

func (a *AnyModuleOutput) IsEmpty() bool {
	if a.MapOutput != nil {
		return len(a.MapOutput.MapOutput.Value) == 0
	}
	return len(a.StoreOutput.DebugStoreDeltas) == 0
}

func (m *MapModuleOutput) ToAny() *AnyModuleOutput {
	return &AnyModuleOutput{
		MapOutput: m,
	}
}

func (s *StoreModuleOutput) ToAny() *AnyModuleOutput {
	return &AnyModuleOutput{
		StoreOutput: s,
	}
}

func (bd *BlockScopedData) AllModuleOutputs() (out []*AnyModuleOutput) {
	out = append(out, bd.Output.ToAny())
	for _, mapOut := range bd.DebugMapOutputs {
		out = append(out, mapOut.ToAny())
	}
	for _, storeOut := range bd.DebugStoreOutputs {
		out = append(out, storeOut.ToAny())
	}
	return
}

func (req *Request) Validate() error {
	seenStores := map[string]bool{}

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

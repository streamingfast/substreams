package pbsubstreamsrpcv2

import (
	"fmt"
	"os"
	"strconv"

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

	// Validate estimate_mode constraints
	if req.EstimateMode {
		// Mutual exclusivity with noop_mode
		if req.NoopMode {
			return fmt.Errorf("estimate_mode and noop_mode are mutually exclusive")
		}

		// Production mode must be disabled
		if req.ProductionMode {
			return fmt.Errorf("estimate_mode requires production_mode to be false")
		}

		// Block range validation when estimate_mode is enabled
		if req.StopBlockNum > 0 {
			blockRange := req.StopBlockNum - uint64(req.StartBlockNum)
			maxBlockRange := getEstimateModeMaxBlockRange()
			if blockRange > maxBlockRange {
				return fmt.Errorf("estimate_mode block range (%d) exceeds maximum allowed range (%d)", blockRange, maxBlockRange)
			}
		}
	}

	return nil
}

// getEstimateModeMaxBlockRange returns the maximum block range allowed for estimate_mode
// from the SUBSTREAMS_ESTIMATE_MODE_MAX_BLOCK_RANGE environment variable, defaulting to 1000
func getEstimateModeMaxBlockRange() uint64 {
	envVar := os.Getenv("SUBSTREAMS_ESTIMATE_MODE_MAX_BLOCK_RANGE")
	if envVar == "" {
		return 1000 // Default value
	}

	value, err := strconv.ParseUint(envVar, 10, 64)
	if err != nil {
		return 1000 // Default value on parse error
	}

	return value
}

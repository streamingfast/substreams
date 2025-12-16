package pbsubstreamsrpcv3

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/streamingfast/substreams/manifest"
	pbsubstreamsrpcv2 "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

func (r *Request) ToV2() (*pbsubstreamsrpcv2.Request, error) {

	_, err := manifest.ApplyPackageTransformations(r.Package, true, r.Network, r.OutputModule, r.Params)
	if err != nil {
		return nil, err
	}

	return &pbsubstreamsrpcv2.Request{
		StartBlockNum:                       r.StartBlockNum,
		StartCursor:                         r.StartCursor,
		StopBlockNum:                        r.StopBlockNum,
		FinalBlocksOnly:                     r.FinalBlocksOnly,
		ProductionMode:                      r.ProductionMode,
		OutputModule:                        r.OutputModule,
		Modules:                             r.Package.Modules,
		DebugInitialStoreSnapshotForModules: r.DebugInitialStoreSnapshotForModules,
		NoopMode:                            r.NoopMode,
		LimitProcessedBlocks:                r.LimitProcessedBlocks,
		DevOutputModules:                    r.DevOutputModules,
		ProgressMessagesIntervalMs:          r.ProgressMessagesIntervalMs,
		IncludePartialBlocks:                r.IncludePartialBlocks,
		PartialBlocksOnly:                   r.PartialBlocksOnly,
	}, nil
}

func ParamsToMap(in []string) (map[string]string, error) {
	if len(in) == 0 {
		return nil, nil
	}
	out := make(map[string]string)
	for _, p := range in {
		kv := strings.SplitN(p, "=", 2)
		if len(kv) != 2 {
			return nil, fmt.Errorf("invalid parameter format: %s", p)
		}

		out[kv[0]] = kv[1]
	}
	return out, nil
}

func (req *Request) Validate() error {
	seenStores := map[string]bool{}

	if req.Package == nil {
		return fmt.Errorf("no package found in request")
	}

	if req.Package.Modules == nil {
		return fmt.Errorf("no modules found in request")
	}

	if req.OutputModule == "" {
		return fmt.Errorf("no output module defined in request")
	}

	if req.DebugInitialStoreSnapshotForModules != nil && req.ProductionMode {
		return fmt.Errorf("cannot set 'debug-modules-initial-snapshot' in 'production-mode'")
	}

	outputModuleFound := false
	for _, mod := range req.Package.Modules.Modules {
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

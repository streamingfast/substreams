package pbsubstreamsrpcv3

import (
	"fmt"
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
	return nil
}

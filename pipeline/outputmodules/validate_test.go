package outputmodules

import (
	"fmt"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/stretchr/testify/require"
	"testing"
)

func Test_ValidateRequest(t *testing.T) {
	tests := []struct {
		name       string
		request    *pbsubstreams.Request
		subrequest bool
		blockType  string
		expect     error
	}{
		{"negative start block num", req(-1), false, "sf.substreams.v1.test.Block", fmt.Errorf("negative start block -1 is not accepted")},
		{"no modules found in request", &pbsubstreams.Request{StartBlockNum: 1}, false, "sf.substreams.v1.test.Block", fmt.Errorf("no modules found in request")},
		{"multiple output modules is not accepted", req(1, withOutputModules([][]string{{"output_mod_1", "store"}, {"output_mod_1", "kind"}}, true)), false, "sf.substreams.v1.test.Block", fmt.Errorf("multiple output modules is not accepted")},
		{"single legacy map output module is accepted for none sub-request", req(1, withOutputModule("output_mod", "map", true)), false, "sf.substreams.v1.test.Block", nil},
		{"single legacy store output module is not accepted for none sub-request", req(1, withOutputModule("output_mod", "store", true)), false, "sf.substreams.v1.test.Block", fmt.Errorf("multiple output modules is not accepted")},
		{"single legacy map output module is accepted for sub-request", req(1, withOutputModule("output_mod", "map", true)), true, "sf.substreams.v1.test.Block", nil},
		{"single legacy store output module is accepted for sub-request", req(1, withOutputModule("output_mod", "store", true)), true, "sf.substreams.v1.test.Block", nil},
		{"single map output module is accepted for none sub-request", req(1, withOutputModule("output_mod", "map", false)), false, "sf.substreams.v1.test.Block", nil},
		{"single store output module is not accepted for none sub-request", req(1, withOutputModule("output_mod", "store", false)), false, "sf.substreams.v1.test.Block", fmt.Errorf("multiple output modules is not accepted")},
		{"single map output module is accepted for none sub-request", req(1, withOutputModule("output_mod", "map", false)), true, "sf.substreams.v1.test.Block", nil},
		{"single store output module is  accepted for none sub-request", req(1, withOutputModule("output_mod", "map", false)), true, "sf.substreams.v1.test.Block", nil},
		{name: "debug initial snapshots not accepted in production mode", request: req(1, withDebugInitialSnapshotForModules([]string{"foo"}), withProductionMode()), expect: fmt.Errorf("debug initial snapshots not accepted in production mode")},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := ValidateRequest(test.request, test.blockType, test.subrequest)
			if test.expect != nil {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func withOutputModules(modules [][]string, legacy bool) reqOption {
	return func(req *pbsubstreams.Request) *pbsubstreams.Request {
		for _, module := range modules {
			addOutputModule(req, module[0], module[1], legacy)
		}
		return req
	}
}

type reqOption func(*pbsubstreams.Request) *pbsubstreams.Request

func withOutputModule(outputModule, kind string, legacy bool) reqOption {
	return func(req *pbsubstreams.Request) *pbsubstreams.Request {
		addOutputModule(req, outputModule, kind, legacy)
		return req
	}
}

func withProductionMode() reqOption {
	return func(req *pbsubstreams.Request) *pbsubstreams.Request {
		req.ProductionMode = true
		return req
	}
}

func withDebugInitialSnapshotForModules(modules []string) reqOption {
	return func(req *pbsubstreams.Request) *pbsubstreams.Request {
		req.DebugInitialStoreSnapshotForModules = modules
		return req
	}
}

func req(startBlockNum int64, opts ...reqOption) *pbsubstreams.Request {
	r := &pbsubstreams.Request{
		StartBlockNum: startBlockNum,
		Modules:       &pbsubstreams.Modules{},
	}
	for _, opt := range opts {
		r = opt(r)
	}
	return r
}

func addOutputModule(req *pbsubstreams.Request, outputModule, kind string, legacy bool) {
	module := &pbsubstreams.Module{
		Name: outputModule,
		Kind: nil,
	}
	if kind == "store" {
		module.Kind = &pbsubstreams.Module_KindStore_{}
	} else {
		module.Kind = &pbsubstreams.Module_KindMap_{}
	}
	req.Modules.Modules = append(req.Modules.Modules, module)
	if legacy {
		req.OutputModules = append(req.OutputModules, outputModule)
	} else {
		req.OutputModule = outputModule
	}

}

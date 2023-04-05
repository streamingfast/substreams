package outputmodules

import (
	"fmt"
	"testing"

	pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/stretchr/testify/require"
)

func Test_ValidateRequest(t *testing.T) {
	tests := []struct {
		name       string
		request    *pbsubstreamsrpc.Request
		subrequest bool
		blockType  string
		expect     error
	}{
		{"negative start block num", req(-1), false, "sf.substreams.v1.test.Block", fmt.Errorf("negative start block -1 is not accepted")},
		{"no modules found in request", &pbsubstreamsrpc.Request{StartBlockNum: 1}, false, "sf.substreams.v1.test.Block", fmt.Errorf("no modules found in request")},
		{"single map output module is accepted for none sub-request", req(1, withOutputModule("output_mod", "map")), false, "sf.substreams.v1.test.Block", nil},
		{"single store output module is not accepted for none sub-request", req(1, withOutputModule("output_mod", "store")), false, "sf.substreams.v1.test.Block", fmt.Errorf("multiple output modules is not accepted")},
		{"single map output module is accepted for none sub-request", req(1, withOutputModule("output_mod", "map")), true, "sf.substreams.v1.test.Block", nil},
		{"single store output module is  accepted for none sub-request", req(1, withOutputModule("output_mod", "map")), true, "sf.substreams.v1.test.Block", nil},
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

type reqOption func(*pbsubstreamsrpc.Request) *pbsubstreamsrpc.Request

func withOutputModule(outputModule, kind string) reqOption {
	return func(req *pbsubstreamsrpc.Request) *pbsubstreamsrpc.Request {
		addOutputModule(req, outputModule, kind)
		return req
	}
}

func withProductionMode() reqOption {
	return func(req *pbsubstreamsrpc.Request) *pbsubstreamsrpc.Request {
		req.ProductionMode = true
		return req
	}
}

func withDebugInitialSnapshotForModules(modules []string) reqOption {
	return func(req *pbsubstreamsrpc.Request) *pbsubstreamsrpc.Request {
		req.DebugInitialStoreSnapshotForModules = modules
		return req
	}
}

func req(startBlockNum int64, opts ...reqOption) *pbsubstreamsrpc.Request {
	r := &pbsubstreamsrpc.Request{
		StartBlockNum: startBlockNum,
		Modules:       &pbsubstreams.Modules{},
	}
	for _, opt := range opts {
		r = opt(r)
	}
	return r
}

func addOutputModule(req *pbsubstreamsrpc.Request, outputModule, kind string) {
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
	req.OutputModule = outputModule

}

package service

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	pbssinternal "github.com/streamingfast/substreams/pb/sf/substreams/intern/v2"
	pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

func Test_ValidateRequest(t *testing.T) {
	testOutputMap := withOutputModule("output_mod", "map")
	testOutputStore := withOutputModule("output_mod", "store")

	testBlockType := "sf.substreams.v1.test.Block"

	tests := []struct {
		name      string
		request   *pbsubstreamsrpc.Request
		blockType string
		expect    error
	}{
		{"negative start block num", req(-1, testOutputMap), testBlockType, nil},
		{"no modules found in request", &pbsubstreamsrpc.Request{StartBlockNum: 1}, testBlockType, fmt.Errorf("validate tier1 request: no modules found in request")},
		{"single legacy map output module is accepted for none sub-request", req(1, testOutputMap), testBlockType, nil},
		{"single map output module is accepted for none sub-request", req(1, testOutputMap), testBlockType, nil},
		{"single store output module is not accepted for none sub-request", req(1, testOutputStore), testBlockType, fmt.Errorf("validate tier1 request: output module must be of kind 'map'")},
		{"debug initial snapshots not accepted in production mode", req(1, testOutputMap, withDebugInitialSnapshotForModules([]string{"foo"}), withProductionMode()), "", fmt.Errorf(`validate tier1 request: cannot set 'debug-modules-initial-snapshot' in 'production-mode'`)},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := ValidateTier1Request(test.request, test.blockType)
			if test.expect != nil {
				require.EqualError(t, err, test.expect.Error())
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func Test_ValidateTier2Request(t *testing.T) {

	//testStoreModule := &pbsubstreams.Module{
	//	Name: "test",
	//	Kind: &pbsubstreams.Module_KindStore_{},
	//}
	testMapModule := &pbsubstreams.Module{
		Name: "test",
		Kind: &pbsubstreams.Module_KindMap_{},
	}

	//	testOutputMap := withOutputModule("output_mod", "map")
	//	testOutputStore := withOutputModule("output_mod", "store")

	getPerfectRequest := func() *pbssinternal.ProcessRangeRequest {
		return &pbssinternal.ProcessRangeRequest{
			Modules:              &pbsubstreams.Modules{Modules: []*pbsubstreams.Module{testMapModule}},
			OutputModule:         "test",
			Stage:                0,
			SegmentNumber:        10,
			SegmentSize:          10,
			FirstStreamableBlock: 0,
			MeteringConfig:       "metering",
			BlockType:            "block",
			StateStore:           "/tmp/state",
			MergedBlocksStore:    "/tmp/merged",
		}
	}

	tests := []struct {
		name     string
		tweakReq func(*pbssinternal.ProcessRangeRequest)
		expect   error
	}{
		{"negative start block num", func(req *pbssinternal.ProcessRangeRequest) { req.Modules = nil },
			fmt.Errorf("validate tier2 request: no modules found in request")},
		{"completely below first streamable block", func(req *pbssinternal.ProcessRangeRequest) { req.FirstStreamableBlock = 110 },
			fmt.Errorf("validate tier2 request: segment is completely below the first streamable block")},
		{"half below first streamable block", func(req *pbssinternal.ProcessRangeRequest) { req.FirstStreamableBlock = 109 },
			nil},
		{"old 'stopBlockNum' is set", func(req *pbssinternal.ProcessRangeRequest) { req.StopBlockNum = 123 },
			fmt.Errorf("validate tier2 request: invalid protocol: update your tier1")},
		{"no output module", func(req *pbssinternal.ProcessRangeRequest) { req.OutputModule = "" },
			fmt.Errorf("validate tier2 request: no output module defined in request")},
		{"no metering config", func(req *pbssinternal.ProcessRangeRequest) { req.MeteringConfig = "" },
			fmt.Errorf("validate tier2 request: metering config is required in request")},
		{"no blocktype", func(req *pbssinternal.ProcessRangeRequest) { req.BlockType = "" },
			fmt.Errorf("validate tier2 request: block type is required in request")},
		{"working", func(req *pbssinternal.ProcessRangeRequest) {},
			nil},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			req := getPerfectRequest()
			test.tweakReq(req)
			err := ValidateTier2Request(req)
			if test.expect != nil {
				require.EqualError(t, err, test.expect.Error())
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

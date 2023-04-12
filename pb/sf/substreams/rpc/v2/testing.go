package pbsubstreamsrpc

import (
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

func TestNewRequest(startBlockNum int64, opts ...testNewRequestOption) *Request {
	r := &Request{
		StartBlockNum: startBlockNum,
		Modules:       &pbsubstreams.Modules{},
	}
	for _, opt := range opts {
		r = opt(r)
	}
	return r
}

func withProductionMode() testNewRequestOption {
	return func(req *Request) *Request {
		req.ProductionMode = true
		return req
	}
}

func withDebugSnapshotsModule(mod string) testNewRequestOption {
	return func(req *Request) *Request {
		req.DebugInitialStoreSnapshotForModules = append(req.DebugInitialStoreSnapshotForModules, mod)
		return req
	}
}

func withTestOutputModule(module string) testNewRequestOption {
	return func(req *Request) *Request {
		req.OutputModule = module
		return req
	}
}

func withTestStoreModule(name string) testNewRequestOption {
	return func(req *Request) *Request {
		req.Modules.Modules = append(req.Modules.Modules, TestNewStoreModule(name))
		return req
	}
}

func withTestMapModule(name string) testNewRequestOption {
	return func(req *Request) *Request {
		req.Modules.Modules = append(req.Modules.Modules, TestNewMapModule(name))
		return req
	}
}

type testNewRequestOption func(*Request) *Request

func TestNewStoreModule(name string) *pbsubstreams.Module {
	return &pbsubstreams.Module{
		Name: name,
		Kind: &pbsubstreams.Module_KindStore_{
			KindStore: &pbsubstreams.Module_KindStore{
				UpdatePolicy: pbsubstreams.Module_KindStore_UPDATE_POLICY_SET,
				ValueType:    "string",
			},
		},
	}
}

func TestNewMapModule(name string) *pbsubstreams.Module {
	return &pbsubstreams.Module{
		Name: name,
		Kind: &pbsubstreams.Module_KindMap_{
			KindMap: &pbsubstreams.Module_KindMap{
				OutputType: "string",
			},
		},
	}

}

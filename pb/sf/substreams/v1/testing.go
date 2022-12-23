package pbsubstreams

func TestNewRequest(startBlockNum int64, opts ...testNewRequestOption) *Request {
	r := &Request{
		StartBlockNum: startBlockNum,
		Modules:       &Modules{},
	}
	for _, opt := range opts {
		r = opt(r)
	}
	return r
}

func withTestOutputModules(modules [][]string, legacy bool) testNewRequestOption {
	return func(req *Request) *Request {
		for _, module := range modules {
			addTestOutputModule(req, module[0], module[1], legacy)
		}
		return req
	}
}

func withTestOutputModule(outputModule, kind string, legacy bool) testNewRequestOption {
	return func(req *Request) *Request {
		addTestOutputModule(req, outputModule, kind, legacy)
		return req
	}
}

type testNewRequestOption func(*Request) *Request

func addTestOutputModule(req *Request, outputModule, kind string, legacy bool) {
	var module *Module
	if kind == "store" {
		module = TestNewStoreModule(outputModule)
	} else {
		module = TestNewMapModule(outputModule)
	}
	req.Modules.Modules = append(req.Modules.Modules, module)
	if legacy {
		req.OutputModules = append(req.OutputModules, outputModule)
	} else {
		req.OutputModule = outputModule
	}
}

func TestNewStoreModule(name string) *Module {
	return &Module{
		Name: name,
		Kind: &Module_KindStore_{
			KindStore: &Module_KindStore{
				UpdatePolicy: Module_KindStore_UPDATE_POLICY_SET,
				ValueType:    "string",
			},
		},
	}
}

func TestNewMapModule(name string) *Module {
	return &Module{
		Name: name,
		Kind: &Module_KindMap_{
			KindMap: &Module_KindMap{
				OutputType: "string",
			},
		},
	}

}

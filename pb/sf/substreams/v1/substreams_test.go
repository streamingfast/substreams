package pbsubstreams

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_ValidateRequest(t *testing.T) {
	tests := []struct {
		name         string
		request      *Request
		isSubrequest bool
		expect       error
	}{
		{"negative start block num", req(-1), false, fmt.Errorf("negative start block -1 is not accepted")},
		{"no modules found in request", &Request{StartBlockNum: 1}, false, fmt.Errorf("no modules found in request")},
		{"multiple output modules is not accepted", req(1, withOutputModules([][]string{{"output_mod_1", "store"}, {"output_mod_1", "kind"}}, true)), false, fmt.Errorf("multiple output modules is not accepted")},
		{"store output module is accepted for sub-request", req(1, withOutputModule("output_mod_1", "store", false)), true, nil},
		{"production mode should fail with debug flag", req(1), false, fmt.Errorf("to fill")},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := ValidateRequest(test.request, test.isSubrequest)
			if test.expect != nil {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func withOutputModules(modules [][]string, legacy bool) reqOption {
	return func(req *Request) *Request {
		for _, module := range modules {
			addOutputModule(req, module[0], module[1], legacy)
		}
		return req
	}
}

func withOutputModule(outputModule, kind string, legacy bool) reqOption {
	return func(req *Request) *Request {
		addOutputModule(req, outputModule, kind, legacy)
		return req
	}
}

type reqOption func(*Request) *Request

func req(startBlockNum int64, opts ...reqOption) *Request {
	r := &Request{
		StartBlockNum: startBlockNum,
		Modules:       &Modules{},
	}
	for _, opt := range opts {
		r = opt(r)
	}
	return r
}

func addOutputModule(req *Request, outputModule, kind string, legacy bool) {
	module := &Module{
		Name: outputModule,
		Kind: nil,
	}
	if kind == "store" {
		module.Kind = &Module_KindStore_{}
	} else {
		module.Kind = &Module_KindMap_{}
	}
	req.Modules.Modules = append(req.Modules.Modules, module)
	if legacy {
		req.OutputModules = append(req.OutputModules, outputModule)
	} else {
		req.OutputModule = outputModule
	}
}

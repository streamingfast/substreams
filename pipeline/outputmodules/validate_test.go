package outputmodules

import (
	"fmt"
	"testing"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/stretchr/testify/require"
)

func Test_ValidateRequest(t *testing.T) {
	testOutputMap := withOutputModule("output_mod", "map", false)
	testOutputStore := withOutputModule("output_mod", "store", false)
	testOutputMapLegacy := withOutputModule("output_mod", "map", true)
	testOutputStoreLegacy := withOutputModule("output_mod", "store", true)

	testBlockType := "sf.substreams.v1.test.Block"

	stepNew := pbsubstreams.ForkStep_STEP_NEW
	stepUndo := pbsubstreams.ForkStep_STEP_UNDO
	stepIrreversible := pbsubstreams.ForkStep_STEP_IRREVERSIBLE
	stepUnknown := pbsubstreams.ForkStep_STEP_UNKNOWN

	tests := []struct {
		name       string
		request    *pbsubstreams.Request
		subrequest bool
		blockType  string
		expect     error
	}{
		{"negative start block num", req(-1, testOutputMap), false, testBlockType, nil},
		{"no modules found in request", &pbsubstreams.Request{StartBlockNum: 1}, false, testBlockType, fmt.Errorf("modules validation failed: no modules found in request")},
		{"multiple output modules is not accepted", req(1, withOutputModules([][]string{{"output_mod_1", "store"}, {"output_mod_1", "kind"}}, true)), false, testBlockType, fmt.Errorf("validate request: output module: multiple output modules is not accepted")},
		{"single legacy map output module is accepted for none sub-request", req(1, testOutputMapLegacy), false, testBlockType, nil},
		{"single legacy store output module is not accepted for none sub-request", req(1, testOutputStoreLegacy), false, testBlockType, fmt.Errorf("validate request: output module must be of kind 'map'")},
		{"single legacy map output module is accepted for sub-request", req(1, testOutputMapLegacy), true, testBlockType, nil},
		{"single legacy store output module is accepted for sub-request", req(1, testOutputStoreLegacy), true, testBlockType, nil},
		{"single map output module is accepted for none sub-request", req(1, testOutputMap), false, testBlockType, nil},
		{"single store output module is not accepted for none sub-request", req(1, testOutputStore), false, testBlockType, fmt.Errorf("validate request: output module must be of kind 'map'")},
		{"single map output module is accepted for none sub-request", req(1, testOutputMap), true, testBlockType, nil},
		{"single store output module is  accepted for none sub-request", req(1, testOutputMap), true, testBlockType, nil},
		{"debug initial snapshots not accepted in production mode", req(1, withDebugInitialSnapshotForModules([]string{"foo"}), withProductionMode()), false, "", fmt.Errorf("debug initial store snapshot feature is not supported in production mode")},

		{"step empty rejected", req(1, testOutputMap, withSteps()), false, testBlockType, fmt.Errorf(`validate request: invalid "fork_steps": cannot be empty`)},

		{"step undo rejected", req(1, testOutputMap, withSteps(stepUndo)), false, testBlockType, fmt.Errorf(`validate request: invalid "fork_steps": step "STEP_UNDO" cannot be specified alone`)},
		{"step unknown rejected", req(1, testOutputMap, withSteps(stepUnknown)), false, testBlockType, fmt.Errorf(`validate request: invalid "fork_steps": step "STEP_UNKNOWN" cannot be specified alone`)},

		{"step only new/undo accepted", req(1, testOutputMap, withSteps(stepNew, stepUndo)), false, testBlockType, nil},
		{"step only undo/new accepted", req(1, testOutputMap, withSteps(stepUndo, stepNew)), false, testBlockType, nil},
		{"step only irreversible accepted", req(1, testOutputMap, withSteps(stepIrreversible)), false, testBlockType, nil},

		{"step new/unknown rejected", req(1, testOutputMap, withSteps(stepNew, stepUnknown)), false, testBlockType, fmt.Errorf(`validate request: invalid "fork_steps": step "STEP_NEW" and step "STEP_UNKNOWN" cannot be provided together accepting "STEP_NEW" and "STEP_UNDO" only`)},
		{"step new/irreversible rejected", req(1, testOutputMap, withSteps(stepNew, stepIrreversible)), false, testBlockType, fmt.Errorf(`validate request: invalid "fork_steps": step "STEP_NEW" and step "STEP_IRREVERSIBLE" cannot be provided together accepting "STEP_NEW" and "STEP_UNDO" only`)},
		{"step irreversible/undo rejected", req(1, testOutputMap, withSteps(stepIrreversible, stepUndo)), false, testBlockType, fmt.Errorf(`validate request: invalid "fork_steps": step "STEP_IRREVERSIBLE" and step "STEP_UNDO" cannot be provided together accepting "STEP_NEW" and "STEP_UNDO" only`)},

		{"step new/undo/irreversible rejected", req(1, testOutputMap, withSteps(stepNew, stepUndo, stepIrreversible)), false, testBlockType, fmt.Errorf(`validate request: invalid "fork_steps": accepting only 1 or 2 steps but there was 3 steps provided`)},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := ValidateRequest(test.request, test.blockType, test.subrequest)
			if test.expect != nil {
				require.EqualError(t, err, test.expect.Error())
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

func withSteps(steps ...pbsubstreams.ForkStep) reqOption {
	return func(req *pbsubstreams.Request) *pbsubstreams.Request {
		req.ForkSteps = steps
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
		ForkSteps:     []pbsubstreams.ForkStep{pbsubstreams.ForkStep_STEP_NEW, pbsubstreams.ForkStep_STEP_UNDO},
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

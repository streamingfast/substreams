package pipeline

import (
	"testing"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/stretchr/testify/require"
)

var reversibleOutputs = map[string][]*pbsubstreams.ModuleOutput{
	"10a": {
		{Name: "module_1"}, {Name: "module_2"}, {Name: "module_3"},
	},
	"20a": {
		{Name: "module_1"}, {Name: "module_2"}, {Name: "module_3"},
	},
	"30a": {
		{Name: "module_1"}, {Name: "module_2"}, {Name: "module_3"},
	},
	"40a": {
		{Name: "module_1"}, {Name: "module_2"}, {Name: "module_3"},
	},
	"50a": {
		{Name: "module_1"}, {Name: "module_2"}, {Name: "module_3"},
	},
}

var reversibleModules = map[string][]*pbsubstreams.Module{
	"10": {
		{Name: "module_1"}, {Name: "module_2"}, {Name: "module_3"},
	},
	"20": {
		{Name: "module_1"}, {Name: "module_2"}, {Name: "module_3"},
	},
	"30": {
		{Name: "module_1"}, {Name: "module_2"}, {Name: "module_3"},
	},
	"40": {
		{Name: "module_1"}, {Name: "module_2"}, {Name: "module_3"},
	},
	"50": {
		{Name: "module_1"}, {Name: "module_2"}, {Name: "module_3"},
	},
}

func Test_HandleIrreversibility(t *testing.T) {

	tests := []struct {
		name              string
		reversibleOutputs map[string][]*pbsubstreams.Module
		blockIDs          []string
		expectedOutputs   map[string][]*pbsubstreams.ModuleOutput
	}{
		{
			name:              "handle irreversibility for block 20",
			reversibleOutputs: reversibleModules,
			blockIDs:          []string{"20a"},
			expectedOutputs: map[string][]*pbsubstreams.ModuleOutput{
				"10a": {
					{Name: "module_1"}, {Name: "module_2"}, {Name: "module_3"},
				},
				"30a": {
					{Name: "module_1"}, {Name: "module_2"}, {Name: "module_3"},
				},
				"40a": {
					{Name: "module_1"}, {Name: "module_2"}, {Name: "module_3"},
				},
				"50a": {
					{Name: "module_1"}, {Name: "module_2"}, {Name: "module_3"},
				},
			},
		},
		{
			name:              "handle irreversibility for block 20 and 30",
			reversibleOutputs: reversibleModules,
			blockIDs:          []string{"20a", "30a"},
			expectedOutputs: map[string][]*pbsubstreams.ModuleOutput{
				"10a": {
					{Name: "module_1"}, {Name: "module_2"}, {Name: "module_3"},
				},
				"40a": {
					{Name: "module_1"}, {Name: "module_2"}, {Name: "module_3"},
				},
				"50a": {
					{Name: "module_1"}, {Name: "module_2"}, {Name: "module_3"},
				},
			},
		},
		{
			name:              "handle irreversibility for block 20, 30, 40 and 50",
			reversibleOutputs: reversibleModules,
			blockIDs:          []string{"20a", "30a", "40a", "50a"},
			expectedOutputs: map[string][]*pbsubstreams.ModuleOutput{
				"10a": {
					{Name: "module_1"}, {Name: "module_2"}, {Name: "module_3"},
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			forkHandler := &ForkHandler{
				reversibleOutputs: reversibleOutputs,
			}
			for _, id := range test.blockIDs {
				forkHandler.removeReversibleOutput(id)
			}
			require.Equal(t, test.expectedOutputs, forkHandler.reversibleOutputs)
		})
	}
}

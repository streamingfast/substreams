package exec

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

func firstStreamableTestModules() *pbsubstreams.Modules {
	return &pbsubstreams.Modules{
		Binaries: []*pbsubstreams.Binary{{Type: "wasm/rust-v1"}},
		Modules: []*pbsubstreams.Module{
			{
				Name:         "map_below",
				InitialBlock: 100,
				Kind:         &pbsubstreams.Module_KindMap_{KindMap: &pbsubstreams.Module_KindMap{OutputType: "proto:test.Output"}},
				Inputs: []*pbsubstreams.Module_Input{
					{Input: &pbsubstreams.Module_Input_Source_{Source: &pbsubstreams.Module_Input_Source{Type: "sf.substreams.v1.test.Block"}}},
				},
				Output: &pbsubstreams.Module_Output{Type: "proto:test.Output"},
			},
			{
				Name:         "map_zero",
				InitialBlock: 0,
				Kind:         &pbsubstreams.Module_KindMap_{KindMap: &pbsubstreams.Module_KindMap{OutputType: "proto:test.Output"}},
				Inputs: []*pbsubstreams.Module_Input{
					{Input: &pbsubstreams.Module_Input_Map_{Map: &pbsubstreams.Module_Input_Map{ModuleName: "map_below"}}},
				},
				Output: &pbsubstreams.Module_Output{Type: "proto:test.Output"},
			},
		},
	}
}

func TestGraph_firstStreamableBlockValidation(t *testing.T) {
	_, err := NewOutputModuleGraph("map_zero", true, firstStreamableTestModules(), 1000)
	require.Error(t, err)
	assert.Contains(t, err.Error(), `module "map_below" has initial block 100 smaller than first streamable block 1000`)
}

func TestGraph_relaxedFirstStreamableBlock(t *testing.T) {
	graph, err := NewOutputModuleGraph("map_zero", true, firstStreamableTestModules(), 1000, WithRelaxedFirstStreamableBlock())
	require.NoError(t, err)

	assert.Equal(t, uint64(1000), graph.ModulesInitBlocks()["map_below"])
	assert.Equal(t, uint64(1000), graph.ModulesInitBlocks()["map_zero"])
	assert.Equal(t, uint64(1000), graph.LowestInitBlock())
}

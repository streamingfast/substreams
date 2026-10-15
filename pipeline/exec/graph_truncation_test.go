package exec

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

func truncationTestModules() *pbsubstreams.Modules {
	mapKind := &pbsubstreams.Module_KindMap_{KindMap: &pbsubstreams.Module_KindMap{OutputType: "proto:test"}}
	return &pbsubstreams.Modules{
		Binaries: []*pbsubstreams.Binary{{Type: "test", Content: []byte("binary")}},
		Modules: []*pbsubstreams.Module{
			{
				Name:         "map_a",
				Kind:         mapKind,
				InitialBlock: 346,
				Inputs:       []*pbsubstreams.Module_Input{{Input: &pbsubstreams.Module_Input_Source_{Source: &pbsubstreams.Module_Input_Source{Type: "block"}}}},
				Output:       &pbsubstreams.Module_Output{Type: "proto:test"},
			},
			{
				Name:         "store_a",
				Kind:         &pbsubstreams.Module_KindStore_{KindStore: &pbsubstreams.Module_KindStore{UpdatePolicy: pbsubstreams.Module_KindStore_UPDATE_POLICY_SET, ValueType: "string"}},
				InitialBlock: 346,
				Inputs:       []*pbsubstreams.Module_Input{{Input: &pbsubstreams.Module_Input_Map_{Map: &pbsubstreams.Module_Input_Map{ModuleName: "map_a"}}}},
			},
			{
				Name:         "map_out",
				Kind:         mapKind,
				InitialBlock: 346,
				Inputs:       []*pbsubstreams.Module_Input{{Input: &pbsubstreams.Module_Input_Store_{Store: &pbsubstreams.Module_Input_Store{ModuleName: "store_a", Mode: pbsubstreams.Module_Input_Store_DELTAS}}}},
				Output:       &pbsubstreams.Module_Output{Type: "proto:test"},
			},
		},
	}
}

func TestNewOutputModuleGraph_Truncation(t *testing.T) {
	_, err := NewOutputModuleGraph("map_out", true, truncationTestModules(), 400, false)
	require.ErrorContains(t, err, "smaller than first streamable block 400")

	untruncated, err := NewOutputModuleGraph("map_out", true, truncationTestModules(), 0, false)
	require.NoError(t, err)

	graph, err := NewOutputModuleGraph("map_out", true, truncationTestModules(), 400, true)
	require.NoError(t, err)

	assert.Equal(t, map[string]uint64{"map_a": 400, "store_a": 400, "map_out": 400}, graph.ModulesInitBlocks())
	assert.Equal(t, uint64(400), graph.LowestInitBlock())
	require.NotNil(t, graph.LowestStoresInitBlock())
	assert.Equal(t, uint64(400), *graph.LowestStoresInitBlock())
	assert.Equal(t, untruncated.ModuleHashes(), graph.ModuleHashes(), "truncation must not change module hashes, or previous snapshots would not be found")
}

func TestNewOutputModuleGraph_TruncationStoreWithoutInitialBlock(t *testing.T) {
	modules := truncationTestModules()
	for _, mod := range modules.Modules {
		mod.InitialBlock = 0
	}

	_, err := NewOutputModuleGraph("map_out", true, modules, 400, false)
	require.NoError(t, err)

	_, err = NewOutputModuleGraph("map_out", true, modules, 400, true)
	require.ErrorContains(t, err, `store "store_a" has no initial block, which this endpoint refuses because it truncates the chain's tail (blocks below 400 are gone, and that block can be raised later): set an initial block at or above 400 on the store`)

	modules.Modules[1].InitialBlock = 450
	_, err = NewOutputModuleGraph("map_out", true, modules, 400, true)
	require.NoError(t, err, "maps without initial block hold no state")
}

package manifest

import (
	"sort"
	"testing"

	"github.com/streamingfast/bstream"
	"github.com/test-go/testify/require"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/stretchr/testify/assert"
)

var ten = uint64(10)
var twenty = uint64(20)
var thirty = uint64(30)

var testModules = []*pbsubstreams.Module{
	{
		Name: "A",
	},
	{
		Name:       "B",
		StartBlock: ten,
		Kind:       &pbsubstreams.Module_KindStore_{KindStore: &pbsubstreams.Module_KindStore{}},
		Inputs: []*pbsubstreams.Module_Input{
			{
				Input: &pbsubstreams.Module_Input_Map_{Map: &pbsubstreams.Module_Input_Map{
					ModuleName: "A",
				}},
			},
		},
	},
	{
		Name:       "C",
		StartBlock: twenty,
		Kind:       &pbsubstreams.Module_KindMap_{KindMap: &pbsubstreams.Module_KindMap{}},
		Inputs: []*pbsubstreams.Module_Input{
			{
				Input: &pbsubstreams.Module_Input_Store_{Store: &pbsubstreams.Module_Input_Store{
					ModuleName: "A",
				}},
			},
		},
	},
	{
		Name: "D",
		Kind: &pbsubstreams.Module_KindMap_{KindMap: &pbsubstreams.Module_KindMap{}},
		Inputs: []*pbsubstreams.Module_Input{
			{
				Input: &pbsubstreams.Module_Input_Store_{Store: &pbsubstreams.Module_Input_Store{
					ModuleName: "B",
				}},
			},
		},
	},
	{
		Name:       "E",
		StartBlock: ten,
		Kind:       &pbsubstreams.Module_KindStore_{KindStore: &pbsubstreams.Module_KindStore{}},
		Inputs: []*pbsubstreams.Module_Input{
			{
				Input: &pbsubstreams.Module_Input_Map_{Map: &pbsubstreams.Module_Input_Map{
					ModuleName: "C",
				}},
			},
		},
	},
	{
		Name: "F",
		Kind: &pbsubstreams.Module_KindStore_{KindStore: &pbsubstreams.Module_KindStore{}},
		Inputs: []*pbsubstreams.Module_Input{
			{
				Input: &pbsubstreams.Module_Input_Store_{Store: &pbsubstreams.Module_Input_Store{
					ModuleName: "C",
				}},
			},
		},
	},
	{
		Name: "G",
		Kind: &pbsubstreams.Module_KindStore_{KindStore: &pbsubstreams.Module_KindStore{}},
		Inputs: []*pbsubstreams.Module_Input{
			{
				Input: &pbsubstreams.Module_Input_Map_{Map: &pbsubstreams.Module_Input_Map{
					ModuleName: "D",
				}},
			},
			{
				Input: &pbsubstreams.Module_Input_Store_{Store: &pbsubstreams.Module_Input_Store{
					ModuleName: "E",
				}},
			},
		},
	},
	{
		Name: "K",
		Kind: &pbsubstreams.Module_KindStore_{KindStore: &pbsubstreams.Module_KindStore{}},
		Inputs: []*pbsubstreams.Module_Input{
			{
				Input: &pbsubstreams.Module_Input_Store_{Store: &pbsubstreams.Module_Input_Store{
					ModuleName: "G",
				}},
			},
		},
	},
	{
		Name:   "H",
		Kind:   &pbsubstreams.Module_KindMap_{KindMap: &pbsubstreams.Module_KindMap{}},
		Inputs: nil,
	},
}

func TestModuleGraph_ParentsOf(t *testing.T) {
	g, err := NewModuleGraph(testModules)
	assert.NoError(t, err)

	parents, err := g.ParentsOf("G")
	assert.NoError(t, err)

	var res []string
	for _, p := range parents {
		res = append(res, p.Name)
	}

	sort.Strings(res)

	assert.Equal(t, []string{"D", "E"}, res)
}

func TestModuleGraph_AncestorsOf(t *testing.T) {
	g, err := NewModuleGraph(testModules)
	assert.NoError(t, err)

	parents, err := g.AncestorsOf("G")
	assert.NoError(t, err)

	var res []string
	for _, p := range parents {
		res = append(res, p.Name)
	}

	sort.Strings(res)

	assert.Equal(t, []string{"A", "B", "C", "D", "E"}, res)
}

func TestModuleGraph_AncestorStoresOf(t *testing.T) {
	g, err := NewModuleGraph(testModules)
	assert.NoError(t, err)

	ancestors, err := g.AncestorStoresOf("G")
	assert.NoError(t, err)

	var res []string
	for _, a := range ancestors {
		res = append(res, a.Name)
	}

	sort.Strings(res)

	assert.Equal(t, []string{"B", "E"}, res)
}

func TestModuleGraph_GroupedAncestorStoresOf(t *testing.T) {
	g, err := NewModuleGraph(testModules)
	assert.NoError(t, err)

	groupedAncestors, err := g.GroupedAncestorStores("G")
	require.Nil(t, err)

	require.Len(t, groupedAncestors, 2)
}

func TestModuleGraph_ModulesDownTo(t *testing.T) {
	g, err := NewModuleGraph(testModules)
	assert.NoError(t, err)

	mods, err := g.ModulesDownTo([]string{"G"})
	assert.NoError(t, err)

	var res []string
	for _, p := range mods {
		res = append(res, p.Name)
	}

	sort.Strings(res)

	assert.Equal(t, []string{"A", "B", "C", "D", "E", "G"}, res)
}

func TestModuleGraph_StoresDownTo(t *testing.T) {
	g, err := NewModuleGraph(testModules)
	assert.NoError(t, err)

	mods, err := g.StoresDownTo([]string{"G"})
	assert.NoError(t, err)

	var res []string
	for _, p := range mods {
		res = append(res, p.Name)
	}

	sort.Strings(res)

	assert.Equal(t, []string{"B", "E", "G"}, res)
}

func TestModuleGraph_computeStartBlocks(t *testing.T) {
	var oldValue = bstream.GetProtocolFirstStreamableBlock
	bstream.GetProtocolFirstStreamableBlock = uint64(99)
	defer func() {
		bstream.GetProtocolFirstStreamableBlock = oldValue
	}()

	var startBlockTestModule = []*pbsubstreams.Module{
		{
			Name:       "block_to_pairs",
			StartBlock: twenty,
		},
		{
			Name:       "pairs",
			StartBlock: UNSET,
			Inputs: []*pbsubstreams.Module_Input{
				{
					Input: &pbsubstreams.Module_Input_Store_{
						Store: &pbsubstreams.Module_Input_Store{
							ModuleName: "block_to_pairs",
						},
					},
				},
			},
		},
		{
			Name:       "block_to_reserves",
			StartBlock: UNSET,
			Inputs: []*pbsubstreams.Module_Input{
				{
					Input: &pbsubstreams.Module_Input_Store_{
						Store: &pbsubstreams.Module_Input_Store{
							ModuleName: "pairs",
						},
					},
				},
			},
		},
		{
			Name:       "reserves",
			StartBlock: UNSET,
			Inputs: []*pbsubstreams.Module_Input{
				{
					Input: &pbsubstreams.Module_Input_Store_{
						Store: &pbsubstreams.Module_Input_Store{
							ModuleName: "pairs",
						},
					},
				},
				{
					Input: &pbsubstreams.Module_Input_Store_{
						Store: &pbsubstreams.Module_Input_Store{
							ModuleName: "block_to_reserves",
						},
					},
				},
			},
		},
		{
			Name:       "prices",
			StartBlock: UNSET,
			Inputs: []*pbsubstreams.Module_Input{
				{
					Input: &pbsubstreams.Module_Input_Store_{
						Store: &pbsubstreams.Module_Input_Store{
							ModuleName: "pairs",
						},
					},
				},
				{
					Input: &pbsubstreams.Module_Input_Store_{
						Store: &pbsubstreams.Module_Input_Store{
							ModuleName: "block_to_reserves",
						},
					},
				},
				{
					Input: &pbsubstreams.Module_Input_Store_{
						Store: &pbsubstreams.Module_Input_Store{
							ModuleName: "reserves",
						},
					},
				},
			},
		},
		{
			Name:       "mint_burn_swaps_extractor",
			StartBlock: UNSET,
			Inputs: []*pbsubstreams.Module_Input{
				{
					Input: &pbsubstreams.Module_Input_Store_{
						Store: &pbsubstreams.Module_Input_Store{
							ModuleName: "pairs",
						},
					},
				},
				{
					Input: &pbsubstreams.Module_Input_Store_{
						Store: &pbsubstreams.Module_Input_Store{
							ModuleName: "prices",
						},
					},
				},
			},
		},
		{
			Name:       "volumes",
			StartBlock: UNSET,
			Inputs: []*pbsubstreams.Module_Input{
				{
					Input: &pbsubstreams.Module_Input_Store_{
						Store: &pbsubstreams.Module_Input_Store{
							ModuleName: "mint_burn_swaps_extractor",
						},
					},
				},
			},
		},
		{
			Name:       "database_output",
			StartBlock: UNSET,
			Inputs: []*pbsubstreams.Module_Input{
				{
					Input: &pbsubstreams.Module_Input_Store_{
						Store: &pbsubstreams.Module_Input_Store{
							ModuleName: "mint_burn_swaps_extractor",
						},
					},
				},
				{
					Input: &pbsubstreams.Module_Input_Store_{
						Store: &pbsubstreams.Module_Input_Store{
							ModuleName: "volumes",
						},
					},
				},
			},
		},
		{
			Name:       "totals",
			StartBlock: UNSET,
			Inputs: []*pbsubstreams.Module_Input{
				{
					Input: &pbsubstreams.Module_Input_Store_{
						Store: &pbsubstreams.Module_Input_Store{
							ModuleName: "mint_burn_swaps_extractor",
						},
					},
				},
				{
					Input: &pbsubstreams.Module_Input_Store_{
						Store: &pbsubstreams.Module_Input_Store{
							ModuleName: "block_to_pairs",
						},
					},
				},
			},
		},
	}

	_, err := NewModuleGraph(startBlockTestModule)
	require.NoError(t, err)

	assert.Equal(t, uint64(20), startBlockTestModule[0].StartBlock)
	assert.Equal(t, uint64(20), startBlockTestModule[1].StartBlock)
}

func TestModuleGraph_ComputeStartBlocks_WithOneParentContainingNoStartBlock(t *testing.T) {
	var oldValue = bstream.GetProtocolFirstStreamableBlock
	bstream.GetProtocolFirstStreamableBlock = uint64(99)
	defer func() {
		bstream.GetProtocolFirstStreamableBlock = oldValue
	}()

	var testModules = []*pbsubstreams.Module{
		{
			Name:       "A",
			StartBlock: UNSET,
		},
		{
			Name:       "B",
			StartBlock: UNSET,
			Inputs: []*pbsubstreams.Module_Input{
				{
					Input: &pbsubstreams.Module_Input_Store_{
						Store: &pbsubstreams.Module_Input_Store{
							ModuleName: "A",
						},
					},
				},
			},
		},
	}

	_, err := NewModuleGraph(testModules)
	require.NoError(t, err)

	assert.Equal(t, bstream.GetProtocolFirstStreamableBlock, testModules[0].StartBlock)
	assert.Equal(t, bstream.GetProtocolFirstStreamableBlock, testModules[1].StartBlock)
}

func TestModuleGraph_ComputeStartBlocks_WithOneParentContainingAStartBlock(t *testing.T) {
	var testModules = []*pbsubstreams.Module{
		{
			Name:       "A",
			StartBlock: ten,
		},
		{
			Name:       "B",
			StartBlock: UNSET,
			Inputs: []*pbsubstreams.Module_Input{
				{
					Input: &pbsubstreams.Module_Input_Store_{
						Store: &pbsubstreams.Module_Input_Store{
							ModuleName: "A",
						},
					},
				},
			},
		},
	}

	_, err := NewModuleGraph(testModules)
	require.NoError(t, err)

	assert.Equal(t, uint64(10), testModules[0].GetStartBlock())
	assert.Equal(t, uint64(10), testModules[1].GetStartBlock())
}

func TestModuleGraph_ComputeStartBlocks_WithTwoParentsAndAGrandParentContainingStartBlock(t *testing.T) {
	var testModules = []*pbsubstreams.Module{
		{
			Name:       "A",
			StartBlock: ten,
		},
		{
			Name:       "B",
			StartBlock: UNSET,
			Inputs: []*pbsubstreams.Module_Input{
				{
					Input: &pbsubstreams.Module_Input_Store_{
						Store: &pbsubstreams.Module_Input_Store{
							ModuleName: "A",
						},
					},
				},
			},
		},
		{
			Name:       "C",
			StartBlock: UNSET,
			Inputs: []*pbsubstreams.Module_Input{
				{
					Input: &pbsubstreams.Module_Input_Store_{
						Store: &pbsubstreams.Module_Input_Store{
							ModuleName: "A",
						},
					},
				},
			},
		},
		{
			Name:       "D",
			StartBlock: UNSET,
			Inputs: []*pbsubstreams.Module_Input{
				{
					Input: &pbsubstreams.Module_Input_Store_{
						Store: &pbsubstreams.Module_Input_Store{
							ModuleName: "B",
						},
					},
				},
				{
					Input: &pbsubstreams.Module_Input_Store_{
						Store: &pbsubstreams.Module_Input_Store{
							ModuleName: "C",
						},
					},
				},
			},
		},
	}

	_, err := NewModuleGraph(testModules)
	require.NoError(t, err)

	assert.Equal(t, uint64(10), testModules[0].GetStartBlock())
	assert.Equal(t, uint64(10), testModules[1].GetStartBlock())
	assert.Equal(t, uint64(10), testModules[2].GetStartBlock())
	assert.Equal(t, uint64(10), testModules[3].GetStartBlock())
}

func TestModuleGraph_ComputeStartBlocks_WithThreeParentsEachContainingAStartBlock(t *testing.T) {
	var testModules = []*pbsubstreams.Module{
		{
			Name:       "A",
			StartBlock: ten,
		},
		{
			Name:       "B",
			StartBlock: twenty,
		},
		{
			Name:       "C",
			StartBlock: thirty,
		},
		{
			Name:       "D",
			StartBlock: UNSET,
			Inputs: []*pbsubstreams.Module_Input{
				{
					Input: &pbsubstreams.Module_Input_Store_{
						Store: &pbsubstreams.Module_Input_Store{
							ModuleName: "A",
						},
					},
				},
				{
					Input: &pbsubstreams.Module_Input_Store_{
						Store: &pbsubstreams.Module_Input_Store{
							ModuleName: "B",
						},
					},
				},
				{
					Input: &pbsubstreams.Module_Input_Store_{
						Store: &pbsubstreams.Module_Input_Store{
							ModuleName: "C",
						},
					},
				},
			},
		},
	}

	require.Panics(t, func() {
		NewModuleGraph(testModules)
	})
}

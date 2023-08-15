package manifest

import (
	"sort"
	"testing"

	"github.com/streamingfast/bstream"
	"github.com/stretchr/testify/require"

	"github.com/stretchr/testify/assert"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

var testModules = NewTestModules()

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

func TestModuleGraph_ChildrenOf(t *testing.T) {
	g, err := NewModuleGraph(testModules)
	assert.NoError(t, err)

	children, err := g.ChildrenOf("C")
	assert.NoError(t, err)

	var res []string
	for _, c := range children {
		res = append(res, c.Name)
	}

	sort.Strings(res)

	assert.Equal(t, []string{"E", "F"}, res)
}

func TestModuleGraph_Context(t *testing.T) {
	g, err := NewModuleGraph(testModules)
	assert.NoError(t, err)

	parents, children, err := g.Context("G")
	assert.NoError(t, err)

	assert.Equal(t, 2, len(parents))
	assert.Equal(t, parents, []string{"D", "E"})
	assert.Equal(t, 1, len(children))
	assert.Equal(t, children, []string{"K"})
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

	assert.Equal(t, []string{"Am", "As", "B", "C", "D", "E"}, res)
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

	assert.Equal(t, []string{"As", "B", "E"}, res)
}

func TestModuleGraph_GroupedAncestorStoresOf(t *testing.T) {
	g, err := NewModuleGraph(testModules)
	assert.NoError(t, err)

	groupedAncestors, err := g.GroupedAncestorStores("G")
	require.Nil(t, err)

	require.Len(t, groupedAncestors, 3)
}

func TestModuleGraph_ModulesDownTo(t *testing.T) {
	g, err := NewModuleGraph(testModules)
	assert.NoError(t, err)

	mods, err := g.ModulesDownTo("G")
	assert.NoError(t, err)

	var res []string
	for _, p := range mods {
		res = append(res, p.Name)
	}

	sort.Strings(res)

	assert.Equal(t, []string{"Am", "As", "B", "C", "D", "E", "G"}, res)
}

func TestModuleGraph_StoresDownTo(t *testing.T) {
	g, err := NewModuleGraph(testModules)
	assert.NoError(t, err)

	mods, err := g.StoresDownTo("G")
	assert.NoError(t, err)

	var res []string
	for _, p := range mods {
		res = append(res, p.Name)
	}

	sort.Strings(res)

	assert.Equal(t, []string{"As", "B", "E", "G"}, res)
}

func TestModuleGraph_computeInitialBlocks(t *testing.T) {
	var oldValue = bstream.GetProtocolFirstStreamableBlock
	bstream.GetProtocolFirstStreamableBlock = uint64(99)
	defer func() {
		bstream.GetProtocolFirstStreamableBlock = oldValue
	}()

	var startBlockTestModule = []*pbsubstreams.Module{
		{
			Name:         "block_to_pairs",
			InitialBlock: twenty,
		},
		{
			Name:         "pairs",
			InitialBlock: UNSET,
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
			Name:         "block_to_reserves",
			InitialBlock: UNSET,
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
			Name:         "reserves",
			InitialBlock: UNSET,
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
			Name:         "prices",
			InitialBlock: UNSET,
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
			Name:         "mint_burn_swaps_extractor",
			InitialBlock: fourty,
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
			Name:         "volumes",
			InitialBlock: UNSET,
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
			Name:         "database_output",
			InitialBlock: UNSET,
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
			Name:         "totals",
			InitialBlock: UNSET,
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

	assert.Equal(t, uint64(20), startBlockTestModule[0].InitialBlock)
	assert.Equal(t, uint64(20), startBlockTestModule[1].InitialBlock)
	assert.Equal(t, uint64(40), startBlockTestModule[len(startBlockTestModule)-1].InitialBlock)
}

func TestModuleGraph_ComputeInitialBlocks_WithOneParentContainingNoInitialBlock(t *testing.T) {
	var oldValue = bstream.GetProtocolFirstStreamableBlock
	bstream.GetProtocolFirstStreamableBlock = uint64(99)
	defer func() {
		bstream.GetProtocolFirstStreamableBlock = oldValue
	}()

	var testModules = []*pbsubstreams.Module{
		{
			Name:         "A",
			InitialBlock: UNSET,
		},
		{
			Name:         "B",
			InitialBlock: UNSET,
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

	assert.Equal(t, bstream.GetProtocolFirstStreamableBlock, testModules[0].InitialBlock)
	assert.Equal(t, bstream.GetProtocolFirstStreamableBlock, testModules[1].InitialBlock)
}

func TestModuleGraph_ComputeInitialBlocks_WithOneParentContainingAInitialBlock(t *testing.T) {
	var testModules = []*pbsubstreams.Module{
		{
			Name:         "A",
			InitialBlock: ten,
		},
		{
			Name:         "B",
			InitialBlock: UNSET,
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

	assert.Equal(t, uint64(10), testModules[0].GetInitialBlock())
	assert.Equal(t, uint64(10), testModules[1].GetInitialBlock())
}

func TestModuleGraph_ComputeInitialBlocks_WithTwoParentsAndAGrandParentContainingInitialBlock(t *testing.T) {
	var testModules = []*pbsubstreams.Module{
		{
			Name:         "A",
			InitialBlock: ten,
		},
		{
			Name:         "B",
			InitialBlock: UNSET,
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
			Name:         "C",
			InitialBlock: UNSET,
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
			Name:         "D",
			InitialBlock: UNSET,
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

	assert.Equal(t, uint64(10), testModules[0].GetInitialBlock())
	assert.Equal(t, uint64(10), testModules[1].GetInitialBlock())
	assert.Equal(t, uint64(10), testModules[2].GetInitialBlock())
	assert.Equal(t, uint64(10), testModules[3].GetInitialBlock())
}

func TestModuleGraph_ComputeInitialBlocks_WithThreeParentsEachContainingAInitialBlock(t *testing.T) {
	var testModules = []*pbsubstreams.Module{
		{
			Name:         "A",
			InitialBlock: ten,
		},
		{
			Name:         "B",
			InitialBlock: twenty,
		},
		{
			Name:         "C",
			InitialBlock: thirty,
		},
		{
			Name:         "D",
			InitialBlock: UNSET,
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

	_, err := NewModuleGraph(testModules)
	assert.Equal(t, `cannot deterministically determine the initialBlock for module "D"; multiple inputs have conflicting initial blocks defined or inherited`, err.Error())
}

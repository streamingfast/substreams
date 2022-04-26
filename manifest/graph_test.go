package manifest

import (
	"sort"
	"testing"

	"github.com/streamingfast/bstream"
	"github.com/test-go/testify/require"

	pbtransform "github.com/streamingfast/substreams/pb/sf/substreams/transform/v1"
	"github.com/stretchr/testify/assert"
)

var ten = uint64(10)
var twenty = uint64(20)
var thirty = uint64(30)

var testModules = []*pbtransform.Module{
	{
		Name: "A",
	},
	{
		Name:       "B",
		StartBlock: ten,
		Kind:       &pbtransform.Module_KindStore{KindStore: &pbtransform.KindStore{}},
		Inputs: []*pbtransform.Input{
			{
				Input: &pbtransform.Input_Store{Store: &pbtransform.InputStore{
					ModuleName: "A",
				}},
			},
		},
	},
	{
		Name:       "C",
		StartBlock: twenty,
		Kind:       &pbtransform.Module_KindMap{KindMap: &pbtransform.KindMap{}},
		Inputs: []*pbtransform.Input{
			{
				Input: &pbtransform.Input_Store{Store: &pbtransform.InputStore{
					ModuleName: "A",
				}},
			},
		},
	},
	{
		Name: "D",
		Kind: &pbtransform.Module_KindMap{KindMap: &pbtransform.KindMap{}},
		Inputs: []*pbtransform.Input{
			{
				Input: &pbtransform.Input_Store{Store: &pbtransform.InputStore{
					ModuleName: "B",
				}},
			},
		},
	},
	{
		Name:       "E",
		StartBlock: ten,
		Kind:       &pbtransform.Module_KindStore{KindStore: &pbtransform.KindStore{}},
		Inputs: []*pbtransform.Input{
			{
				Input: &pbtransform.Input_Store{Store: &pbtransform.InputStore{
					ModuleName: "C",
				}},
			},
		},
	},
	{
		Name: "F",
		Kind: &pbtransform.Module_KindStore{KindStore: &pbtransform.KindStore{}},
		Inputs: []*pbtransform.Input{
			{
				Input: &pbtransform.Input_Store{Store: &pbtransform.InputStore{
					ModuleName: "C",
				}},
			},
		},
	},
	{
		Name: "G",
		Kind: &pbtransform.Module_KindStore{KindStore: &pbtransform.KindStore{}},
		Inputs: []*pbtransform.Input{
			{
				Input: &pbtransform.Input_Store{Store: &pbtransform.InputStore{
					ModuleName: "D",
				}},
			},
			{
				Input: &pbtransform.Input_Store{Store: &pbtransform.InputStore{
					ModuleName: "E",
				}},
			},
		},
	},
	{
		Name: "K",
		Kind: &pbtransform.Module_KindStore{KindStore: &pbtransform.KindStore{}},
		Inputs: []*pbtransform.Input{
			{
				Input: &pbtransform.Input_Store{Store: &pbtransform.InputStore{
					ModuleName: "G",
				}},
			},
		},
	},
	{
		Name:   "H",
		Kind:   &pbtransform.Module_KindMap{KindMap: &pbtransform.KindMap{}},
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

	var startBlockTestModule = []*pbtransform.Module{
		{
			Name:       "block_to_pairs",
			StartBlock: twenty,
		},
		{
			Name:       "pairs",
			StartBlock: UNSET,
			Inputs: []*pbtransform.Input{
				{
					Input: &pbtransform.Input_Store{
						Store: &pbtransform.InputStore{
							ModuleName: "block_to_pairs",
						},
					},
				},
			},
		},
		{
			Name:       "block_to_reserves",
			StartBlock: UNSET,
			Inputs: []*pbtransform.Input{
				{
					Input: &pbtransform.Input_Store{
						Store: &pbtransform.InputStore{
							ModuleName: "pairs",
						},
					},
				},
			},
		},
		{
			Name:       "reserves",
			StartBlock: UNSET,
			Inputs: []*pbtransform.Input{
				{
					Input: &pbtransform.Input_Store{
						Store: &pbtransform.InputStore{
							ModuleName: "pairs",
						},
					},
				},
				{
					Input: &pbtransform.Input_Store{
						Store: &pbtransform.InputStore{
							ModuleName: "block_to_reserves",
						},
					},
				},
			},
		},
		{
			Name:       "prices",
			StartBlock: UNSET,
			Inputs: []*pbtransform.Input{
				{
					Input: &pbtransform.Input_Store{
						Store: &pbtransform.InputStore{
							ModuleName: "pairs",
						},
					},
				},
				{
					Input: &pbtransform.Input_Store{
						Store: &pbtransform.InputStore{
							ModuleName: "block_to_reserves",
						},
					},
				},
				{
					Input: &pbtransform.Input_Store{
						Store: &pbtransform.InputStore{
							ModuleName: "reserves",
						},
					},
				},
			},
		},
		{
			Name:       "mint_burn_swaps_extractor",
			StartBlock: UNSET,
			Inputs: []*pbtransform.Input{
				{
					Input: &pbtransform.Input_Store{
						Store: &pbtransform.InputStore{
							ModuleName: "pairs",
						},
					},
				},
				{
					Input: &pbtransform.Input_Store{
						Store: &pbtransform.InputStore{
							ModuleName: "prices",
						},
					},
				},
			},
		},
		{
			Name:       "volumes",
			StartBlock: UNSET,
			Inputs: []*pbtransform.Input{
				{
					Input: &pbtransform.Input_Store{
						Store: &pbtransform.InputStore{
							ModuleName: "mint_burn_swaps_extractor",
						},
					},
				},
			},
		},
		{
			Name:       "database_output",
			StartBlock: UNSET,
			Inputs: []*pbtransform.Input{
				{
					Input: &pbtransform.Input_Store{
						Store: &pbtransform.InputStore{
							ModuleName: "mint_burn_swaps_extractor",
						},
					},
				},
				{
					Input: &pbtransform.Input_Store{
						Store: &pbtransform.InputStore{
							ModuleName: "volumes",
						},
					},
				},
			},
		},
		{
			Name:       "totals",
			StartBlock: UNSET,
			Inputs: []*pbtransform.Input{
				{
					Input: &pbtransform.Input_Store{
						Store: &pbtransform.InputStore{
							ModuleName: "mint_burn_swaps_extractor",
						},
					},
				},
				{
					Input: &pbtransform.Input_Store{
						Store: &pbtransform.InputStore{
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

	var testModules = []*pbtransform.Module{
		{
			Name:       "A",
			StartBlock: UNSET,
		},
		{
			Name:       "B",
			StartBlock: UNSET,
			Inputs: []*pbtransform.Input{
				{
					Input: &pbtransform.Input_Store{
						Store: &pbtransform.InputStore{
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
	var testModules = []*pbtransform.Module{
		{
			Name:       "A",
			StartBlock: ten,
		},
		{
			Name:       "B",
			StartBlock: UNSET,
			Inputs: []*pbtransform.Input{
				{
					Input: &pbtransform.Input_Store{
						Store: &pbtransform.InputStore{
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
	var testModules = []*pbtransform.Module{
		{
			Name:       "A",
			StartBlock: ten,
		},
		{
			Name:       "B",
			StartBlock: UNSET,
			Inputs: []*pbtransform.Input{
				{
					Input: &pbtransform.Input_Store{
						Store: &pbtransform.InputStore{
							ModuleName: "A",
						},
					},
				},
			},
		},
		{
			Name:       "C",
			StartBlock: UNSET,
			Inputs: []*pbtransform.Input{
				{
					Input: &pbtransform.Input_Store{
						Store: &pbtransform.InputStore{
							ModuleName: "A",
						},
					},
				},
			},
		},
		{
			Name:       "D",
			StartBlock: UNSET,
			Inputs: []*pbtransform.Input{
				{
					Input: &pbtransform.Input_Store{
						Store: &pbtransform.InputStore{
							ModuleName: "B",
						},
					},
				},
				{
					Input: &pbtransform.Input_Store{
						Store: &pbtransform.InputStore{
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
	var testModules = []*pbtransform.Module{
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
			Inputs: []*pbtransform.Input{
				{
					Input: &pbtransform.Input_Store{
						Store: &pbtransform.InputStore{
							ModuleName: "A",
						},
					},
				},
				{
					Input: &pbtransform.Input_Store{
						Store: &pbtransform.InputStore{
							ModuleName: "B",
						},
					},
				},
				{
					Input: &pbtransform.Input_Store{
						Store: &pbtransform.InputStore{
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

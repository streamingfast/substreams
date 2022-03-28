package manifest

import (
	"fmt"
	"sort"
	"testing"

	pbtransform "github.com/streamingfast/substreams/pb/sf/substreams/transform/v1"
	"github.com/stretchr/testify/assert"
)

var ten = uint64(10)
var twenty = uint64(20)

var testModules = []*pbtransform.Module{
	{
		Name: "A",
	},
	{
		Name:       "B",
		StartBlock: &ten,
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
		StartBlock: &twenty,
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
		Name: "E",
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

	mods, err := g.ModulesDownTo("G")
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

	mods, err := g.StoresDownTo("G")
	assert.NoError(t, err)

	var res []string
	for _, p := range mods {
		res = append(res, p.Name)
	}

	sort.Strings(res)

	assert.Equal(t, []string{"B", "E", "G"}, res)
}

func TestModuleGraph_computeStartBlocks(t *testing.T) {
	g, err := NewModuleGraph(testModules)
	assert.NoError(t, err)
	for _, module := range g.modules {
		fmt.Println(module.Name, "start block:", *module.StartBlock)
	}
}

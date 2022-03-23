package manifest

import (
	"sort"
	"testing"

	pbtransform "github.com/streamingfast/substreams/pb/sf/substreams/transform/v1"
	"github.com/stretchr/testify/assert"
)

var testModules = []*pbtransform.Module{
	{
		Name: "A",
	},
	{
		Name: "B",
		Kind: &pbtransform.Module_KindStore{KindStore: &pbtransform.KindStore{}},
		Inputs: []*pbtransform.Input{
			{
				Input: &pbtransform.Input_Store{Store: &pbtransform.InputStore{
					ModuleName: "A",
				}},
			},
		},
	},
	{
		Name: "C",
		Kind: &pbtransform.Module_KindMap{KindMap: &pbtransform.KindMap{}},
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

func TestModuleGraph_GroupedModulesDownTo(t *testing.T) {
	g, err := NewModuleGraph(testModules)
	assert.NoError(t, err)

	modgroups, err := g.GroupedModulesDownTo("G")
	assert.NoError(t, err)

	var res [][]string
	for _, modgroup := range modgroups {
		var mods []string
		for _, p := range modgroup {
			mods = append(mods, p.Name)
		}
		sort.Strings(mods)
		res = append(res, mods)
	}

	expected := [][]string{
		{"A"}, {"B", "C"}, {"D", "E"}, {"G"},
	}

	assert.Equal(t, expected, res)
}

func TestModuleGraph_GroupedStoresDownTo(t *testing.T) {
	g, err := NewModuleGraph(testModules)
	assert.NoError(t, err)

	modgroups, err := g.GroupedStoresDownTo("G")
	assert.NoError(t, err)

	var res [][]string
	for _, modgroup := range modgroups {
		var mods []string
		for _, p := range modgroup {
			mods = append(mods, p.Name)
		}
		sort.Strings(mods)
		res = append(res, mods)
	}

	expected := [][]string{
		{"B"}, {"E"}, {"G"},
	}

	assert.Equal(t, expected, res)
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

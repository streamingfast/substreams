package manifest

import (
	"github.com/stretchr/testify/assert"
	"sort"
	"testing"
)

var testModules = []*Module{
	&Module{
		Name:   "A",
		Kind:   ModuleKindMap,
		Inputs: nil,
	},
	&Module{
		Name: "B",
		Kind: ModuleKindStore,
		Inputs: []*Input{
			&Input{
				Map:  "A",
				Name: "map:A",
			},
		},
	},
	&Module{
		Name: "C",
		Kind: ModuleKindMap,
		Inputs: []*Input{
			&Input{
				Map:  "A",
				Name: "map:A",
			},
		},
	},
	&Module{
		Name: "D",
		Kind: ModuleKindMap,
		Inputs: []*Input{
			&Input{
				Name:  "store:B",
				Store: "B",
			},
		},
	},
	&Module{
		Name: "E",
		Kind: ModuleKindStore,
		Inputs: []*Input{
			&Input{
				Map:  "C",
				Name: "map:C",
			},
		},
	},
	&Module{
		Name: "F",
		Kind: ModuleKindStore,
		Inputs: []*Input{
			&Input{
				Name: "map:C",
				Map:  "C",
			},
		},
	},
	&Module{
		Name: "G",
		Kind: ModuleKindStore,
		Inputs: []*Input{
			&Input{
				Map:  "D",
				Name: "map:D",
			},
			&Input{
				Store: "E",
				Name:  "store:E",
			},
		},
	},
	&Module{
		Name:   "H",
		Kind:   ModuleKindMap,
		Inputs: nil,
	},
}

func TestModuleGraph_FromManifestFile_AncestorsOf(t *testing.T) {
	man, err := New("./test/test_manifest.yaml")
	assert.NoError(t, err)

	x, _ := man.Graph.AncestorsOf("reserves_extractor")
	assert.Equal(t, 2, len(x))
	assert.Equal(t, "pair_extractor", x[0].Name)
	assert.Equal(t, "pairs", x[1].Name)
}

func TestModuleGraph_FromManifestFile_ModulesDownTo(t *testing.T) {
	man, err := New("./test/test_manifest.yaml")
	assert.NoError(t, err)

	x, _ := man.Graph.ModulesDownTo("reserves_extractor")
	assert.Equal(t, 3, len(x))
	assert.Equal(t, "pair_extractor", x[0].Name)
	assert.Equal(t, "pairs", x[1].Name)
	assert.Equal(t, "reserves_extractor", x[2].Name)

}

func TestModuleGraph_FromManifestFile_GroupedModulesDownTo(t *testing.T) {
	man, err := New("./test/test_manifest.yaml")
	assert.NoError(t, err)

	xy, _ := man.Graph.GroupedModulesDownTo("reserves_extractor")
	assert.Equal(t, 3, len(xy))
	assert.Equal(t, "pair_extractor", xy[0][0].Name)
	assert.Equal(t, "pairs", xy[1][0].Name)
	assert.Equal(t, "reserves_extractor", xy[2][0].Name)
}

func TestModuleGraph_ParentsOf(t *testing.T) {
	g, err := NewModuleGraph(testModules)
	assert.NoError(t, err)

	parents, err := g.ParentsOf("G")
	assert.NoError(t, err)

	var res []string
	for _, p := range parents {
		res = append(res, p.String())
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
		res = append(res, p.String())
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
		res = append(res, a.String())
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
			mods = append(mods, p.String())
		}
		sort.Strings(mods)
		res = append(res, mods)
	}

	expected := [][]string{
		{"A"}, {"B", "C"}, {"D", "E"}, {"G"},
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
		res = append(res, p.String())
	}

	sort.Strings(res)

	assert.Equal(t, []string{"A", "B", "C", "D", "E", "G"}, res)
}

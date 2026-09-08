package manifest

import (
	"os"
	"path/filepath"
	"testing"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func TestDensifyNetworks_recordsDerivedInitialBlocksPerNetwork(t *testing.T) {
	pkg := &pbsubstreams.Package{
		Network: "mainnet",
		Modules: &pbsubstreams.Modules{
			Modules: []*pbsubstreams.Module{
				{Name: "mod1", InitialBlock: UNSET},
				{Name: "mod2", InitialBlock: UNSET, Inputs: []*pbsubstreams.Module_Input{mapInput("mod1")}},
			},
		},
		Networks: map[string]*pbsubstreams.NetworkParams{
			"mainnet": {InitialBlocks: map[string]uint64{"mod1": 200}},
			"sepolia": {InitialBlocks: map[string]uint64{"mod1": 400}},
		},
	}

	densifyPackage(t, pkg)

	assert.Equal(t, map[string]uint64{"mod1": 200, "mod2": 200}, pkg.Networks["mainnet"].InitialBlocks)
	assert.Equal(t, map[string]uint64{"mod1": 400, "mod2": 400}, pkg.Networks["sepolia"].InitialBlocks)
}

func TestDensifyNetworks_leavesModulesUntouched(t *testing.T) {
	pkg := &pbsubstreams.Package{
		Network: "mainnet",
		Modules: &pbsubstreams.Modules{
			Modules: []*pbsubstreams.Module{
				{Name: "mod1", InitialBlock: UNSET},
				{Name: "mod2", InitialBlock: UNSET, Inputs: []*pbsubstreams.Module_Input{mapInput("mod1")}},
			},
		},
		Networks: map[string]*pbsubstreams.NetworkParams{
			"mainnet": {InitialBlocks: map[string]uint64{"mod1": 200}},
			"sepolia": {InitialBlocks: map[string]uint64{"mod1": 400}},
		},
	}

	densifyPackage(t, pkg)

	assert.Equal(t, uint64(UNSET), pkg.Modules.Modules[0].InitialBlock, "mod1 must keep its authored value so ApplyNetwork can still decide")
	assert.Equal(t, uint64(UNSET), pkg.Modules.Modules[1].InitialBlock, "mod2 must keep its authored value so ApplyNetwork can still decide")
}

func TestDensifyNetworks_keepsExplicitlyAuthoredInitialBlocks(t *testing.T) {
	pkg := &pbsubstreams.Package{
		Network: "mainnet",
		Modules: &pbsubstreams.Modules{
			Modules: []*pbsubstreams.Module{
				{Name: "mod1", InitialBlock: 500},
				{Name: "mod2", InitialBlock: UNSET, Inputs: []*pbsubstreams.Module_Input{mapInput("mod1")}},
			},
		},
		Networks: map[string]*pbsubstreams.NetworkParams{
			"mainnet": {},
			"sepolia": {},
		},
	}

	densifyPackage(t, pkg)

	assert.Equal(t, map[string]uint64{"mod1": 500, "mod2": 500}, pkg.Networks["mainnet"].InitialBlocks)
	assert.Equal(t, map[string]uint64{"mod1": 500, "mod2": 500}, pkg.Networks["sepolia"].InitialBlocks)
}

func TestDensifyNetworks_recordsEffectiveParams(t *testing.T) {
	pkg := &pbsubstreams.Package{
		Network: "mainnet",
		Modules: &pbsubstreams.Modules{
			Modules: []*pbsubstreams.Module{
				{Name: "mod1", InitialBlock: 100, Inputs: []*pbsubstreams.Module_Input{paramsInput("val=default")}},
				{Name: "mod2", InitialBlock: UNSET, Inputs: []*pbsubstreams.Module_Input{mapInput("mod1")}},
			},
		},
		Networks: map[string]*pbsubstreams.NetworkParams{
			"mainnet": {Params: map[string]string{"mod1": "val=toto"}},
			"sepolia": {},
		},
	}

	densifyPackage(t, pkg)

	assert.Equal(t, map[string]string{"mod1": "val=toto"}, pkg.Networks["mainnet"].Params)
	assert.Equal(t, map[string]string{"mod1": "val=default"}, pkg.Networks["sepolia"].Params,
		"a network without an override must record the module's authored param value")
}

func TestDensifyNetworks_skipsModulesWithoutParamsInput(t *testing.T) {
	pkg := &pbsubstreams.Package{
		Network: "mainnet",
		Modules: &pbsubstreams.Modules{
			Modules: []*pbsubstreams.Module{
				{Name: "mod1", InitialBlock: 100, Inputs: []*pbsubstreams.Module_Input{paramsInput("val=default")}},
				{Name: "mod2", InitialBlock: UNSET, Inputs: []*pbsubstreams.Module_Input{mapInput("mod1")}},
			},
		},
		Networks: map[string]*pbsubstreams.NetworkParams{
			"mainnet": {},
		},
	}

	densifyPackage(t, pkg)

	assert.NotContains(t, pkg.Networks["mainnet"].Params, "mod2")
}

func TestDensifyNetworks_synthesizesEntryForDefaultNetwork(t *testing.T) {
	pkg := &pbsubstreams.Package{
		Network: "mainnet",
		Modules: &pbsubstreams.Modules{
			Modules: []*pbsubstreams.Module{
				{Name: "mod1", InitialBlock: 100},
			},
		},
		Networks: map[string]*pbsubstreams.NetworkParams{
			"sepolia": {InitialBlocks: map[string]uint64{"mod1": 400}},
		},
	}

	densifyPackage(t, pkg)

	require.Contains(t, pkg.Networks, "mainnet")
	assert.Equal(t, map[string]uint64{"mod1": 100}, pkg.Networks["mainnet"].InitialBlocks)
}

func TestDensifyNetworks_isIdempotent(t *testing.T) {
	newPkg := func() *pbsubstreams.Package {
		return &pbsubstreams.Package{
			Network: "mainnet",
			Modules: &pbsubstreams.Modules{
				Modules: []*pbsubstreams.Module{
					{Name: "mod1", InitialBlock: UNSET, Inputs: []*pbsubstreams.Module_Input{paramsInput("val=default")}},
					{Name: "mod2", InitialBlock: UNSET, Inputs: []*pbsubstreams.Module_Input{mapInput("mod1")}},
				},
			},
			Networks: map[string]*pbsubstreams.NetworkParams{
				"mainnet": {InitialBlocks: map[string]uint64{"mod1": 200}},
				"sepolia": {InitialBlocks: map[string]uint64{"mod1": 400}},
			},
		}
	}

	once := newPkg()
	densifyPackage(t, once)

	twice := newPkg()
	graph := densifyPackage(t, twice)
	require.NoError(t, densifyNetworks(twice, graph))

	assert.Equal(t, once.Networks, twice.Networks)
}

// A module inheriting its initial block used to be frozen to whichever network was active when the
// package got packed, because computeInitialBlock had already replaced its UNSET marker.
func TestReadPackedPackage_derivedInitialBlockFollowsNetworkOverride(t *testing.T) {
	reader, err := NewReader("./testdata/networks.yaml")
	require.NoError(t, err)

	bundle, err := reader.Read()
	require.NoError(t, err)

	content, err := proto.Marshal(bundle.Package)
	require.NoError(t, err)

	packedPath := filepath.Join(t.TempDir(), "networks.spkg")
	require.NoError(t, os.WriteFile(packedPath, content, 0644))

	repackedReader, err := NewReader(packedPath, WithOverrideNetwork("sepolia"))
	require.NoError(t, err)

	repacked, err := repackedReader.Read()
	require.NoError(t, err)

	initialBlocks := map[string]uint64{}
	for _, module := range repacked.Package.Modules.Modules {
		initialBlocks[module.Name] = module.InitialBlock
	}

	assert.Equal(t, map[string]uint64{"mod1": 400, "mod2": 400}, initialBlocks)
}

func TestDensifyNetworks_errorsWhenANetworkYieldsConflictingInitialBlocks(t *testing.T) {
	pkg := &pbsubstreams.Package{
		Network: "mainnet",
		Modules: &pbsubstreams.Modules{
			Modules: []*pbsubstreams.Module{
				{Name: "mod1", InitialBlock: UNSET},
				{Name: "mod2", InitialBlock: UNSET},
				{Name: "mod3", InitialBlock: UNSET, Inputs: []*pbsubstreams.Module_Input{mapInput("mod1"), mapInput("mod2")}},
			},
		},
		Networks: map[string]*pbsubstreams.NetworkParams{
			"mainnet": {InitialBlocks: map[string]uint64{"mod1": 200, "mod2": 200}},
			"sepolia": {InitialBlocks: map[string]uint64{"mod1": 400, "mod2": 700}},
		},
	}

	graph, err := NewModuleGraph(pkg.Modules.Modules)
	require.NoError(t, err)

	err = densifyNetworks(pkg, graph)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "sepolia")
}

func mapInput(moduleName string) *pbsubstreams.Module_Input {
	return &pbsubstreams.Module_Input{
		Input: &pbsubstreams.Module_Input_Map_{
			Map: &pbsubstreams.Module_Input_Map{ModuleName: moduleName},
		},
	}
}

func paramsInput(value string) *pbsubstreams.Module_Input {
	return &pbsubstreams.Module_Input{
		Input: &pbsubstreams.Module_Input_Params_{
			Params: &pbsubstreams.Module_Input_Params{Value: value},
		},
	}
}

func densifyPackage(t *testing.T, pkg *pbsubstreams.Package) *ModuleGraph {
	t.Helper()

	graph, err := NewModuleGraph(pkg.Modules.Modules)
	require.NoError(t, err)

	require.NoError(t, densifyNetworks(pkg, graph))

	return graph
}

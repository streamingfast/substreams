package manifest

import (
	"encoding/json"
	"fmt"
	"sort"

	"go.uber.org/zap"

	"github.com/streamingfast/bstream"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/yourbasic/graph"
)

type ModuleGraph struct {
	*graph.Mutable

	currentHashesCache map[string][]byte // moduleName => hash

	modules     []*pbsubstreams.Module
	moduleIndex map[string]int
	indexIndex  map[int]*pbsubstreams.Module
}

func NewModuleGraph(modules []*pbsubstreams.Module) (*ModuleGraph, error) {
	g := &ModuleGraph{
		Mutable:            graph.New(len(modules)),
		modules:            modules,
		moduleIndex:        make(map[string]int),
		indexIndex:         make(map[int]*pbsubstreams.Module),
		currentHashesCache: make(map[string][]byte),
	}

	for i, module := range modules {
		g.moduleIndex[module.Name] = i
		g.indexIndex[i] = module
	}

	for i, module := range modules {
		for _, input := range module.Inputs {
			var moduleName string
			if v := input.GetMap(); v != nil {
				moduleName = v.ModuleName
			} else if v := input.GetStore(); v != nil {
				moduleName = v.ModuleName
			}
			if moduleName == "" {
				continue
			}

			if j, found := g.moduleIndex[moduleName]; found {
				g.AddCost(i, j, 1)
			}
		}
	}

	if !graph.Acyclic(g) {
		return nil, fmt.Errorf("modules graph has a cycle")
	}

	if err := computeInitialBlock(modules, g); err != nil {
		return nil, err
	}

	return g, nil
}

// ResetGraphHashes is to be called when you want to force a recomputation of the module hashes.
func (graph *ModuleGraph) ResetGraphHashes() {
	graph.currentHashesCache = make(map[string][]byte)
	// TODO: when we support multiple `initialBlock` for a given `moduleName`, we'll want
	// to make sure we call this between the boundaries, to reset the module hashes.
}

func (g *ModuleGraph) GetSources() []string {
	var sources []string
	for _, module := range g.modules {
		for _, input := range module.Inputs {
			if s := input.GetSource(); s != nil {
				sources = append(sources, s.GetType())
			}
		}
	}
	return sources
}

func computeInitialBlock(modules []*pbsubstreams.Module, g *ModuleGraph) error {
	for _, module := range modules {
		if module.InitialBlock == UNSET {
			moduleIndex := g.moduleIndex[module.Name]
			startBlock, err := startBlockForModule(moduleIndex, g)
			if err != nil {
				return err
			}

			module.InitialBlock = startBlock
			zlog.Info("computed start block", zap.String("module_name", module.Name), zap.Uint64("start_block", startBlock))
		}
	}
	return nil
}

func startBlockForModule(moduleIndex int, g *ModuleGraph) (out uint64, err error) {
	parentsInitialBlock := int64(-1)
	g.Visit(moduleIndex, func(w int, c int64) bool {
		parent := g.modules[w]
		currentInitialBlock := int64(-1)
		if parent.InitialBlock == UNSET {
			var newVal uint64
			newVal, err = startBlockForModule(w, g)
			if err != nil {
				return true
			}
			currentInitialBlock = int64(newVal)
		} else {
			currentInitialBlock = int64(parent.GetInitialBlock())
		}

		if parentsInitialBlock == -1 {
			if currentInitialBlock != -1 {
				parentsInitialBlock = currentInitialBlock
			}
			return false
		}
		if parentsInitialBlock != currentInitialBlock {
			err = fmt.Errorf("cannot deterministically determine the initialBlock for module %q; multiple inputs have conflicting initial blocks defined or inherited", g.modules[moduleIndex].Name)
			return true
		}
		return false
	})
	if err != nil {
		return uint64(0), err
	}

	if parentsInitialBlock == -1 {
		return bstream.GetProtocolFirstStreamableBlock, nil
	}
	return uint64(parentsInitialBlock), nil
}

func (g *ModuleGraph) TopologicalSort() ([]*pbsubstreams.Module, bool) {
	order, ok := graph.TopSort(g)
	if !ok {
		return nil, ok
	}

	var res []*pbsubstreams.Module
	for _, i := range order {
		res = append(res, g.indexIndex[i])
	}

	return res, ok
}

func (g *ModuleGraph) AncestorsOf(moduleName string) ([]*pbsubstreams.Module, error) {
	if _, found := g.moduleIndex[moduleName]; !found {
		return nil, fmt.Errorf("could not find module %s in graph", moduleName)
	}

	_, distances := graph.ShortestPaths(g, g.moduleIndex[moduleName])

	var res []*pbsubstreams.Module
	for i, d := range distances {
		if d >= 1 {
			res = append(res, g.indexIndex[i])
		}
	}

	return res, nil
}

func (g *ModuleGraph) AncestorStoresOf(moduleName string) ([]*pbsubstreams.Module, error) {
	ancestors, err := g.AncestorsOf(moduleName)
	if err != nil {
		return nil, err
	}

	result := make([]*pbsubstreams.Module, 0, len(ancestors))
	for _, a := range ancestors {
		kind := a.GetKindStore()
		if kind != nil {
			result = append(result, a)
		}
	}

	return result, nil
}

func (g *ModuleGraph) ParentsOf(moduleName string) ([]*pbsubstreams.Module, error) {
	if _, found := g.moduleIndex[moduleName]; !found {
		return nil, fmt.Errorf("could not find module %s in graph", moduleName)
	}

	_, distances := graph.ShortestPaths(g, g.moduleIndex[moduleName])

	var res []*pbsubstreams.Module
	for i, d := range distances {
		if d == 1 {
			res = append(res, g.indexIndex[i])
		}
	}

	return res, nil
}

func (g *ModuleGraph) StoresDownTo(moduleName string) ([]*pbsubstreams.Module, error) {
	alreadyAdded := map[string]bool{}
	topologicalIndex := map[string]int{}

	sortedModules, ok := g.TopologicalSort()
	if !ok {
		return nil, fmt.Errorf("could not get topological sort of module graph")
	}

	for i, node := range sortedModules {
		topologicalIndex[node.Name] = i
	}

	var res []*pbsubstreams.Module
	if _, found := g.moduleIndex[moduleName]; !found {
		return nil, fmt.Errorf("could not find module %s in graph", moduleName)
	}

	_, distances := graph.ShortestPaths(g, g.moduleIndex[moduleName])

	for i, d := range distances {
		if d >= 0 { // connected node or myself
			module := g.indexIndex[i]
			if module.GetKindStore() == nil {
				continue
			}

			if _, ok := alreadyAdded[module.Name]; ok {
				continue
			}

			res = append(res, module)
			alreadyAdded[module.Name] = true
		}
	}

	sort.Slice(res, func(i, j int) bool {
		return topologicalIndex[res[i].Name] > topologicalIndex[res[j].Name]
	})

	return res, nil
}

func (g *ModuleGraph) GroupedAncestorStores(moduleName string) ([][]*pbsubstreams.Module, error) {
	ancestorStores, err := g.AncestorStoresOf(moduleName)
	if err != nil {
		return nil, fmt.Errorf("getting stores down to %s: %w", moduleName, err)
	}

	distanceMap := map[int64][]*pbsubstreams.Module{}
	distanceIndex := map[*pbsubstreams.Module]int64{}

	_, distances := graph.ShortestPaths(g, g.moduleIndex[moduleName])
	for _, ancestorStore := range ancestorStores {

		for i, d := range distances {
			if g.indexIndex[i].Name == ancestorStore.Name {
				distanceMap[d] = append(distanceMap[d], ancestorStore)
				distanceIndex[ancestorStore] = d
			}
		}
	}

	var result [][]*pbsubstreams.Module
	for _, stores := range distanceMap {
		result = append(result, stores)
	}

	sort.Slice(result, func(i, j int) bool {
		di := distanceIndex[result[i][0]]
		dj := distanceIndex[result[i][0]]
		return di > dj
	})

	return result, nil
}

func (g *ModuleGraph) ParentStoresOf(moduleName string) ([]*pbsubstreams.Modules, error) {
	return nil, nil
}

func (g *ModuleGraph) ModulesDownTo(moduleName string) ([]*pbsubstreams.Module, error) {
	alreadyAdded := map[string]bool{}
	topologicalIndex := map[string]int{}

	sortedModules, ok := g.TopologicalSort()
	if !ok {
		return nil, fmt.Errorf("could not get topological sort of module graph")
	}

	for i, node := range sortedModules {
		topologicalIndex[node.Name] = i
	}

	var res []*pbsubstreams.Module
	if _, found := g.moduleIndex[moduleName]; !found {
		return nil, fmt.Errorf("could not find module %s in graph", moduleName)
	}

	_, distances := graph.ShortestPaths(g, g.moduleIndex[moduleName])

	for i, d := range distances {
		if d >= 0 { // connected node or myself
			module := g.indexIndex[i]
			if _, ok := alreadyAdded[module.Name]; ok {
				continue
			}

			res = append(res, module)
			alreadyAdded[module.Name] = true
		}
	}

	sort.Slice(res, func(i, j int) bool {
		return topologicalIndex[res[i].Name] > topologicalIndex[res[j].Name]
	})

	return res, nil
}

func (g *ModuleGraph) ModuleInitialBlock(moduleName string) (uint64, error) {
	if moduleIndex, found := g.moduleIndex[moduleName]; found {
		return g.modules[moduleIndex].GetInitialBlock(), nil
	}
	return 0, fmt.Errorf("could not find module %s in graph", moduleName)
}

func (g *ModuleGraph) Module(moduleName string) (*pbsubstreams.Module, error) {
	if moduleIndex, found := g.moduleIndex[moduleName]; found {
		return g.modules[moduleIndex], nil
	}
	return nil, fmt.Errorf("could not find module %s in graph", moduleName)
}

type ModuleMarshaler []*pbsubstreams.Module

func (m ModuleMarshaler) MarshalJSON() ([]byte, error) {
	l := make([]string, 0, len(m))
	for _, mod := range m {
		l = append(l, mod.Name)
	}

	return json.Marshal(l)
}

func SortModuleNamesByGraphTopology(mods []string, g *ModuleGraph) []string {
	g.TopologicalSort()

	sort.Slice(mods, func(i, j int) bool {
		return g.moduleIndex[mods[i]] < g.moduleIndex[mods[j]]
	})

	return mods
}

package manifest

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/streamingfast/bstream"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/yourbasic/graph"
	"go.uber.org/zap"
)

type ModuleGraph struct {
	*graph.Mutable

	currentHashesCache map[string][]byte // moduleName => hash

	modules         []*pbsubstreams.Module
	moduleIndex     map[string]int
	indexIndex      map[int]*pbsubstreams.Module
	inputOrderIndex map[string]map[string]int
}

func NewModuleGraph(modules []*pbsubstreams.Module) (*ModuleGraph, error) {
	g := &ModuleGraph{
		Mutable:            graph.New(len(modules)),
		modules:            modules,
		moduleIndex:        make(map[string]int),
		indexIndex:         make(map[int]*pbsubstreams.Module),
		currentHashesCache: make(map[string][]byte),
		inputOrderIndex:    map[string]map[string]int{},
	}

	for i, module := range modules {
		g.moduleIndex[module.Name] = i
		g.indexIndex[i] = module
		g.inputOrderIndex[module.Name] = map[string]int{}
	}

	for i, module := range modules {
		for j, input := range module.Inputs {
			var moduleName string
			if v := input.GetMap(); v != nil {
				moduleName = v.GetModuleName()
			} else if v := input.GetStore(); v != nil {
				moduleName = v.GetModuleName()
			} else if v := input.GetSource(); v != nil {
				moduleName = v.GetType()
			} else if v := input.GetParams(); v != nil {
				moduleName = v.GetValue()
			}

			if moduleName == "" {
				continue
			}

			if j, found := g.moduleIndex[moduleName]; found {
				g.AddCost(i, j, 1)
			}

			g.inputOrderIndex[module.Name][moduleName] = j
		}
	}

	if !graph.Acyclic(g) {
		return nil, fmt.Errorf("modules graph has a cycle")
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

func (g *ModuleGraph) ModuleNameFromIndex(index int) string {
	return g.indexIndex[index].Name
}

func (g *ModuleGraph) ModuleIndexFromName(name string) (int, bool) {
	v, ok := g.moduleIndex[name]
	return v, ok
}

func (g *ModuleGraph) Modules() []string {
	var modules []string
	for _, module := range g.modules {
		modules = append(modules, module.Name)
	}

	SortModuleNamesByGraphTopology(modules, g)

	return modules
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

func (g *ModuleGraph) TopologicalSortKnownModules(known map[string]bool) ([]*pbsubstreams.Module, bool) {
	order, ok := graph.TopSort(g)
	if !ok {
		return nil, ok
	}

	var res []*pbsubstreams.Module
	for _, i := range order {
		if known[g.indexIndex[i].Name] {
			res = append(res, g.indexIndex[i])
		}
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

func (g *ModuleGraph) Context(moduleName string) (parents []string, children []string, err error) {
	// loop over inputs to get parents
	mod, found := g.ModuleIndexFromName(moduleName)
	if !found {
		return nil, nil, fmt.Errorf("could not find module %s in graph", moduleName)
	}

	inputSeen := map[string]bool{}
	for _, input := range g.modules[mod].Inputs {
		if inputSeen[input.Pretty()] {
			continue
		}
		parents = append(parents, input.Pretty())
		inputSeen[input.Pretty()] = true
	}

	for _, m := range g.MustChildrenOf(moduleName) {
		children = append(children, m.Name)
	}

	sort.Strings(children)

	return
}

func (g *ModuleGraph) MustParentsOf(moduleName string) []*pbsubstreams.Module {
	res, err := g.ParentsOf(moduleName)
	if err != nil {
		panic(err)
	}
	return res
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

	sort.Slice(res, func(i, j int) bool {
		return g.inputOrderIndex[moduleName][res[i].Name] < g.inputOrderIndex[moduleName][res[j].Name]
	})

	return res, nil
}

func (g *ModuleGraph) MustChildrenOf(moduleName string) []*pbsubstreams.Module {
	res, err := g.ChildrenOf(moduleName)
	if err != nil {
		panic(err)
	}
	return res
}

func (g *ModuleGraph) ChildrenOf(moduleName string) ([]*pbsubstreams.Module, error) {
	if _, found := g.moduleIndex[moduleName]; !found {
		return nil, fmt.Errorf("could not find module %s in graph", moduleName)
	}

	var res []*pbsubstreams.Module
	resSet := map[string]*pbsubstreams.Module{}
	for _, module := range g.modules {
		_, distances := graph.ShortestPaths(g, g.moduleIndex[module.Name])
		for i, d := range distances {
			if d == 1 {
				if g.indexIndex[i].Name == moduleName {
					resSet[module.Name] = module
				}
			}
		}
	}

	for _, module := range resSet {
		res = append(res, module)
	}

	sortedModules, ok := g.TopologicalSort()
	if !ok {
		return nil, fmt.Errorf("could not get topological sort of module graph")
	}

	topologicalIndex := map[string]int{}

	for i, node := range sortedModules {
		topologicalIndex[node.Name] = i
	}

	sort.Slice(res, func(i, j int) bool {
		return topologicalIndex[res[i].Name] > topologicalIndex[res[j].Name]
	})

	return res, nil
}

func (g *ModuleGraph) HasStatefulDependencies(moduleName string) (bool, error) {
	stores, err := g.StoresDownTo(moduleName)
	if err != nil {
		return false, fmt.Errorf("getting stores down to %s: %w", moduleName, err)
	}

	if len(stores) > 0 {
		return true, nil
	}

	return false, nil
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

//// ArrayLayout returns a 2D array of module indexes. The first layer of the array contains the target module,
//// and each subsequent layer contains the modules that depend on the modules in the previous layer.
//// It also returns a map of module indexes to their coordinates in the array.
//func (g *ModuleGraph) ArrayLayout(targetModule string) ([][]string, map[string][2]int, error) {
//	_, distances := graph.ShortestPaths(g, g.ModuleIndexFromName(targetModule))
//
//	alreadyAdded := map[string]bool{}
//	distanceMap := map[int64][]int{}
//
//	for i, d := range distances {
//		if d < 0 {
//			continue
//		}
//
//		module := g.ModuleNameFromIndex(i)
//		if _, ok := alreadyAdded[module]; ok {
//			continue
//		}
//
//		if distanceMap[d] == nil {
//			distanceMap[d] = []int{}
//		}
//		distanceMap[d] = append(distanceMap[d], i)
//	}
//
//	var distanceKeys []int64
//	for k := range distanceMap {
//		distanceKeys = append(distanceKeys, k)
//	}
//	sort.Slice(distanceKeys, func(i, j int) bool {
//		return distanceKeys[i] < distanceKeys[j]
//	})
//
//	res := make([][]string, len(distanceKeys))
//
//	for i, d := range distanceKeys {
//		tmp := distanceMap[d]
//		strRow := make([]string, len(tmp))
//		for j, idx := range tmp {
//			strRow[j] = g.ModuleNameFromIndex(idx)
//		}
//		res[i] = strRow
//	}
//
//	locationIndex := make(map[string][2]int)
//	for i, col := range res {
//		for j, modIdx := range col {
//			locationIndex[modIdx] = [2]int{i, j}
//		}
//	}
//
//	return res, locationIndex, nil
//}

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

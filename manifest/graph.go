package manifest

import (
	"fmt"
	"github.com/yourbasic/graph"
	"sort"
	"strings"
)

type ModuleGraph struct {
	*graph.Mutable

	modules     []*Module
	moduleIndex map[string]int
	indexIndex  map[int]*Module
}

func NewModuleGraph(modules []*Module) (*ModuleGraph, error) {
	g := &ModuleGraph{
		Mutable:     graph.New(len(modules)),
		modules:     modules,
		moduleIndex: make(map[string]int),
		indexIndex:  make(map[int]*Module),
	}

	for i, module := range modules {
		g.moduleIndex[module.Name] = i
		g.indexIndex[i] = module
	}

	inputNameToModule := func(inputName string) (string, int, bool) {
		var input string
		var found bool
		var ix int

		if strings.HasPrefix(inputName, fmt.Sprintf("%s:", ModuleKindMap)) {
			input = strings.TrimPrefix(inputName, "map:")
		}
		if strings.HasPrefix(inputName, fmt.Sprintf("%s:", ModuleKindStore)) {
			input = strings.TrimPrefix(inputName, "store:")
		}

		ix, found = g.moduleIndex[input]

		return input, ix, found
	}

	for i, module := range modules {
		for _, input := range module.Inputs {
			_, j, found := inputNameToModule(input.Name)
			if !found {
				continue
			}
			g.AddCost(i, j, 1)
		}
	}

	if !graph.Acyclic(g) {
		return nil, fmt.Errorf("modules graph has a cycle")
	}

	return g, nil
}

func (g *ModuleGraph) topSort() ([]*Module, bool) {
	order, ok := graph.TopSort(g)
	if !ok {
		return nil, ok
	}

	var res []*Module
	for _, i := range order {
		res = append(res, g.indexIndex[i])
	}

	return res, ok
}

func (g *ModuleGraph) AncestorsOf(moduleName string) ([]*Module, error) {
	if _, found := g.moduleIndex[moduleName]; !found {
		return nil, fmt.Errorf("could not find module %s in graph", moduleName)
	}

	_, distances := graph.ShortestPaths(g, g.moduleIndex[moduleName])

	var res []*Module
	for i, d := range distances {
		if d >= 1 {
			res = append(res, g.indexIndex[i])
		}
	}

	return res, nil
}

func (g *ModuleGraph) AncestorStoresOf(moduleName string) ([]*Module, error) {
	ancestors, err := g.AncestorsOf(moduleName)
	if err != nil {
		return nil, err
	}

	result := make([]*Module, 0, len(ancestors))
	for _, a := range ancestors {
		if a.Kind == ModuleKindStore {
			result = append(result, a)
		}
	}

	return result, nil
}

func (g *ModuleGraph) ParentsOf(moduleName string) ([]*Module, error) {
	if _, found := g.moduleIndex[moduleName]; !found {
		return nil, fmt.Errorf("could not find module %s in graph", moduleName)
	}

	_, distances := graph.ShortestPaths(g, g.moduleIndex[moduleName])

	var res []*Module
	for i, d := range distances {
		if d == 1 {
			res = append(res, g.indexIndex[i])
		}
	}

	return res, nil
}

func (g *ModuleGraph) StoresDownTo(moduleName string) ([]*Module, error) {
	if _, found := g.moduleIndex[moduleName]; !found {
		return nil, fmt.Errorf("could not find module %s in graph", moduleName)
	}

	_, distances := graph.ShortestPaths(g, g.moduleIndex[moduleName])

	var res []*Module
	for i, d := range distances {
		if d >= 0 { // connected node or myself
			module := g.indexIndex[i]
			if module.Kind == ModuleKindStore {
				res = append(res, g.indexIndex[i])
			}
		}
	}

	return res, nil
}

func (g *ModuleGraph) ModulesDownTo(moduleName string) ([]*Module, error) {
	if _, found := g.moduleIndex[moduleName]; !found {
		return nil, fmt.Errorf("could not find module %s in graph", moduleName)
	}

	_, distances := graph.ShortestPaths(g, g.moduleIndex[moduleName])

	var res []*Module
	for i, d := range distances {
		if d >= 0 { // connected node or myself
			res = append(res, g.indexIndex[i])
		}
	}

	return res, nil
}

func (g *ModuleGraph) GroupedModulesDownTo(moduleName string) ([][]*Module, error) {
	v, found := g.moduleIndex[moduleName]

	if !found {
		return nil, fmt.Errorf("could not find module %s in graph", moduleName)
	}

	mods, err := g.ModulesDownTo(moduleName)
	if err != nil {
		return nil, fmt.Errorf("could not determine dependencies graph for %s: %w", moduleName, err)
	}

	_, dist := graph.ShortestPaths(g, v)

	distmap := map[int][]*Module{}
	distkeys := []int{}
	for _, mod := range mods {
		mix := g.moduleIndex[mod.Name]
		if _, found := distmap[int(dist[mix])]; !found {
			distkeys = append(distkeys, int(dist[mix]))
		}
		distmap[int(dist[mix])] = append(distmap[int(dist[mix])], mod)
	}

	//reverse sort
	sort.Slice(distkeys, func(i, j int) bool {
		return distkeys[j] < distkeys[i]
	})

	res := make([][]*Module, 0, len(distmap))
	for _, ix := range distkeys {
		res = append(res, distmap[ix])
	}

	return res, nil
}

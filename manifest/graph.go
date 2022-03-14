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

		if strings.HasPrefix(inputName, "map:") {
			input = strings.TrimPrefix(inputName, "map:")
		}
		if strings.HasPrefix(inputName, "store:") {
			input = strings.TrimPrefix(inputName, "store:")
		}

		ix, found = g.moduleIndex[input]

		return input, ix, found
	}

	for i, module := range modules {
		for _, input := range module.Inputs {
			inputName, j, found := inputNameToModule(input.Name)
			if !found {
				continue
			}
			fmt.Printf("adding edge from %s (%d) to %s (%d) \n", module.Name, i, inputName, j)
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

	sorted, ok := g.topSort()
	if !ok {
		return nil, fmt.Errorf("could not determine topological sort of graph")
	}

	revSorted := make([]*Module, len(sorted), len(sorted))
	for i, m := range sorted {
		revSorted[len(sorted)-i-1] = m
	}

	var res []*Module
	for _, m := range revSorted {
		if m.Name == moduleName {
			break
		}
		res = append(res, m)
	}

	return res, nil
}

func (g *ModuleGraph) ModulesDownTo(moduleName string) ([]*Module, error) {
	if _, found := g.moduleIndex[moduleName]; !found {
		return nil, fmt.Errorf("could not find module %s in graph", moduleName)
	}

	sorted, ok := g.topSort()
	if !ok {
		return nil, fmt.Errorf("could not determine topological sort of graph")
	}

	revSorted := make([]*Module, len(sorted), len(sorted))
	for i, m := range sorted {
		revSorted[len(sorted)-i-1] = m
	}

	var res []*Module
	for _, m := range revSorted {
		res = append(res, m)
		if m.Name == moduleName {
			break
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

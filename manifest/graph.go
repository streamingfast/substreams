package manifest

import (
	"encoding/json"
	"fmt"
	"math"

	"github.com/streamingfast/bstream"

	pbtransform "github.com/streamingfast/substreams/pb/sf/substreams/transform/v1"
	"github.com/yourbasic/graph"
)

type ModuleGraph struct {
	*graph.Mutable

	modules     []*pbtransform.Module
	moduleIndex map[string]int
	indexIndex  map[int]*pbtransform.Module
}

func NewModuleGraph(modules []*pbtransform.Module) (*ModuleGraph, error) {
	g := &ModuleGraph{
		Mutable:     graph.New(len(modules)),
		modules:     modules,
		moduleIndex: make(map[string]int),
		indexIndex:  make(map[int]*pbtransform.Module),
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

	computeStartBlock(modules, g)

	return g, nil
}

func computeStartBlock(modules []*pbtransform.Module, g *ModuleGraph) {
	for _, module := range modules {
		if module.StartBlock == nil {
			moduleIndex := g.moduleIndex[module.Name]
			startBlock := startBlockForModule(moduleIndex, g, math.MaxUint64)
			module.StartBlock = &startBlock
		}
	}
}

func startBlockForModule(moduleIndex int, g *ModuleGraph, startBlock uint64) uint64 {
	sb := startBlock
	g.Visit(moduleIndex, func(w int, c int64) bool {
		parent := g.modules[w]
		if parent.StartBlock != nil {
			if parent.GetStartBlock() < sb {
				sb = parent.GetStartBlock()
			}
			return false
		}
		return false
	})
	if sb == math.MaxUint64 {
		return bstream.GetProtocolFirstStreamableBlock
	}
	return sb
}

func (g *ModuleGraph) topSort() ([]*pbtransform.Module, bool) {
	order, ok := graph.TopSort(g)
	if !ok {
		return nil, ok
	}

	var res []*pbtransform.Module
	for _, i := range order {
		res = append(res, g.indexIndex[i])
	}

	return res, ok
}

func (g *ModuleGraph) AncestorsOf(moduleName string) ([]*pbtransform.Module, error) {
	if _, found := g.moduleIndex[moduleName]; !found {
		return nil, fmt.Errorf("could not find module %s in graph", moduleName)
	}

	_, distances := graph.ShortestPaths(g, g.moduleIndex[moduleName])

	var res []*pbtransform.Module
	for i, d := range distances {
		if d >= 1 {
			res = append(res, g.indexIndex[i])
		}
	}

	return res, nil
}

func (g *ModuleGraph) AncestorStoresOf(moduleName string) ([]*pbtransform.Module, error) {
	ancestors, err := g.AncestorsOf(moduleName)
	if err != nil {
		return nil, err
	}

	result := make([]*pbtransform.Module, 0, len(ancestors))
	for _, a := range ancestors {
		kind := a.GetKindStore()
		if kind != nil {
			result = append(result, a)
		}
	}

	return result, nil
}

func (g *ModuleGraph) ParentsOf(moduleName string) ([]*pbtransform.Module, error) {
	if _, found := g.moduleIndex[moduleName]; !found {
		return nil, fmt.Errorf("could not find module %s in graph", moduleName)
	}

	_, distances := graph.ShortestPaths(g, g.moduleIndex[moduleName])

	var res []*pbtransform.Module
	for i, d := range distances {
		if d == 1 {
			res = append(res, g.indexIndex[i])
		}
	}

	return res, nil
}

func (g *ModuleGraph) StoresDownTo(moduleName string) ([]*pbtransform.Module, error) {
	if _, found := g.moduleIndex[moduleName]; !found {
		return nil, fmt.Errorf("could not find module %s in graph", moduleName)
	}

	_, distances := graph.ShortestPaths(g, g.moduleIndex[moduleName])

	var res []*pbtransform.Module
	for i, d := range distances {
		if d >= 0 { // connected node or myself
			module := g.indexIndex[i]
			if module.GetKindStore() != nil {
				res = append(res, g.indexIndex[i])
			}
		}
	}

	return res, nil
}

func (g *ModuleGraph) ModulesDownTo(moduleName string) ([]*pbtransform.Module, error) {
	if _, found := g.moduleIndex[moduleName]; !found {
		return nil, fmt.Errorf("could not find module %s in graph", moduleName)
	}

	_, distances := graph.ShortestPaths(g, g.moduleIndex[moduleName])

	var res []*pbtransform.Module
	for i, d := range distances {
		if d >= 0 { // connected node or myself
			res = append(res, g.indexIndex[i])
		}
	}

	return res, nil
}

func (g *ModuleGraph) ModuleStartBlock(moduleName string) (uint64, error) {
	if moduleIndex, found := g.moduleIndex[moduleName]; found {
		return g.modules[moduleIndex].GetStartBlock(), nil
	}
	return 0, fmt.Errorf("could not find module %s in graph", moduleName)
}

type ModuleMarshaler []*pbtransform.Module

func (m ModuleMarshaler) MarshalJSON() ([]byte, error) {
	l := make([]string, 0, len(m))
	for _, mod := range m {
		l = append(l, mod.Name)
	}

	return json.Marshal(l)
}

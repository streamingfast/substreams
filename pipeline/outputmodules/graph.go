package outputmodules

import (
	"fmt"

	"github.com/streamingfast/substreams/manifest"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

type Graph struct {
	requestModules    *pbsubstreams.Modules
	usedModules       []*pbsubstreams.Module // all modules that need to be processed (requested directly or a required module ancestor)
	stagedUsedModules ExecutionStages        // all modules that need to be processed (requested directly or a required module ancestor)
	moduleHashes      *manifest.ModuleHashes
	stores            []*pbsubstreams.Module // subset of allModules: only the stores
	lowestInitBlock   uint64

	outputModule *pbsubstreams.Module

	schedulableModules      []*pbsubstreams.Module // stores and output mappers needed to execute to produce output for all `output_modules`.
	schedulableAncestorsMap map[string][]string    // modules that are ancestors (therefore dependencies) of a given module
}

func (g *Graph) OutputModule() *pbsubstreams.Module  { return g.outputModule }
func (g *Graph) Stores() []*pbsubstreams.Module      { return g.stores }
func (g *Graph) UsedModules() []*pbsubstreams.Module { return g.usedModules }
func (g *Graph) UsedModulesUpToStage(stage int) (out []*pbsubstreams.Module) {
	for i := 0; i <= int(stage); i++ {
		for _, layer := range g.StagedUsedModules()[i] {
			for _, mod := range layer {
				out = append(out, mod)
			}
		}
	}
	return
}
func (g *Graph) StagedUsedModules() ExecutionStages   { return g.stagedUsedModules }
func (g *Graph) IsOutputModule(name string) bool      { return g.outputModule.Name == name }
func (g *Graph) ModuleHashes() *manifest.ModuleHashes { return g.moduleHashes }
func (g *Graph) LowestInitBlock() uint64              { return g.lowestInitBlock }

func NewOutputModuleGraph(outputModule string, productionMode bool, modules *pbsubstreams.Modules) (out *Graph, err error) {
	out = &Graph{
		requestModules: modules,
	}
	if err := out.computeGraph(outputModule, productionMode, modules); err != nil {
		return nil, fmt.Errorf("module graph: %w", err)
	}

	return out, nil
}

func (g *Graph) computeGraph(outputModule string, productionMode bool, modules *pbsubstreams.Modules) error {
	graph, err := manifest.NewModuleGraph(modules.Modules)
	if err != nil {
		return fmt.Errorf("compute graph: %w", err)
	}
	outputModuleName := outputModule

	processModules, err := graph.ModulesDownTo(outputModuleName)
	if err != nil {
		return fmt.Errorf("building execution moduleGraph: %w", err)
	}
	g.usedModules = processModules
	g.stagedUsedModules = computeStages(processModules)
	g.lowestInitBlock = computeLowestInitBlock(processModules)

	if err := g.hashModules(graph); err != nil {
		return fmt.Errorf("cannot hash module: %w", err)
	}

	g.outputModule = computeOutputModule(g.usedModules, outputModuleName)

	storeModules, err := graph.StoresDownTo(g.outputModule.Name)
	if err != nil {
		return fmt.Errorf("stores down: %w", err)
	}
	g.stores = storeModules

	g.schedulableModules = computeSchedulableModules(storeModules, g.outputModule, productionMode)

	ancestorsMap, err := computeSchedulableAncestors(graph, g.schedulableModules)
	if err != nil {
		return fmt.Errorf("computing ancestors: %w", err)
	}
	g.schedulableAncestorsMap = ancestorsMap

	return nil
}

func computeLowestInitBlock(modules []*pbsubstreams.Module) (out uint64) {
	lowest := modules[0].InitialBlock
	for _, mod := range modules {
		if mod.InitialBlock < lowest {
			lowest = mod.InitialBlock
		}
	}
	return lowest
}

// A list of units that we can schedule, that might include some mappers and a store,
// or the last module could be an exeuction layer with only a map.
type ExecutionStages []StageLayers

func (e ExecutionStages) LastStage() StageLayers {
	return e[len(e)-1]
}

// For a given execution stage, the layers of execution, for example:
// a layer of mappers, followed by a layer of stores.
type StageLayers []LayerModules

func (l StageLayers) IsLastStage() bool {
	return !l.LastLayer().IsStoreLayer()
}

func (l StageLayers) LastLayer() LayerModules {
	return l[len(l)-1]
}

// The list of modules in a given layer of either maps or stores. A given layer
// will always be comprised of only the same kind of modules.
type LayerModules []*pbsubstreams.Module

func (l LayerModules) IsStoreLayer() bool {
	return l[0].GetKindStore() != nil
}

func computeStages(mods []*pbsubstreams.Module) (stages ExecutionStages) {
	seen := map[string]bool{}

	var layers StageLayers

	for i := 0; ; i++ {
		if len(seen) == len(mods) {
			break
		}
		var layer LayerModules
	modLoop:
		for _, mod := range mods {
			switch mod.Kind.(type) {
			case *pbsubstreams.Module_KindMap_:
				if i%2 == 0 {
					continue
				}
			case *pbsubstreams.Module_KindStore_:
				if i%2 == 1 {
					continue
				}
			}

			if seen[mod.Name] {
				continue
			}

			for _, dep := range mod.Inputs {
				var depModName string
				switch input := dep.Input.(type) {
				case *pbsubstreams.Module_Input_Params_:
					continue
				case *pbsubstreams.Module_Input_Source_:
					continue
				case *pbsubstreams.Module_Input_Map_:
					depModName = input.Map.ModuleName
				case *pbsubstreams.Module_Input_Store_:
					depModName = input.Store.ModuleName
				default:
					panic(fmt.Errorf("unsupported input type %T", dep.Input))
				}
				if !seen[depModName] {
					continue modLoop
				}
			}

			layer = append(layer, mod)
		}
		if len(layer) != 0 {
			layers = append(layers, layer)
			for _, mod := range layer {
				seen[mod.Name] = true
			}
		}
	}

	lastLayerIndex := len(layers) - 1
	var newStage StageLayers
	for idx, layer := range layers {
		isLastStage := idx == lastLayerIndex
		isStoreLayer := layer.IsStoreLayer()

		newStage = append(newStage, layer)
		if isStoreLayer || isLastStage {
			stages = append(stages, newStage)
			newStage = nil
		}
	}

	return stages
}

func computeOutputModule(mods []*pbsubstreams.Module, outputModule string) *pbsubstreams.Module {
	for _, module := range mods {
		if module.Name == outputModule {
			return module
		}
	}
	panic(fmt.Errorf("unable to find output module %q in modules list", outputModule))

}

func computeSchedulableModules(stores []*pbsubstreams.Module, outputModule *pbsubstreams.Module, productionMode bool) []*pbsubstreams.Module {
	if !productionMode { // dev never schedules maps, all stores are in there
		return stores
	}

	if outputModule.GetKindStore() != nil {
		return stores
	}

	return append(stores, outputModule)
}

func computeSchedulableAncestors(graph *manifest.ModuleGraph, schedulableModules []*pbsubstreams.Module) (out map[string][]string, err error) {
	out = map[string][]string{}
	for _, mod := range schedulableModules {
		ancestors, err := graph.AncestorStoresOf(mod.Name)
		if err != nil {
			return nil, fmt.Errorf("getting ancestor stores for module %s: %w", mod.Name, err)
		}
		out[mod.Name] = moduleNames(ancestors)
	}
	return out, nil
}

func (g *Graph) SchedulableModuleNames() []string {
	return moduleNames(g.schedulableModules)
}

func (g *Graph) AncestorsFrom(moduleName string) []string {
	return g.schedulableAncestorsMap[moduleName]
}

func moduleNames(modules []*pbsubstreams.Module) (out []string) {
	for _, mod := range modules {
		out = append(out, mod.Name)
	}
	return
}

func (g *Graph) hashModules(graph *manifest.ModuleGraph) error {
	g.moduleHashes = manifest.NewModuleHashes()
	for _, module := range g.usedModules {
		if _, err := g.moduleHashes.HashModule(g.requestModules, module, graph); err != nil {
			return err
		}
	}
	return nil
}

func (g *Graph) ValidateRequestStartBlock(requestStartBlockNum uint64) error {
	if requestStartBlockNum < g.outputModule.InitialBlock {
		return fmt.Errorf("start block %d smaller than request outputs for module %q with start block %d", requestStartBlockNum, g.outputModule.Name, g.outputModule.InitialBlock)
	}
	return nil
}

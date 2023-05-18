package outputmodules

import (
	"fmt"

	"github.com/streamingfast/substreams/manifest"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

type Graph struct {
	requestModules    *pbsubstreams.Modules
	usedModules       []*pbsubstreams.Module   // all modules that need to be processed (requested directly or a required module ancestor)
	stagedUsedModules [][]*pbsubstreams.Module // all modules that need to be processed (requested directly or a required module ancestor)
	moduleHashes      *manifest.ModuleHashes
	stores            []*pbsubstreams.Module // subset of allModules: only the stores

	outputModule *pbsubstreams.Module

	schedulableModules      []*pbsubstreams.Module // stores and output mappers needed to execute to produce output for all `output_modules`.
	schedulableAncestorsMap map[string][]string    // modules that are ancestors (therefore dependencies) of a given module
}

func (g *Graph) OutputModule() *pbsubstreams.Module          { return g.outputModule }
func (g *Graph) Stores() []*pbsubstreams.Module              { return g.stores }
func (g *Graph) UsedModules() []*pbsubstreams.Module         { return g.usedModules }
func (g *Graph) StagedUsedModules() [][]*pbsubstreams.Module { return g.stagedUsedModules }
func (g *Graph) IsOutputModule(name string) bool             { return g.outputModule.Name == name }
func (g *Graph) ModuleHashes() *manifest.ModuleHashes        { return g.moduleHashes }

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

func computeStages(mods []*pbsubstreams.Module) (stages [][]*pbsubstreams.Module) {
	seen := map[string]bool{}

	for i := 0; ; i++ {
		if len(seen) == len(mods) {
			break
		}
		var stage []*pbsubstreams.Module
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

			stage = append(stage, mod)
		}
		if len(stage) != 0 {
			stages = append(stages, stage)
			for _, mod := range stage {
				seen[mod.Name] = true
			}
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

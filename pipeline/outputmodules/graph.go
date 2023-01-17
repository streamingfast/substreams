package outputmodules

import (
	"fmt"

	"github.com/streamingfast/substreams/manifest"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

type Graph struct {
	request *pbsubstreams.Request

	allModules   []*pbsubstreams.Module // all modules that need to be processed (requested directly or a required module ancestor)
	moduleHashes *manifest.ModuleHashes
	stores       []*pbsubstreams.Module // subset of allModules: only the stores

	outputModule *pbsubstreams.Module

	schedulableModules      []*pbsubstreams.Module // stores and output mappers needed to execute to produce output for all `output_modules`.
	schedulableAncestorsMap map[string][]string    // modules that are ancestors (therefore dependencies) of a given module
}

func (g *Graph) OutputModule() *pbsubstreams.Module   { return g.outputModule }
func (g *Graph) Stores() []*pbsubstreams.Module       { return g.stores }
func (g *Graph) AllModules() []*pbsubstreams.Module   { return g.allModules }
func (g *Graph) IsOutputModule(name string) bool      { return g.outputModule.Name == name }
func (g *Graph) ModuleHashes() *manifest.ModuleHashes { return g.moduleHashes }

func NewOutputModuleGraph(request *pbsubstreams.Request) (out *Graph, err error) {
	out = &Graph{
		request: request,
	}
	if err := out.computeGraph(); err != nil {
		return nil, fmt.Errorf("module graph: %w", err)
	}

	return out, nil
}

func (g *Graph) computeGraph() error {
	graph, err := manifest.NewModuleGraph(g.request.Modules.Modules)
	if err != nil {
		return fmt.Errorf("compute graph: %w", err)
	}
	outputModuleName := g.request.MustGetOutputModuleName()

	processModules, err := graph.ModulesDownTo(outputModuleName)
	if err != nil {
		return fmt.Errorf("building execution moduleGraph: %w", err)
	}
	g.allModules = processModules
	g.hashModules(graph)

	g.outputModule = computeOutputModule(g.allModules, outputModuleName)

	storeModules, err := graph.StoresDownTo(g.outputModule.Name)
	if err != nil {
		return fmt.Errorf("stores down: %w", err)
	}
	g.stores = storeModules

	g.schedulableModules = computeSchedulableModules(storeModules, g.outputModule, g.request.ProductionMode)

	ancestorsMap, err := computeSchedulableAncestors(graph, g.schedulableModules)
	if err != nil {
		return fmt.Errorf("computing ancestors: %w", err)
	}
	g.schedulableAncestorsMap = ancestorsMap

	return nil
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

func (g *Graph) hashModules(graph *manifest.ModuleGraph) {
	g.moduleHashes = manifest.NewModuleHashes()
	for _, module := range g.allModules {
		g.moduleHashes.HashModule(g.request.Modules, module, graph)
	}
}

func (g *Graph) ValidateRequestStartBlock(requestStartBlockNum uint64) error {
	if requestStartBlockNum < g.outputModule.InitialBlock {
		return fmt.Errorf("start block %d smaller than request outputs for module %q with start block %d", requestStartBlockNum, g.outputModule.Name, g.outputModule.InitialBlock)
	}
	return nil
}

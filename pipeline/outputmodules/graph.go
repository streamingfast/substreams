package outputmodules

import (
	"fmt"

	"github.com/streamingfast/substreams/manifest"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

type Graph struct {
	request *pbsubstreams.Request

	stores                  []*pbsubstreams.Module // stores that need to be processed, either requested directly of as ancestor to a required module.
	requestedMappers        []*pbsubstreams.Module
	schedulableModules      []*pbsubstreams.Module // stores and output mappers needed to execute to produce output for all `output_modules`.
	allModules              []*pbsubstreams.Module // subset of request.Modules, needed for any `OutputModules`.
	requestedOutputs        []*pbsubstreams.Module // modules requested in `OutputModules`
	outputModuleMap         map[string]bool
	schedulableAncestorsMap map[string][]string // modules that are ancestors (therefore dependencies) of a given module
	moduleHashes            *manifest.ModuleHashes
}

func (t *Graph) RequestedMapperModules() []*pbsubstreams.Module { return t.requestedMappers }
func (t *Graph) RequestedMapperModulesMap() map[string]bool {
	out := make(map[string]bool)
	for _, mod := range t.RequestedMapperModules() {
		out[mod.Name] = true
	}
	return out
}
func (t *Graph) Stores() []*pbsubstreams.Module       { return t.stores }
func (g *Graph) AllModules() []*pbsubstreams.Module   { return g.allModules }
func (t *Graph) IsOutputModule(name string) bool      { return t.outputModuleMap[name] }
func (t *Graph) OutputMap() map[string]bool           { return t.outputModuleMap }
func (t *Graph) ModuleHashes() *manifest.ModuleHashes { return t.moduleHashes }

func NewOutputModuleGraph(request *pbsubstreams.Request) (out *Graph, err error) {
	outMap := make(map[string]bool)
	for _, name := range request.OutputModules {
		outMap[name] = true
	}
	out = &Graph{
		request:         request,
		outputModuleMap: outMap,
	}
	if err := out.computeGraph(); err != nil {
		return nil, fmt.Errorf("module graph: %w", err)
	}

	return out, nil
}

func (t *Graph) computeGraph() error {
	graph, err := manifest.NewModuleGraph(t.request.Modules.Modules)
	if err != nil {
		return fmt.Errorf("compute graph: %w", err)
	}

	processModules, err := graph.ModulesDownTo(t.request.OutputModules)
	if err != nil {
		return fmt.Errorf("building execution moduleGraph: %w", err)
	}
	t.allModules = processModules
	t.hashModules(graph)

	storeModules, err := graph.StoresDownTo(t.request.OutputModules)
	if err != nil {
		return fmt.Errorf("stores down: %w", err)
	}
	t.stores = storeModules

	t.requestedOutputs = computeOutputModules(t.allModules, t.outputModuleMap)
	t.requestedMappers = computeRequestedMappers(t.allModules, t.outputModuleMap)
	t.schedulableModules = computeSchedulableModules(storeModules, t.requestedMappers)

	ancestorsMap, err := computeSchedulableAncestors(graph, t.schedulableModules)
	if err != nil {
		return fmt.Errorf("computing ancestors: %w", err)
	}
	t.schedulableAncestorsMap = ancestorsMap

	return nil
}

func computeOutputModules(mods []*pbsubstreams.Module, outMap map[string]bool) (out []*pbsubstreams.Module) {
	for _, module := range mods {
		isOutput := outMap[module.Name]
		if isOutput {
			out = append(out, module)
		}
	}
	if len(outMap) != len(out) {
		panic(fmt.Errorf("inconsistent output modules and output modules map: %d and %d", len(out), len(outMap)))
	}
	return
}

func computeRequestedMappers(mods []*pbsubstreams.Module, outMap map[string]bool) (out []*pbsubstreams.Module) {
	for _, module := range mods {
		isOutput := outMap[module.Name]
		if isOutput && module.GetKindMap() != nil {
			out = append(out, module)
		}
	}
	return
}

func computeSchedulableModules(stores []*pbsubstreams.Module, requestedMappers []*pbsubstreams.Module) (out []*pbsubstreams.Module) {
	return append(append(out, stores...), requestedMappers...)
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

func (t *Graph) hashModules(graph *manifest.ModuleGraph) {
	t.moduleHashes = manifest.NewModuleHashes()
	for _, module := range t.allModules {
		t.moduleHashes.HashModule(t.request.Modules, module, graph)
	}
}

func (t *Graph) ValidateRequestStartBlock(requestStartBlockNum uint64) error {
	for _, module := range t.requestedOutputs {
		if requestStartBlockNum < module.InitialBlock {
			return fmt.Errorf("start block %d smaller than request outputs for module %q with start block %d", requestStartBlockNum, module.Name, module.InitialBlock)
		}
	}
	return nil
}

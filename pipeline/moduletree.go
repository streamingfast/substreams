package pipeline

import (
	"fmt"
	"github.com/streamingfast/substreams/manifest"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/pipeline/exec"
)

type ModuleTree struct {
	request *pbsubstreams.Request

	storeModules    []*pbsubstreams.Module
	processModules  []*pbsubstreams.Module // only those needed to output `OutputModules`, a subset of the `request.Modules`
	outputModuleMap map[string]bool

	graph        *manifest.ModuleGraph
	moduleHashes *manifest.ModuleHashes

	moduleExecutorsInitialized bool
	moduleExecutors            []exec.ModuleExecutor
}

func NewModuleTree(request *pbsubstreams.Request, blockType string) (out *ModuleTree, err error) {
	outMap := make(map[string]bool)
	for _, name := range request.OutputModules {
		outMap[name] = true
	}
	out = &ModuleTree{
		request:         request,
		outputModuleMap: outMap,
	}
	if err := validateRequest(request, blockType); err != nil {
		return nil, fmt.Errorf("validate request: %w", err)
	}
	if err := out.computeGraph(); err != nil {
		return nil, fmt.Errorf("compute graph: %w", err)
	}
	out.hashModules()

	return out, nil
}

func validateRequest(request *pbsubstreams.Request, blockType string) error {
	if request.StartBlockNum < 0 {
		// TODO(abourget) start block resolving is an art, it should be handled here
		return fmt.Errorf("negative start block %d is not accepted", request.StartBlockNum)
	}

	if request.Modules == nil {
		return fmt.Errorf("no modules found in request")
	}

	if err := manifest.ValidateModules(request.Modules); err != nil {
		return fmt.Errorf("modules validation failed: %w", err)
	}

	if err := pbsubstreams.ValidateRequest(request); err != nil {
		return fmt.Errorf("validate request: %s", err)
	}

	for _, binary := range request.Modules.Binaries {
		if binary.Type != "wasm/rust-v1" {
			return fmt.Errorf(`unsupported binary type: %q, please use "wasm/rust-v1"`, binary.Type)
		}
	}

	for _, mod := range request.Modules.Modules {
		for _, input := range mod.Inputs {
			if src := input.GetSource(); src != nil {
				if src.Type != blockType && src.Type != "sf.substreams.v1.Clock" {
					return fmt.Errorf("input source %q not supported, only %q and 'sf.substreams.v1.Clock' are valid", src, blockType)
				}
			}
		}
	}

	return nil
}

func (t *ModuleTree) computeGraph() error {
	graph, err := manifest.NewModuleGraph(t.request.Modules.Modules)
	if err != nil {
		return fmt.Errorf("compute graph: %w", err)
	}
	t.graph = graph

	processModules, err := t.graph.ModulesDownTo(t.request.OutputModules)
	if err != nil {
		return fmt.Errorf("building execution moduleGraph: %w", err)
	}
	t.processModules = processModules
	t.hashModules()

	storeModules, err := t.graph.StoresDownTo(t.request.OutputModules)
	if err != nil {
		return err
	}
	t.storeModules = storeModules

	return nil
}

func (t *ModuleTree) hashModules() {
	t.moduleHashes = manifest.NewModuleHashes()
	for _, module := range t.processModules {
		t.moduleHashes.HashModule(t.request.Modules, module, t.graph)
	}
}

func (t *ModuleTree) ValidateEffectiveStartBlock(effectiveStartBlockNum uint64) error {
	for _, module := range t.processModules {
		isOutput := t.outputModuleMap[module.Name]
		if isOutput && effectiveStartBlockNum < module.InitialBlock {
			return fmt.Errorf("start block %d smaller than request outputs for module %q with start block %d", effectiveStartBlockNum, module.Name, module.InitialBlock)
		}
	}
	return nil
}

func (t *ModuleTree) IsOutputModule(name string) bool {
	return t.outputModuleMap[name]
}

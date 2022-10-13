package service

import (
	"fmt"
	"github.com/streamingfast/substreams/manifest"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

func validateGraph(request *pbsubstreams.Request, blockType string) (*manifest.ModuleGraph, error) {
	if request.StartBlockNum < 0 {
		// TODO(abourget) start block resolving is an art, it should be handled here
		return nil, fmt.Errorf("invalid negative startblock (not handled in substreams): %d", request.StartBlockNum)
	}

	if request.Modules == nil {
		return nil, fmt.Errorf("no modules found in request")
	}

	if err := manifest.ValidateModules(request.Modules); err != nil {
		return nil, fmt.Errorf("modules validation failed: %w", err)
	}

	if err := pbsubstreams.ValidateRequest(request); err != nil {
		return nil, fmt.Errorf("validate request: %s", err)
	}

	graph, err := manifest.NewModuleGraph(request.Modules.Modules)
	if err != nil {
		return nil, fmt.Errorf("creating module graph from request: %s", err)
	}

	sources := graph.GetSources()
	for _, source := range sources {
		if source != blockType && source != "sf.substreams.v1.Clock" {
			return nil, fmt.Errorf("input source %q not supported, only %q and 'sf.substreams.v1.Clock' are valid", source, blockType)
		}
	}
	return graph, nil
}

package outputmodules

import (
	"fmt"
	"github.com/streamingfast/substreams/manifest"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

func ValidateRequest(request *pbsubstreams.Request, blockType string, isSubRquest bool) error {
	if request.StartBlockNum < 0 {
		// TODO(abourget): remove this check once we support StartBlockNum being negative
		return fmt.Errorf("negative start block %d is not accepted", request.StartBlockNum)
	}

	if request.Modules == nil {
		return fmt.Errorf("no modules found in request")
	}

	if err := validateOutputModule(request); err != nil {
		return fmt.Errorf("output module: %w", err)
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
		// We want to make sure thate the outputModule is a map for None Sub-Request
		if !isSubRquest {
			if mod.Name == request.GetOutputModuleName() {
				if _, ok := mod.Kind.(*pbsubstreams.Module_KindMap_); !ok {
					return fmt.Errorf("the output module specified must be of kind 'map'")
				}
			}
		}

	}

	return nil
}

func validateOutputModule(request *pbsubstreams.Request) error {
	if request.OutputModule != "" {
		return nil
	}
	outputCount := len(request.OutputModules)
	if outputCount == 0 {
		return fmt.Errorf("no output module found in request")
	}
	if outputCount > 1 {
		return fmt.Errorf("multiple output modules is not accepted")
	}
	return nil
}

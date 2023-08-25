package manifest

import (
	"encoding/base64"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"google.golang.org/protobuf/types/known/anypb"
	"io"
	"strings"
)

type ConfigurationOverride struct {
	Package       *PackageOverride  `yaml:"package,omitempty"`
	Network       string            `yaml:"network,omitempty"`
	InitialBlocks map[string]int64  `yaml:"initialBlocks,omitempty"`
	Params        map[string]string `yaml:"params,omitempty"`

	SinkConfig *SinkConfigOverride `yaml:"sinkConfig,omitempty"`
	SinkModule string              `yaml:"sinkModule,omitempty"`
}

type SinkConfigOverride struct {
	TypeUrl string `yaml:"typeUrl,omitempty"`
	Value   string `yaml:"value,omitempty"`
}

type PackageOverride struct {
	Name string `yaml:"name,omitempty"`
}

func mergeManifests(main *pbsubstreams.Package, override *ConfigurationOverride) error {
	if override.Package != nil && override.Package.Name != "" {
		if main.PackageMeta == nil {
			main.PackageMeta = []*pbsubstreams.PackageMetadata{}
		}

		if len(main.PackageMeta) == 0 {
			main.PackageMeta = append(main.PackageMeta, &pbsubstreams.PackageMetadata{Name: override.Package.Name})
		} else {
			main.PackageMeta[0].Name = override.Package.Name
		}
	}

	if override.Network != "" {
		main.Network = override.Network
	}

	if override.Params != nil {
		mergeParams(main, override)
	}

	if override.InitialBlocks != nil {
		mergeInitialBlocks(main, override)
	}

	// Overriding SinkModule
	if override.Package != nil && override.SinkModule != "" {
		main.SinkModule = override.SinkModule
	}

	// Overriding SinkConfig
	if override.Package != nil && override.SinkConfig != nil {
		decoder := base64.NewDecoder(base64.StdEncoding, strings.NewReader(string(override.SinkConfig.Value)))
		configValueBytes, err := io.ReadAll(decoder)
		if err != nil {
			return err
		}

		main.SinkConfig = &anypb.Any{
			TypeUrl: override.SinkConfig.TypeUrl,
			Value:   configValueBytes,
		}
	}

	return nil
}

func mergeInitialBlocks(main *pbsubstreams.Package, override *ConfigurationOverride) {
	if override.InitialBlocks == nil {
		return
	}

	mainModulesMap := make(map[string]*pbsubstreams.Module)
	for _, mod := range main.Modules.Modules {
		mainModulesMap[mod.Name] = mod
	}

	for name, block := range override.InitialBlocks {
		if mainMod, exists := mainModulesMap[name]; exists {
			mainMod.InitialBlock = uint64(block)
		}
	}
}

func mergeParams(main *pbsubstreams.Package, override *ConfigurationOverride) {
	if override.Params == nil {
		return
	}

	mainModulesMap := make(map[string]*pbsubstreams.Module)
	for _, mod := range main.Modules.Modules {
		mainModulesMap[mod.Name] = mod
	}

	for name, param := range override.Params {
		if mainMod, exists := mainModulesMap[name]; exists {
			mainmodInputs := mainMod.GetInputs()
			if mainmodInputs == nil || len(mainmodInputs) == 0 {
				continue
			}

			mainmodInputFirst := mainmodInputs[0]
			if mainmodInputFirst.GetParams() == nil {
				continue
			}

			newInput := &pbsubstreams.Module_Input{Input: &pbsubstreams.Module_Input_Params_{Params: &pbsubstreams.Module_Input_Params{Value: param}}}
			mainmodInputs[0] = newInput

			mainMod.Inputs = mainmodInputs
		}
	}

}

func mergeOverrides(overrides ...*ConfigurationOverride) *ConfigurationOverride {
	merged := &ConfigurationOverride{}

	for _, override := range overrides {
		if override == nil {
			continue
		}

		if override.Package != nil {
			merged.Package = override.Package
		}

		if override.Network != "" {
			merged.Network = override.Network
		}

		if override.InitialBlocks != nil {
			if merged.InitialBlocks == nil {
				merged.InitialBlocks = make(map[string]int64)
			}

			for name, block := range override.InitialBlocks {
				merged.InitialBlocks[name] = block
			}
		}

		if override.Params != nil {
			if merged.Params == nil {
				merged.Params = make(map[string]string)
			}

			for name, value := range override.Params {
				merged.Params[name] = value
			}
		}

		if override.SinkConfig != nil {
			merged.SinkConfig = override.SinkConfig
		}

		if override.SinkModule != "" {
			merged.SinkModule = override.SinkModule
		}
	}

	return merged
}

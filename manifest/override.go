package manifest

import (
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

type ConfigurationOverride struct {
	Package       *PackageOverride  `yaml:"package,omitempty"`
	Network       *string           `yaml:"network,omitempty"`
	InitialBlocks map[string]uint64 `yaml:"initialBlocks,omitempty"`
	Params        map[string]string `yaml:"params,omitempty"`

	DeriveFrom string `yaml:"deriveFrom,omitempty"`
}

type PackageOverride struct {
	Name    *string `yaml:"name,omitempty"`
	Version *string `yaml:"version,omitempty"`
}

func mergeOverrides(overrides ...*ConfigurationOverride) *ConfigurationOverride {
	var merged *ConfigurationOverride

	for _, override := range overrides {
		if override == nil {
			continue
		}

		if merged == nil {
			merged = &ConfigurationOverride{}
		}

		if override.Package != nil {
			if merged.Package == nil {
				merged.Package = &PackageOverride{}
			}

			if override.Package.Name != nil {
				merged.Package.Name = override.Package.Name
			}

			if override.Package.Version != nil {
				merged.Package.Version = override.Package.Version
			}
		}

		if override.Network != nil {
			merged.Network = override.Network
		}

		if override.InitialBlocks != nil {
			if merged.InitialBlocks == nil {
				merged.InitialBlocks = make(map[string]uint64)
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
	}

	return merged
}

func applyOverride(main *pbsubstreams.Package, override *ConfigurationOverride) error {
	if override == nil {
		return nil
	}

	if override.Package != nil {
		mergePackageMeta(main, override)
	}

	if override.Network != nil {
		main.Network = *override.Network
	}

	if override.Params != nil {
		mergeParams(main, override)
	}

	if override.InitialBlocks != nil {
		mergeInitialBlocks(main, override)
	}

	return nil
}

func mergePackageMeta(main *pbsubstreams.Package, override *ConfigurationOverride) {
	if override.Package == nil {
		return
	}

	currentPackageMeta := main.GetPackageMeta()
	if currentPackageMeta == nil {
		currentPackageMeta = []*pbsubstreams.PackageMetadata{}
	}
	if len(currentPackageMeta) == 0 {
		currentPackageMeta = append(currentPackageMeta, &pbsubstreams.PackageMetadata{})
	}

	if override.Package.Name != nil {
		currentPackageMeta[0].Name = *override.Package.Name
	}

	if override.Package.Version != nil {
		currentPackageMeta[0].Version = *override.Package.Version
	}
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

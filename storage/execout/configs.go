package execout

import (
	"fmt"

	"github.com/streamingfast/dstore"
	"github.com/streamingfast/substreams/manifest"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

type Configs struct {
	configMap              map[string]*Config
	execOutputSaveInterval uint64
}

func NewConfigs(execOutputSaveInterval uint64, confMap map[string]*Config) *Configs {
	return &Configs{
		execOutputSaveInterval: execOutputSaveInterval,
		configMap:              confMap,
	}
}

func NewConfigMap(baseObjectStore dstore.Store, allRequestedModules []*pbsubstreams.Module, moduleHashes *manifest.ModuleHashes) (out map[string]*Config, err error) {
	out = make(map[string]*Config)
	for _, mod := range allRequestedModules {
		conf, err := NewConfig(
			mod.Name,
			mod.InitialBlock,
			moduleHashes.Get(mod.Name),
			baseObjectStore,
		)
		if err != nil {
			return nil, fmt.Errorf("new exec output config for %q: %w", mod.Name, err)
		}
		out[mod.Name] = conf
	}
	return out, nil
}

package execout

import (
	"fmt"

	"go.uber.org/zap"

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

func (c *Configs) NewFiles(logger *zap.Logger) map[string]*File {
	out := make(map[string]*File)
	for modName, config := range c.configMap {
		out[modName] = config.NewFile(c.execOutputSaveInterval, logger)
	}
	return out
}

func (c *Configs) NewFile(moduleName string, logger *zap.Logger) *File {
	return c.configMap[moduleName].NewFile(c.execOutputSaveInterval, logger)
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

package execout

import (
	"fmt"

	"github.com/streamingfast/substreams/block"

	"go.uber.org/zap"

	"github.com/streamingfast/dstore"
	"github.com/streamingfast/substreams/manifest"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

type Configs struct {
	ConfigMap              map[string]*Config
	execOutputSaveInterval uint64
	logger                 *zap.Logger
}

func NewConfigs(execOutputSaveInterval uint64, confMap map[string]*Config, logger *zap.Logger) *Configs {
	return &Configs{
		execOutputSaveInterval: execOutputSaveInterval,
		ConfigMap:              confMap,
		logger:                 logger,
	}
}

func (c *Configs) NewFile(moduleName string, targetRange *block.BoundedRange) *File {
	return c.ConfigMap[moduleName].NewFile(targetRange)
}

func NewConfigMap(baseObjectStore dstore.Store, allRequestedModules []*pbsubstreams.Module, moduleHashes *manifest.ModuleHashes, logger *zap.Logger) (out map[string]*Config, err error) {
	out = make(map[string]*Config)
	for _, mod := range allRequestedModules {
		conf, err := NewConfig(
			mod.Name,
			mod.InitialBlock,
			mod.ModuleKind(),
			moduleHashes.Get(mod.Name),
			baseObjectStore,
			logger,
		)
		if err != nil {
			return nil, fmt.Errorf("new exec output config for %q: %w", mod.Name, err)
		}
		out[mod.Name] = conf
	}
	return out, nil
}

package execout

import (
	"fmt"

	"github.com/streamingfast/dstore"
	"github.com/streamingfast/substreams/block"
	"github.com/streamingfast/substreams/manifest"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"go.uber.org/zap"
)

type Configs struct {
	ConfigMap              map[string]*Config
	execOutputSaveInterval uint64
	logger                 *zap.Logger
}

func NewConfigs(baseObjectStore dstore.Store, allRequestedModules []*pbsubstreams.Module, moduleHashes map[string]string, execOutputSaveInterval uint64, firstStreamableBlock uint64, logger *zap.Logger) (*Configs, error) {
	out := make(map[string]*Config)
	for _, mod := range allRequestedModules {

		modKind := mod.ModuleKind()
		if modKind == pbsubstreams.ModuleKindInvalid {
			return nil, fmt.Errorf("invalid module kind for %q", mod.Name)
		}

		initialBlock := max(firstStreamableBlock, mod.InitialBlock)
		hash := moduleHashes[mod.Name]
		extendedHash := manifest.ExtendedModuleHash(mod, hash)
		conf, err := NewConfig(
			mod.Name,
			initialBlock,
			modKind,
			hash,
			extendedHash,
			baseObjectStore,
			logger,
		)
		if err != nil {
			return nil, fmt.Errorf("new exec output config for %q: %w", mod.Name, err)
		}
		out[mod.Name] = conf
	}

	return &Configs{
		execOutputSaveInterval: execOutputSaveInterval,
		ConfigMap:              out,
		logger:                 logger,
	}, nil
}

func WrapConfigs(execOutputSaveInterval uint64, logger *zap.Logger, confs ...*Config) *Configs {
	out := &Configs{
		execOutputSaveInterval: execOutputSaveInterval,
		logger:                 logger,
		ConfigMap:              make(map[string]*Config),
	}
	for _, conf := range confs {
		out.ConfigMap[conf.name] = conf
	}
	return out
}

func (c *Configs) NewFile(moduleName string, targetRange *block.Range) *File {
	return c.ConfigMap[moduleName].NewFile(targetRange)
}

func (c *Configs) NewFileWalker(moduleName string, segmenter *block.Segmenter) *FileWalker {
	return NewFileWalker(c.ConfigMap[moduleName], segmenter, c.logger)
}

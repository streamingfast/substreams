package index

import (
	"fmt"

	"github.com/streamingfast/dstore"
	"go.uber.org/zap"
)

type Config struct {
	name       string
	moduleHash string
	objStore   dstore.Store

	moduleInitialBlock uint64

	logger *zap.Logger
}

func NewConfig(name string, moduleInitialBlock uint64, moduleHash string, baseStore dstore.Store, logger *zap.Logger) (*Config, error) {
	subStore, err := baseStore.SubStore(fmt.Sprintf("%s/index", moduleHash))
	if err != nil {
		return nil, fmt.Errorf("creating sub store: %w", err)
	}

	return &Config{
		name:               name,
		objStore:           subStore,
		moduleInitialBlock: moduleInitialBlock,
		moduleHash:         moduleHash,
		logger:             logger.With(zap.String("module", name)),
	}, nil
}

func (c *Config) NewFile() *File {
	return &File{
		moduleInitialBlock: c.moduleInitialBlock,
		store:              c.objStore,
		moduleName:         c.name,
		logger:             c.logger,
	}
}

package execout

import (
	"fmt"
	"sync"

	"go.uber.org/zap"

	"github.com/streamingfast/dstore"
)

type Config struct {
	name       string
	moduleHash string
	store      dstore.Store

	moduleInitialBlock uint64
}

func NewConfig(name string, moduleInitialBlock uint64, moduleHash string, baseStore dstore.Store) (*Config, error) {
	subStore, err := baseStore.SubStore(fmt.Sprintf("%s/outputs", moduleHash))
	if err != nil {
		return nil, fmt.Errorf("creating sub store: %w", err)
	}
	return &Config{
		name:               name,
		store:              subStore,
		moduleInitialBlock: moduleInitialBlock,
		moduleHash:         moduleHash,
	}, nil
}

func (conf *Config) NewFile(saveBlockInterval uint64, logger *zap.Logger) *File {
	return &File{
		wg:                &sync.WaitGroup{},
		ModuleName:        conf.name,
		store:             conf.store,
		saveBlockInterval: saveBlockInterval,
		logger:            logger.Named("cache").With(zap.String("module_name", conf.name)),
	}
}

package execout

import (
	"context"
	"fmt"
	"sync"

	"github.com/streamingfast/derr"
	"github.com/streamingfast/substreams/block"

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

type ExecOutputFiles = block.Ranges

func (c *Config) ListSnapshotFiles(ctx context.Context) (files ExecOutputFiles, err error) {
	err = derr.RetryContext(ctx, 3, func(ctx context.Context) error {
		if err := c.store.Walk(ctx, "", func(filename string) (err error) {
			fileInfo, ok := parseFileName(filename)
			if !ok {
				return nil
			}
			files = append(files, fileInfo)
			return nil
		}); err != nil {
			return fmt.Errorf("walking snapshots: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return files, nil
}

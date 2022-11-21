package execout

import (
	"context"
	"fmt"
	"sync"

	"github.com/streamingfast/derr"
	"github.com/streamingfast/dstore"
	"go.uber.org/zap"
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

func (c *Config) NewFile(saveBlockInterval uint64, logger *zap.Logger) *File {
	return &File{
		wg:                &sync.WaitGroup{},
		ModuleName:        c.name,
		store:             c.store,
		saveBlockInterval: saveBlockInterval,
		logger:            logger.Named("cache").With(zap.String("module_name", c.name)),
	}
}

func (c *Config) Name() string               { return c.name }
func (c *Config) ModuleInitialBlock() uint64 { return c.moduleInitialBlock }

func (c *Config) ListSnapshotFiles(ctx context.Context) (files FileInfos, err error) {
	err = derr.RetryContext(ctx, 3, func(ctx context.Context) error {
		if err := c.store.Walk(ctx, "", func(filename string) (err error) {
			fileInfo, err := parseFileName(filename)
			if err != nil {
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

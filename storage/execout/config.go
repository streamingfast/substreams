package execout

import (
	"context"
	"fmt"
	"sync"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"

	"github.com/streamingfast/derr"
	"github.com/streamingfast/dstore"
	"go.uber.org/zap"
)

type Config struct {
	name       string
	moduleHash string
	objStore   dstore.Store

	modKind pbsubstreams.ModuleKind

	moduleInitialBlock uint64
}

func NewConfig(name string, moduleInitialBlock uint64, modKind pbsubstreams.ModuleKind, moduleHash string, baseStore dstore.Store) (*Config, error) {
	subStore, err := baseStore.SubStore(fmt.Sprintf("%s/outputs", moduleHash))
	if err != nil {
		return nil, fmt.Errorf("creating sub store: %w", err)
	}
	return &Config{
		name:               name,
		objStore:           subStore,
		modKind:            modKind,
		moduleInitialBlock: moduleInitialBlock,
		moduleHash:         moduleHash,
	}, nil
}

func (c *Config) NewFile(saveBlockInterval uint64, logger *zap.Logger) *File {
	return &File{
		wg:                &sync.WaitGroup{},
		ModuleName:        c.name,
		store:             c.objStore,
		saveBlockInterval: saveBlockInterval,
		logger:            logger.Named("cache").With(zap.String("module_name", c.name)),
	}
}

func (c *Config) Name() string                        { return c.name }
func (c *Config) ModuleKind() pbsubstreams.ModuleKind { return c.modKind }
func (c *Config) ModuleInitialBlock() uint64          { return c.moduleInitialBlock }

func (c *Config) ListSnapshotFiles(ctx context.Context) (files FileInfos, err error) {
	err = derr.RetryContext(ctx, 3, func(ctx context.Context) error {
		if err := c.objStore.Walk(ctx, "", func(filename string) (err error) {
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

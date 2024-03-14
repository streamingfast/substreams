package execout

import (
	"context"
	"fmt"

	"github.com/streamingfast/bstream"
	"github.com/streamingfast/derr"
	"github.com/streamingfast/dstore"
	"go.uber.org/zap"

	"github.com/streamingfast/substreams/block"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	pboutput "github.com/streamingfast/substreams/storage/execout/pb"
)

type Config struct {
	name       string
	moduleHash string
	objStore   dstore.Store

	modKind            pbsubstreams.ModuleKind
	moduleInitialBlock uint64

	logger *zap.Logger
}

func NewConfig(name string, moduleInitialBlock uint64, modKind pbsubstreams.ModuleKind, moduleHash string, baseStore dstore.Store, logger *zap.Logger) (*Config, error) {
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
		logger:             logger.With(zap.String("module", name)),
	}, nil
}

func (c *Config) NewFile(targetRange *block.Range) *File {
	return &File{
		kv:         make(map[string]*pboutput.Item),
		ModuleName: c.name,
		store:      c.objStore,
		Range:      targetRange,
		logger:     c.logger,
	}
}

func (c *Config) Name() string                        { return c.name }
func (c *Config) ModuleKind() pbsubstreams.ModuleKind { return c.modKind }
func (c *Config) ModuleInitialBlock() uint64          { return c.moduleInitialBlock }

func (c *Config) ListSnapshotFiles(ctx context.Context, inRange *bstream.Range) (files FileInfos, err error) {
	err = derr.RetryContext(ctx, 3, func(ctx context.Context) error {
		// We must reset accumulated files between each retry
		files = nil

		return c.objStore.WalkFrom(ctx, "", computeDBinFilename(inRange.StartBlock(), 0), func(filename string) (err error) {
			fileInfo, err := parseFileName(filename)
			if err != nil {
				c.logger.Warn("seen exec output file that we don't know how to parse", zap.String("filename", filename), zap.Error(err))
				return nil
			}
			if inRange.ReachedEndBlock(fileInfo.BlockRange.ExclusiveEndBlock - 1) {
				return dstore.StopIteration
			}

			files = append(files, fileInfo)
			return nil
		})
	})
	if err != nil {
		return nil, fmt.Errorf("walking files: %s", err)
	}

	return files, nil
}

func (c *Config) ReadFile(ctx context.Context, inrange *block.Range) (*File, error) {

	file := c.NewFile(inrange)
	if err := file.Load(ctx); err != nil {
		return nil, err
	}
	return file, nil
}

package execout

import (
	"context"
	"fmt"
	"strings"

	"github.com/streamingfast/derr"
	"github.com/streamingfast/dstore"
	"go.uber.org/zap"

	"github.com/streamingfast/substreams/block"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

type Config struct {
	name               string
	moduleHash         string
	extendedModuleHash string // used for storing errors
	objStore           dstore.Store
	errStore           dstore.Store

	modKind            pbsubstreams.ModuleKind
	moduleInitialBlock uint64

	logger *zap.Logger
}

func NewConfig(name string, moduleInitialBlock uint64, modKind pbsubstreams.ModuleKind, moduleHash string, extendedModuleHash string, baseStore dstore.Store, logger *zap.Logger) (*Config, error) {
	subName := fmt.Sprintf("%s/outputs", moduleHash)
	if modKind == pbsubstreams.ModuleKindBlockIndex {
		subName = fmt.Sprintf("%s/index", moduleHash)
	}

	subStore, err := baseStore.SubStore(subName)
	if err != nil {
		return nil, fmt.Errorf("creating sub store: %w", err)
	}

	errStore, err := baseStore.SubStore(moduleHash)
	if err != nil {
		return nil, fmt.Errorf("creating err store: %w", err)
	}

	return &Config{
		name:               name,
		objStore:           subStore,
		errStore:           errStore,
		modKind:            modKind,
		moduleInitialBlock: moduleInitialBlock,
		moduleHash:         moduleHash,
		extendedModuleHash: extendedModuleHash,
		logger:             logger.With(zap.String("module", name)),
	}, nil
}

func (c *Config) WriteDeterministicError(ctx context.Context, atBlock uint64, err error) error {
	// Never write deterministic errors if context is cancelled, it cannot be deterministic in this case,
	// safeguard against wrong error handling over time.
	if ctx.Err() != nil {
		return nil
	}

	r := strings.NewReader(err.Error())
	return c.errStore.WriteObject(ctx, fmt.Sprintf("errors.%010d.%s", atBlock, c.extendedModuleHash), r)
}

func (c *Config) NewFile(targetRange *block.Range) *File {
	return &File{
		moduleName: c.name,
		store:      c.objStore,
		Range:      targetRange,
		logger:     c.logger,
	}
}

func (c *Config) OpenFileReader(ctx context.Context, targetRange *block.Range) (FileReader, error) {
	return OpenFileReader(ctx, c.objStore, c.logger, targetRange, c.name)
}

func (c *Config) NewFileWriter(ctx context.Context, targetRange *block.Range) FileWriter {
	return NewFileWriter(ctx, c.objStore, c.logger, targetRange, c.name)
}

func (c *Config) Name() string                        { return c.name }
func (c *Config) ModuleKind() pbsubstreams.ModuleKind { return c.modKind }
func (c *Config) ModuleInitialBlock() uint64          { return c.moduleInitialBlock }

func (c *Config) ListSnapshotFiles(ctx context.Context, from uint64, to uint64) (files FileInfos, err error) {
	err = derr.RetryContext(ctx, 3, func(ctx context.Context) error {
		// We must reset accumulated files between each retry
		files = nil

		fromFilename := computeDBinFilename(from, 0)
		toFilename := computeDBinFilename(to, 0)

		err := c.objStore.WalkFromTo(ctx, "", fromFilename, toFilename, func(filename string) (err error) {
			var fileInfo *FileInfo

			switch c.modKind {
			case pbsubstreams.ModuleKindBlockIndex:
				fileInfo, err = parseIndexFileName(filename)
			case pbsubstreams.ModuleKindMap:
				fileInfo, err = parseExecoutFileName(filename)
			default:
				return fmt.Errorf("wrong module kind: %v", c.modKind)
			}
			if err != nil {
				c.logger.Warn("seen exec output file that we don't know how to parse", zap.String("filename", filename), zap.Error(err))
				return nil
			}
			files = append(files, fileInfo)
			return nil
		})
		if err == dstore.StopIteration {
			return nil
		}
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("walking files: %w", err)
	}

	return files, nil
}

package store

import (
	"context"
	"fmt"
	"io"

	"github.com/streamingfast/derr"
	"github.com/streamingfast/dstore"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/storage/store/marshaller"
	"go.uber.org/zap"
)

type Config struct {
	name       string
	moduleHash string
	store      dstore.Store

	moduleInitialBlock uint64
	updatePolicy       pbsubstreams.Module_KindStore_UpdatePolicy
	valueType          string

	appendLimit uint64
}

func NewConfig(name string, moduleInitialBlock uint64, moduleHash string, updatePolicy pbsubstreams.Module_KindStore_UpdatePolicy, valueType string, store dstore.Store) (*Config, error) {
	subStore, err := store.SubStore(fmt.Sprintf("%s/states", moduleHash))
	if err != nil {
		return nil, fmt.Errorf("creating sub store: %w", err)
	}

	return &Config{
		name:               name,
		updatePolicy:       updatePolicy,
		valueType:          valueType,
		store:              subStore,
		moduleInitialBlock: moduleInitialBlock,
		moduleHash:         moduleHash,
		appendLimit:        8_388_608, // 8kb = 8 * 1024 * 1024,  // TODO(colin): make this configurable instead of hardcoded at 8kb
	}, nil
}

func (c *Config) newBaseStore(logger *zap.Logger) *baseStore {
	return &baseStore{
		Config:     c,
		kv:         make(map[string][]byte),
		logger:     logger.Named("store").With(zap.String("store_name", c.name)),
		marshaller: marshaller.Default(),
	}
}

func (c *Config) Name() string {
	return c.name
}

func (c *Config) ModuleHash() string {
	return c.moduleHash
}

func (c *Config) ValueType() string {
	return c.valueType
}

func (c *Config) UpdatePolicy() pbsubstreams.Module_KindStore_UpdatePolicy {
	return c.updatePolicy
}

func (c *Config) ModuleInitialBlock() uint64 {
	return c.moduleInitialBlock
}

func (c *Config) NewFullKV(logger *zap.Logger) *FullKV {
	return &FullKV{c.newBaseStore(logger), "N/A"}
}

func (c *Config) NewPartialKV(initialBlock uint64, logger *zap.Logger) *PartialKV {
	return &PartialKV{
		baseStore:    c.newBaseStore(logger),
		initialBlock: initialBlock,
	}
}

func (c *Config) FileSize(ctx context.Context, fileInfo *FileInfo) (uint64, error) {
	var size uint64
	err := derr.RetryContext(ctx, 3, func(ctx context.Context) error {
		rc, err := c.store.OpenObject(ctx, fileInfo.Filename)
		if err != nil {
			return fmt.Errorf("opening file: %w", err)
		}
		defer rc.Close()

		w := io.Discard
		n, err := io.Copy(w, rc)
		if err != nil {
			return fmt.Errorf("reading file: %w", err)
		}

		size = uint64(n)

		return nil

	})
	if err != nil {
		return 0, err
	}
	return size, nil
}

func (c *Config) ListSnapshotFiles(ctx context.Context) (files []*FileInfo, err error) {
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

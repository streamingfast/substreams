package stage

import (
	"context"
	"errors"
	"fmt"

	"go.uber.org/zap"

	"github.com/streamingfast/substreams/block"
	"github.com/streamingfast/substreams/storage/store"
)

// An individual module's progress towards synchronizing its `store`
type StoreModuleState struct {
	name   string
	logger *zap.Logger

	segmenter *block.Segmenter

	storeConfig *store.Config

	cachedStore      *store.FullKV
	lastBlockInStore uint64
}

func NewModuleState(logger *zap.Logger, name string, segmenter *block.Segmenter, storeConfig *store.Config) *StoreModuleState {
	return &StoreModuleState{
		name:        name,
		segmenter:   segmenter,
		logger:      logger,
		storeConfig: storeConfig,
	}
}

func (s *StoreModuleState) Name() string {
	return s.name
}

func (s *StoreModuleState) estimateStoreSizeBytes(ctx context.Context, exclusiveEndBlock uint64) (uncompressedSizeBytes uint64, metadata map[string]string, err error) {
	if s.lastBlockInStore == exclusiveEndBlock && s.cachedStore != nil {
		return s.cachedStore.SizeBytes(), nil, nil
	}

	fullKV := s.storeConfig.NewFullKV(s.logger)

	moduleInitBlock := s.storeConfig.ModuleInitialBlock()
	if moduleInitBlock < exclusiveEndBlock {
		fullKVFile := store.NewCompleteFileInfo(s.name, moduleInitBlock, exclusiveEndBlock)
		compressed, uncompressed, metadata, err := fullKV.GetSize(ctx, fullKVFile.Filename)
		if err != nil {
			return 0, nil, fmt.Errorf("get size of store %q: %w", s.name, err)
		}

		if uncompressed == nil {
			return compressed * 4, metadata, nil // gross estimation for cases where we don't have metadata
		}
		return *uncompressed, metadata, nil
	}
	return 0, metadata, nil
}

func (s *StoreModuleState) getStore(ctx context.Context, exclusiveEndBlock uint64) (*store.FullKV, error) {
	if s.lastBlockInStore == exclusiveEndBlock && s.cachedStore != nil {
		return s.cachedStore, nil
	}
	loadStore := s.storeConfig.NewFullKV(s.logger)
	moduleInitBlock := s.storeConfig.ModuleInitialBlock()
	if moduleInitBlock < exclusiveEndBlock { // note that 'endBlock' in a fullKV means the last block inserted in this store, from where we can start
		fullKVFile := store.NewCompleteFileInfo(s.name, moduleInitBlock, exclusiveEndBlock)
		err := loadStore.Load(ctx, fullKVFile)
		if err != nil {
			if errors.Is(err, store.ErrInvalidFullKVFile) {
				s.logger.Warn("found a corrupted fullKV store, deleting it", zap.Error(err), zap.String("store_name", loadStore.Name()), zap.String("store_hash", loadStore.ModuleHash()), zap.String("filename", fullKVFile.Filename))
				if err := loadStore.Delete(ctx, fullKVFile); err != nil {
					s.logger.Error("cannot delete corrupted fullKV store", zap.Error(err), zap.String("store_name", loadStore.Name()), zap.String("store_hash", loadStore.ModuleHash()), zap.String("filename", fullKVFile.Filename))
				}
			}
			return nil, fmt.Errorf("load full store %s (%s): %w", loadStore.Name(), loadStore.ModuleHash(), err)
		}
	}
	s.cachedStore = loadStore
	s.lastBlockInStore = exclusiveEndBlock
	return loadStore, nil
}

func (s *StoreModuleState) derivePartialKV(initialBlock uint64) *store.PartialKV {
	return s.storeConfig.NewPartialKV(initialBlock, s.logger)
}

//type MergeState int
//
//const (
//	MergeIdle MergeState = iota
//	MergeMerging
//	MergeCompleted // All merging operations were completed for the provided Segmenter
//)

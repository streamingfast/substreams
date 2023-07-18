package stage

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/streamingfast/substreams/block"
	"github.com/streamingfast/substreams/storage/store"
)

// An individual module's progress towards synchronizing its `store`
type ModuleState struct {
	name   string
	logger *zap.Logger

	segmenter *block.Segmenter

	storeConfig *store.Config

	cachedStore      *store.FullKV
	lastBlockInStore uint64
}

func NewModuleState(logger *zap.Logger, name string, segmenter *block.Segmenter, storeConfig *store.Config) *ModuleState {
	return &ModuleState{
		name:        name,
		segmenter:   segmenter,
		logger:      logger,
		storeConfig: storeConfig,
	}
}

func (s *ModuleState) getStore(ctx context.Context, exclusiveEndBlock uint64) (*store.FullKV, error) {
	if s.lastBlockInStore == exclusiveEndBlock && s.cachedStore != nil {
		return s.cachedStore, nil
	}
	loadStore := s.storeConfig.NewFullKV(s.logger)
	moduleInitBlock := s.storeConfig.ModuleInitialBlock()
	if moduleInitBlock != exclusiveEndBlock {
		fullKVFile := store.NewCompleteFileInfo(s.name, moduleInitBlock, exclusiveEndBlock)
		err := loadStore.Load(ctx, fullKVFile)
		if err != nil {
			return nil, fmt.Errorf("load store %q: %w", s.name, err)
		}
	}
	s.cachedStore = loadStore
	s.lastBlockInStore = exclusiveEndBlock
	return loadStore, nil
}

func (s *ModuleState) derivePartialKV(initialBlock uint64) *store.PartialKV {
	return s.storeConfig.NewPartialKV(initialBlock, s.logger)
}

//type MergeState int
//
//const (
//	MergeIdle MergeState = iota
//	MergeMerging
//	MergeCompleted // All merging operations were completed for the provided Segmenter
//)

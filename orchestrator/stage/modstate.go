package stage

import (
	"context"
	"errors"
	"fmt"
	"sync"

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

	// snapshotMu guards snapshotEnd: the squasher's upload pool advances it from
	// goroutines that outlive the merge that scheduled them (see singleSquash), so the
	// merge loop's own reads of it (canCopySnapshots, copyModuleSnapshots) and the
	// synchronous writes below can run concurrently with an in-flight upload.
	snapshotMu sync.Mutex
	// snapshotEnd is the end block of the last full store this module saved to or loaded
	// from storage. When it equals lastBlockInStore, that file holds cachedStore's content.
	snapshotEnd uint64
}

// SnapshotEnd is the end block of the last full store this module saved to or loaded from
// storage; see snapshotEnd.
func (s *StoreModuleState) SnapshotEnd() uint64 {
	s.snapshotMu.Lock()
	defer s.snapshotMu.Unlock()
	return s.snapshotEnd
}

// setSnapshotEnd unconditionally sets snapshotEnd, for callers that already know it to be
// current (a fresh load, or the empty-partial copy-forward path, which has its own
// ordering guarantees). Concurrent upload completions use advanceSnapshotEnd instead.
func (s *StoreModuleState) setSnapshotEnd(end uint64) {
	s.snapshotMu.Lock()
	defer s.snapshotMu.Unlock()
	s.snapshotEnd = end
}

// advanceSnapshotEnd raises snapshotEnd to end, never lowering it. The squasher's upload
// pool can complete segments out of order (a later segment's upload may finish before an
// earlier one's), and only the highest successfully-uploaded end block is meaningful.
func (s *StoreModuleState) advanceSnapshotEnd(end uint64) {
	s.snapshotMu.Lock()
	defer s.snapshotMu.Unlock()
	s.snapshotEnd = max(s.snapshotEnd, end)
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
	defer fullKV.Close() // only used for GetSize; releases the backing mmap file

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
	loadStore := s.storeConfig.NewSquasherFullKV(s.logger)
	moduleInitBlock := s.storeConfig.ModuleInitialBlock()
	if moduleInitBlock < exclusiveEndBlock { // note that 'endBlock' in a fullKV means the last block inserted in this store, from where we can start
		fullKVFile := store.NewCompleteFileInfo(s.name, moduleInitBlock, exclusiveEndBlock)
		err := loadStore.Load(ctx, fullKVFile)
		if err != nil {
			// Only a live context means real corruption: a canceled load also
			// surfaces as ErrInvalidFullKVFile and must not delete a valid file.
			if errors.Is(err, store.ErrInvalidFullKVFile) && ctx.Err() == nil {
				s.logger.Warn("found a corrupted fullKV store, deleting it", zap.Error(err), zap.String("store_name", loadStore.Name()), zap.String("store_hash", loadStore.ModuleHash()), zap.String("filename", fullKVFile.Filename))
				if err := loadStore.Delete(ctx, fullKVFile); err != nil {
					s.logger.Error("cannot delete corrupted fullKV store", zap.Error(err), zap.String("store_name", loadStore.Name()), zap.String("store_hash", loadStore.ModuleHash()), zap.String("filename", fullKVFile.Filename))
				}
			}
			loadStore.Close()
			return nil, fmt.Errorf("load full store %s (%s): %w", loadStore.Name(), loadStore.ModuleHash(), err)
		}
	}
	if s.cachedStore != nil {
		s.cachedStore.Close()
	}
	s.cachedStore = loadStore
	s.lastBlockInStore = exclusiveEndBlock
	if moduleInitBlock < exclusiveEndBlock {
		s.setSnapshotEnd(exclusiveEndBlock)
	}
	return loadStore, nil
}

func (s *StoreModuleState) Close() {
	if s.cachedStore != nil {
		s.cachedStore.Close()
		s.cachedStore = nil
	}
}

func (s *StoreModuleState) derivePartialKV(initialBlock uint64) *store.PartialKV {
	return s.storeConfig.NewPartialKV(initialBlock, s.logger)
}

// tryLoadFullKV attempts to load a FullKV from storage for the given block range.
// It does NOT touch cachedStore — use getStore for cache-aware loading.
// Returns (nil, nil) if moduleInitBlock >= exclusiveEndBlock.
// Returns (nil, err) if the file doesn't exist or is corrupt; callers treat this as "no fullKV available".
func (s *StoreModuleState) tryLoadFullKV(ctx context.Context, exclusiveEndBlock uint64) (*store.FullKV, error) {
	moduleInitBlock := s.storeConfig.ModuleInitialBlock()
	if moduleInitBlock >= exclusiveEndBlock {
		return nil, nil
	}
	kv := s.storeConfig.NewSquasherFullKV(s.logger)
	fullKVFile := store.NewCompleteFileInfo(s.name, moduleInitBlock, exclusiveEndBlock)
	if err := kv.Load(ctx, fullKVFile); err != nil {
		// A canceled load also surfaces as ErrInvalidFullKVFile; only delete
		// when the context is still live so we never drop a valid store file.
		if errors.Is(err, store.ErrInvalidFullKVFile) && ctx.Err() == nil {
			s.logger.Warn("found a corrupted fullKV store, deleting it", zap.Error(err), zap.String("store_name", kv.Name()), zap.String("store_hash", kv.ModuleHash()), zap.String("filename", fullKVFile.Filename))
			if delErr := kv.Delete(ctx, fullKVFile); delErr != nil {
				s.logger.Error("cannot delete corrupted fullKV store", zap.Error(delErr), zap.String("store_name", kv.Name()), zap.String("store_hash", kv.ModuleHash()), zap.String("filename", fullKVFile.Filename))
			}
		}
		kv.Close()
		return nil, err
	}
	return kv, nil
}

//type MergeState int
//
//const (
//	MergeIdle MergeState = iota
//	MergeMerging
//	MergeCompleted // All merging operations were completed for the provided Segmenter
//)

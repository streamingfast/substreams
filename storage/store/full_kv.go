package store

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	pbssinternal "github.com/streamingfast/substreams/pb/sf/substreams/intern/v2"
	"github.com/streamingfast/substreams/storage/store/marshaller"
)

var _ Store = (*FullKV)(nil)

type FullKV struct {
	*baseStore

	loadedFrom string
}

func (s *FullKV) Marshaller() marshaller.Marshaller {
	return s.marshaller
}

func (s *FullKV) DerivePartialStore(initialBlock uint64) *PartialKV {
	b := &baseStore{
		Config:     s.Config,
		pendingOps: &pbssinternal.Operations{},
		kv:         make(map[string][]byte),
		logger:     s.logger,
		marshaller: marshaller.Default(),
	}
	return &PartialKV{
		baseStore:    b,
		initialBlock: initialBlock,
		seen:         make(map[string]bool),
	}
}

func (s *FullKV) Load(ctx context.Context, file *FileInfo) error {
	s.loadedFrom = file.Filename
	s.logger.Debug("loading full store state from file", zap.String("fileName", file.Filename))

	data, err := loadStore(ctx, s.objStore, file.Filename)
	if err != nil {
		return fmt.Errorf("load full store %s at %s: %w", s.name, file.Filename, err)
	}

	storeData, size, err := s.marshaller.Unmarshal(data)
	if err != nil {
		return fmt.Errorf("unmarshal store: %w", err)
	}

	s.kv = storeData.Kv
	s.totalSizeBytes = size
	if s.kv == nil {
		s.kv = make(map[string][]byte)
	}

	s.logger.Debug("full store loaded", zap.String("fileName", file.Filename), zap.Int("key_count", len(s.kv)), zap.Uint64("data_size", size))
	return nil
}

// Save is to be called ONLY when we just passed the
// `nextExpectedBoundary` and processed nothing more after that
// boundary.
func (s *FullKV) Save(endBoundaryBlock uint64) (*FileInfo, *fileWriter, error) {
	s.logger.Debug("writing full store state", zap.Object("store", s))

	stateData := &marshaller.StoreData{
		Kv: s.kv,
	}

	content, err := s.marshaller.Marshal(stateData)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal kv state: %w", err)
	}

	file := NewCompleteFileInfo(s.name, s.moduleInitialBlock, endBoundaryBlock)

	s.logger.Debug("saving store",
		zap.String("file_name", file.Filename),
		zap.Object("block_range", file.Range),
	)

	fw := &fileWriter{
		store:    s.objStore,
		filename: file.Filename,
		content:  content,
	}

	return file, fw, nil
}

func (s *FullKV) String() string {
	return fmt.Sprintf("fullKV name %s moduleInitialBlock %d keyCount %d loadedFrom %s deltasCount %d", s.Name(), s.moduleInitialBlock, len(s.kv), s.loadedFrom, len(s.deltas))
}

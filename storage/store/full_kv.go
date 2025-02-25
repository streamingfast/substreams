package store

import (
	"context"
	"fmt"
	"io"

	pbssinternal "github.com/streamingfast/substreams/pb/sf/substreams/intern/v2"
	"github.com/streamingfast/substreams/storage/store/marshaller"
	"go.uber.org/zap"
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
		kvOps:      &pbssinternal.Operations{},
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

var ErrNoQuickSaveStore = fmt.Errorf("no quick save store")

func (s *FullKV) QuickLoad(ctx context.Context, atBlockHash string) error {
	if s.quickSaveStore == nil {
		return ErrNoQuickSaveStore
	}

	filename := atBlockHash + ".quicksave"
	s.logger.Debug("loading full store state from temporary file", zap.String("fileName", filename), zap.String("module_hash", s.moduleHash))

	r, err := s.quickSaveStore.OpenObject(ctx, filename)
	if err != nil {
		return fmt.Errorf("opening file %q (in module %q): %w", filename, s.moduleHash, err)
	}

	defer r.Close()
	data, err := io.ReadAll(r)
	if err != nil {
		return fmt.Errorf("reading data: %w", err)
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

	s.logger.Debug("full store loaded", zap.String("fileName", filename), zap.Int("key_count", len(s.kv)), zap.Uint64("data_size", size))
	return nil
}

func (s *FullKV) QuickSave(ctx context.Context, atBlockHash string) error {
	if s.quickSaveStore == nil {
		return ErrNoQuickSaveStore
	}
	s.logger.Debug("writing temporary store state", zap.Object("store", s))

	stateData := &marshaller.StoreData{
		Kv: s.kv,
	}

	content, err := s.marshaller.Marshal(stateData)
	if err != nil {
		return fmt.Errorf("marshal kv state: %w", err)
	}

	fw := &fileWriter{
		store:    s.quickSaveStore,
		filename: atBlockHash + ".quicksave",
		content:  content,
	}
	return fw.Write(ctx)
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

package store

import (
	"context"
	"fmt"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/store/marshaller"
	"google.golang.org/protobuf/proto"

	"github.com/streamingfast/substreams/block"
	"go.uber.org/zap"
)

var _ Store = (*FullKV)(nil)

type FullKV struct {
	*baseStore
}

func (s *FullKV) Marshaller() marshaller.Marshaller {
	return s.marshaller
}

func (s *FullKV) DerivePartialStore(initialBlock uint64) *PartialKV {
	b := &baseStore{
		Config:     s.Config,
		kv:         make(map[string][]byte),
		logger:     s.logger,
		marshaller: marshaller.Default(),
	}
	return &PartialKV{
		baseStore:    b,
		initialBlock: initialBlock,
	}
}

func (s *FullKV) storageFilename(exclusiveEndBlock uint64) string {
	return fullStateFileName(block.NewRange(s.moduleInitialBlock, exclusiveEndBlock))
}

func (s *FullKV) Load(ctx context.Context, exclusiveEndBlock uint64) error {
	fileName := s.storageFilename(exclusiveEndBlock)
	s.logger.Debug("loading full store state from file", zap.String("module_name", s.name), zap.String("fileName", fileName))

	data, err := loadStore(ctx, s.store, fileName)
	if err != nil {
		return fmt.Errorf("load full store %s at %s: %w", s.name, fileName, err)
	}

	stateData := &pbsubstreams.StoreData{}
	if err := proto.Unmarshal(data, stateData); err != nil {
		return fmt.Errorf("unmarshal store: %w", err)
	}

	s.kv = stateData.GetKv()

	s.logger.Debug("full store loaded", zap.String("store_name", s.name), zap.String("fileName", fileName))
	return nil
}

// Save is to be called ONLY when we just passed the
// `nextExpectedBoundary` and processed nothing more after that
// boundary.
func (s *FullKV) Save(endBoundaryBlock uint64) (*block.Range, *FileWriter, error) {
	s.logger.Debug("writing full store state", zap.Object("store", s))

	stateData := &pbsubstreams.StoreData{
		Kv: s.kv,
	}

	content, err := proto.Marshal(stateData)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal kv state: %w", err)
	}

	filename := s.storageFilename(endBoundaryBlock)
	brange := block.NewRange(s.moduleInitialBlock, endBoundaryBlock)

	s.logger.Info("full store state saved",
		zap.String("store", s.name),
		zap.String("file_name", filename),
		zap.Object("block_range", brange),
	)

	fw := &FileWriter{
		store:    s.store,
		filename: filename,
		content:  content,
	}

	return brange, fw, nil
}

func (s *FullKV) Reset() {
	if tracer.Enabled() {
		s.logger.Debug("flushing store", zap.String("name", s.name), zap.Int("delta_count", len(s.deltas)), zap.Int("entry_count", len(s.kv)))
	}
	s.deltas = nil
	s.lastOrdinal = 0
}

package store

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/streamingfast/substreams/block"
	"go.uber.org/zap"
)

// compile-time check that BaseStore implements all interfaces
var _ Store = (*PartialKV)(nil)

type PartialKV struct {
	*BaseStore

	initialBlock    uint64 // block at which we initialized this store
	DeletedPrefixes []string
}

func NewPartialKV(store *BaseStore, initialBlock uint64) *PartialKV {
	return &PartialKV{
		BaseStore:    store,
		initialBlock: initialBlock,
	}
}

func (p *PartialKV) Roll(lastBlock uint64) {
	p.initialBlock = lastBlock
	p.BaseStore.kv = map[string][]byte{}
}

func (s *PartialKV) InitialBlock() uint64 { return s.initialBlock }

type storeData struct {
	KV              map[string][]byte `json:"kv"`
	DeletedPrefixes []string          `json:"deleted_prefixes"`
}

func (p *PartialKV) Load(ctx context.Context, exclusiveEndBlock uint64) error {
	filename := p.storageFilename(exclusiveEndBlock)
	p.logger.Debug("loading partial store state from file", zap.String("filename", filename))

	data, err := loadStore(ctx, p.store, filename)
	if err != nil {
		return fmt.Errorf("load partial store %s at %s: %w", p.name, filename, err)
	}

	stateData := &storeData{}
	if err = json.Unmarshal(data, &stateData); err != nil {
		return fmt.Errorf("unmarshal data: %w", err)
	}
	p.kv = stateData.KV
	p.DeletedPrefixes = stateData.DeletedPrefixes

	p.logger.Debug("partial store loaded", zap.String("filename", filename))
	return nil
}

func (p *PartialKV) Save(ctx context.Context, endBoundaryBlock uint64) (*block.Range, error) {
	p.logger.Debug("writing partial store  state", zap.Object("store", p))

	data := &storeData{
		KV:              p.kv,
		DeletedPrefixes: p.DeletedPrefixes,
	}

	content, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal partial data: %w", err)
	}

	filename := p.storageFilename(endBoundaryBlock)
	if err := saveStore(ctx, p.store, filename, content); err != nil {
		return nil, fmt.Errorf("write partial store %q in file %q: %w", p.name, filename, err)
	}

	brange := block.NewRange(p.initialBlock, endBoundaryBlock)
	p.logger.Info("partial store state written",
		zap.String("store", p.name),
		zap.String("file_name", filename),
		zap.Object("block_range", brange),
	)

	return brange, nil
}

func (p *PartialKV) DeletePrefix(ord uint64, prefix string) {
	p.BaseStore.DeletePrefix(ord, prefix)

	p.DeletedPrefixes = append(p.DeletedPrefixes, prefix)
}

func (p *PartialKV) DeleteStore(ctx context.Context, endBlock uint64) (err error) {
	filename := p.storageFilename(endBlock)
	zlog.Debug("deleting partial store file", zap.String("file_name", filename))

	if err = p.store.DeleteObject(ctx, filename); err != nil {
		zlog.Warn("deleting file", zap.String("file_name", filename), zap.Error(err))
	}
	return err
}

func (p *PartialKV) storageFilename(exclusiveEndBlock uint64) string {
	return partialFileName(block.NewRange(p.initialBlock, exclusiveEndBlock))
}

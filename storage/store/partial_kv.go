package store

import (
	"context"
	"fmt"

	"github.com/streamingfast/substreams/storage/store/marshaller"

	"github.com/streamingfast/substreams/block"
	"go.uber.org/zap"
)

var _ Store = (*PartialKV)(nil)

type PartialKV struct {
	*baseStore

	initialBlock    uint64 // block at which we initialized this store
	DeletedPrefixes []string

	loadedFrom string
}

func (p *PartialKV) Roll(lastBlock uint64) {
	p.initialBlock = lastBlock
	p.baseStore.kv = map[string][]byte{}
}

func (p *PartialKV) InitialBlock() uint64 { return p.initialBlock }

func (p *PartialKV) Load(ctx context.Context, exclusiveEndBlock uint64) error {
	filename := p.storageFilename(exclusiveEndBlock)
	p.loadedFrom = filename
	p.logger.Debug("loading partial store state from file", zap.String("filename", filename))

	data, err := loadStore(ctx, p.objStore, filename)
	if err != nil {
		return fmt.Errorf("load partial store %s at %s: %w", p.name, filename, err)
	}

	storeData, size, err := p.marshaller.Unmarshal(data)
	if err != nil {
		return fmt.Errorf("unmarshal store: %w", err)
	}

	p.kv = storeData.Kv
	if p.kv == nil {
		p.kv = map[string][]byte{}
	}
	p.totalSizeBytes = size
	p.DeletedPrefixes = storeData.DeletePrefixes

	p.logger.Debug("partial store loaded", zap.String("filename", filename), zap.Int("key_count", len(p.kv)), zap.Uint64("data_size", size))
	return nil
}

func (p *PartialKV) Save(endBoundaryBlock uint64) (*block.Range, *fileWriter, error) {
	p.logger.Debug("writing partial store state", zap.Object("store", p))

	stateData := &marshaller.StoreData{
		Kv:             p.kv,
		DeletePrefixes: p.DeletedPrefixes,
	}

	content, err := p.marshaller.Marshal(stateData)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal partial data: %w", err)
	}

	filename := p.storageFilename(endBoundaryBlock)

	brange := block.NewRange(p.initialBlock, endBoundaryBlock)
	p.logger.Info("partial store save written",
		zap.String("file_name", filename),
		zap.Object("block_range", brange),
	)

	fw := &fileWriter{
		store:    p.objStore,
		filename: filename,
		content:  content,
	}

	return brange, fw, nil
}

func (p *PartialKV) DeletePrefix(ord uint64, prefix string) {
	p.baseStore.DeletePrefix(ord, prefix)

	p.DeletedPrefixes = append(p.DeletedPrefixes, prefix)
}

func (p *PartialKV) DeleteStore(ctx context.Context, endBlock uint64) (err error) {
	filename := p.storageFilename(endBlock)
	zlog.Debug("deleting partial store file", zap.String("file_name", filename))

	if err = p.objStore.DeleteObject(ctx, filename); err != nil {
		zlog.Warn("deleting file", zap.String("file_name", filename), zap.Error(err))
	}
	return err
}

func (p *PartialKV) storageFilename(exclusiveEndBlock uint64) string {
	return partialFileName(block.NewRange(p.initialBlock, exclusiveEndBlock))
}

func (p *PartialKV) String() string {
	return fmt.Sprintf("partialKV name %s moduleInitialBlock %d  keyCount %d deltasCount %d loadFrom %s", p.Name(), p.moduleInitialBlock, len(p.kv), len(p.deltas), p.loadedFrom)
}

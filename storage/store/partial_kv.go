package store

import (
	"context"
	"encoding/binary"
	"fmt"
	"math"
	"math/big"

	"github.com/shopspring/decimal"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/storage/store/marshaller"
	"go.uber.org/zap"
)

var _ Store = (*PartialKV)(nil)

type PartialKV struct {
	*baseStore

	initialBlock    uint64 // block at which we initialized this store
	DeletedPrefixes []string

	loadedFrom string
	seen       map[string]bool
}

func (p *PartialKV) Roll(lastBlock uint64) {
	p.initialBlock = lastBlock
	p.baseStore.kv = map[string][]byte{}
}

func (p *PartialKV) InitialBlock() uint64 { return p.initialBlock }

func (p *PartialKV) Load(ctx context.Context, file *FileInfo) error {
	p.loadedFrom = file.Filename
	p.logger.Debug("loading partial store state from file", zap.String("filename", file.Filename))

	data, err := loadStore(ctx, p.objStore, file.Filename)
	if err != nil {
		return fmt.Errorf("load partial store %s at %s: %w", p.name, file.Filename, err)
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

	p.logger.Debug("partial store loaded", zap.String("filename", file.Filename), zap.Int("key_count", len(p.kv)), zap.Uint64("data_size", size))
	return nil
}

func (p *PartialKV) Save(endBoundaryBlock uint64) (*FileInfo, *fileWriter, error) {
	p.logger.Debug("writing partial store state", zap.Object("store", p))

	stateData := &marshaller.StoreData{
		Kv:             p.kv,
		DeletePrefixes: p.DeletedPrefixes,
	}

	content, err := p.marshaller.Marshal(stateData)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal partial data: %w", err)
	}

	file := NewPartialFileInfo(p.name, p.initialBlock, endBoundaryBlock)
	p.logger.Debug("partial store save written", zap.String("file_name", file.Filename), zap.Stringer("block_range", file.Range))

	fw := &fileWriter{
		store:    p.objStore,
		filename: file.Filename,
		content:  content,
	}

	return file, fw, nil
}

func (p *PartialKV) DeletePrefix(ord uint64, prefix string) {
	p.baseStore.DeletePrefix(ord, prefix)

	if !p.seen[prefix] {
		p.DeletedPrefixes = append(p.DeletedPrefixes, prefix)
		p.seen[prefix] = true
	}
}

func (p *PartialKV) DeleteStore(ctx context.Context, file *FileInfo) (err error) {
	zlog.Debug("deleting partial store file", zap.String("file_name", file.Filename))

	if err = p.objStore.DeleteObject(ctx, file.Filename); err != nil {
		zlog.Warn("deleting file", zap.String("file_name", file.Filename), zap.Error(err))
	}
	return err
}

func (p *PartialKV) String() string {
	return fmt.Sprintf("partialKV name %s moduleInitialBlock %d  keyCount %d deltasCount %d loadFrom %s", p.Name(), p.moduleInitialBlock, len(p.kv), len(p.deltas), p.loadedFrom)
}

func valueToFloat64(value []byte) float64 {
	return math.Float64frombits(binary.LittleEndian.Uint64(value))
}

func valueToInt64(value []byte) int64 {
	big := new(big.Int)
	big.SetString(string(value), 10)
	return big.Int64()
}

func valueToBigInt(value []byte) *big.Int {
	big := new(big.Int)
	big.SetString(string(value), 10)
	return big
}

func valueToBigDecimal(value []byte) (decimal.Decimal, error) {
	val := decimal.Decimal{}
	err := val.UnmarshalBinary(value)
	if err != nil {
		return decimal.Decimal{}, err
	}
	return val, nil
}

func (p *PartialKV) ApplyDelta(delta *pbsubstreams.StoreDelta) {
	panic("caching store cannot be used with deltas")
}

func (p *PartialKV) ApplyDeltasReverse(deltas []*pbsubstreams.StoreDelta) {
	panic("caching store cannot be used with deltas")
}

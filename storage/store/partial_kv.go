package store

import (
	"context"
	"encoding/binary"
	"fmt"
	"math"
	"math/big"

	"github.com/shopspring/decimal"
	pbssinternal "github.com/streamingfast/substreams/pb/sf/substreams/intern/v2"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/storage/store/marshaller"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
)

var _ Store = (*PartialKV)(nil)

type PartialKV struct {
	*baseStore

	operations      *pbssinternal.Operations
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
	p.operations.Operations = append(p.operations.Operations, &pbssinternal.Operation{
		Type: pbssinternal.Operation_DELETE_PREFIX,
		Ord:  ord,
		Key:  prefix,
	})

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

func (p *PartialKV) Reset() {
	p.operations = &pbssinternal.Operations{}
	p.baseStore.Reset()
}

func (p *PartialKV) ApplyOps(in []byte) error {
	return applyOps(in, p.baseStore)
}

func (p *PartialKV) ReadOps() []byte {
	data, err := proto.Marshal(p.operations)
	if err != nil {
		panic(err)
	}
	return data
}

func applyOps(in []byte, store *baseStore) error {

	ops := &pbssinternal.Operations{}
	if err := proto.Unmarshal(in, ops); err != nil {
		return err
	}

	for _, op := range ops.Operations {
		switch op.Type {
		case pbssinternal.Operation_SET:
			store.Set(op.Ord, op.Key, string(op.Value))
		case pbssinternal.Operation_SET_BYTES:
			store.SetBytes(op.Ord, op.Key, op.Value)
		case pbssinternal.Operation_SET_IF_NOT_EXISTS:
			store.SetIfNotExists(op.Ord, op.Key, string(op.Value))
		case pbssinternal.Operation_SET_BYTES_IF_NOT_EXISTS:
			store.SetBytesIfNotExists(op.Ord, op.Key, op.Value)
		case pbssinternal.Operation_APPEND:
			store.Append(op.Ord, op.Key, op.Value)
		case pbssinternal.Operation_DELETE_PREFIX:
			store.DeletePrefix(op.Ord, op.Key)
		case pbssinternal.Operation_SET_MAX_BIG_INT:
			big := new(big.Int)
			big.SetBytes(op.Value)
			store.SetMaxBigInt(op.Ord, op.Key, big)
		case pbssinternal.Operation_SET_MAX_INT64:
			big := new(big.Int)
			big.SetBytes(op.Value)
			val := big.Int64()
			store.SetMaxInt64(op.Ord, op.Key, val)
		case pbssinternal.Operation_SET_MAX_FLOAT64:
			val := math.Float64frombits(binary.LittleEndian.Uint64(op.Value))
			store.SetMaxFloat64(op.Ord, op.Key, val)
		case pbssinternal.Operation_SET_MAX_BIG_DECIMAL:
			val, err := decimal.NewFromString(string(op.Value))
			if err != nil {
				return err
			}
			store.SetMaxBigDecimal(op.Ord, op.Key, val)
		case pbssinternal.Operation_SET_MIN_BIG_INT:
			big := new(big.Int)
			big.SetBytes(op.Value)
			store.SetMinBigInt(op.Ord, op.Key, big)
		case pbssinternal.Operation_SET_MIN_INT64:
			big := new(big.Int)
			big.SetBytes(op.Value)
			val := big.Int64()
			store.SetMinInt64(op.Ord, op.Key, val)
		case pbssinternal.Operation_SET_MIN_FLOAT64:
			val := math.Float64frombits(binary.LittleEndian.Uint64(op.Value))
			store.SetMinFloat64(op.Ord, op.Key, val)
		case pbssinternal.Operation_SET_MIN_BIG_DECIMAL:
			val, err := decimal.NewFromString(string(op.Value))
			if err != nil {
				return err
			}
			store.SetMinBigDecimal(op.Ord, op.Key, val)
		case pbssinternal.Operation_SUM_BIG_INT:
			big := new(big.Int)
			big.SetBytes(op.Value)
			store.SumBigInt(op.Ord, op.Key, big)
		case pbssinternal.Operation_SUM_INT64:
			big := new(big.Int)
			big.SetBytes(op.Value)
			val := big.Int64()
			store.SumInt64(op.Ord, op.Key, val)
		case pbssinternal.Operation_SUM_FLOAT64:
			val := math.Float64frombits(binary.LittleEndian.Uint64(op.Value))
			store.SumFloat64(op.Ord, op.Key, val)
		case pbssinternal.Operation_SUM_BIG_DECIMAL:
			val, err := decimal.NewFromString(string(op.Value))
			if err != nil {
				return err
			}
			store.SumBigDecimal(op.Ord, op.Key, val)
		}
	}
	return nil
}

func (p *PartialKV) ApplyDelta(delta *pbsubstreams.StoreDelta) {
	panic("caching store cannot be used with deltas")
}

func (p *PartialKV) ApplyDeltasReverse(deltas []*pbsubstreams.StoreDelta) {
	panic("caching store cannot be used with deltas")
}

// apparently this is faster than append() method
func cloneBytes(b []byte) []byte {
	out := make([]byte, len(b))
	copy(out, b)
	return out
}

func (p *PartialKV) Set(ord uint64, key string, value string) {
	p.operations.Operations = append(p.operations.Operations, &pbssinternal.Operation{
		Type:  pbssinternal.Operation_SET,
		Ord:   ord,
		Key:   key,
		Value: cloneBytes([]byte(value)),
	})

	p.baseStore.Set(ord, key, value)
}

func (p *PartialKV) SetBytes(ord uint64, key string, value []byte) {
	p.operations.Operations = append(p.operations.Operations, &pbssinternal.Operation{
		Type:  pbssinternal.Operation_SET_BYTES,
		Ord:   ord,
		Key:   key,
		Value: cloneBytes(value),
	})

	p.baseStore.SetBytes(ord, key, value)
}

func (p *PartialKV) SetIfNotExists(ord uint64, key string, value string) {
	p.operations.Operations = append(p.operations.Operations, &pbssinternal.Operation{
		Type:  pbssinternal.Operation_SET_IF_NOT_EXISTS,
		Ord:   ord,
		Key:   key,
		Value: cloneBytes([]byte(value)),
	})

	p.baseStore.SetIfNotExists(ord, key, value)
}

func (p *PartialKV) SetBytesIfNotExists(ord uint64, key string, value []byte) {
	p.operations.Operations = append(p.operations.Operations, &pbssinternal.Operation{
		Type:  pbssinternal.Operation_SET_BYTES_IF_NOT_EXISTS,
		Ord:   ord,
		Key:   key,
		Value: cloneBytes(value),
	})

	p.baseStore.SetBytesIfNotExists(ord, key, value)
}

func (p *PartialKV) Append(ord uint64, key string, value []byte) error {
	p.operations.Operations = append(p.operations.Operations, &pbssinternal.Operation{
		Type:  pbssinternal.Operation_APPEND,
		Ord:   ord,
		Key:   key,
		Value: cloneBytes(value),
	})

	return p.baseStore.Append(ord, key, value)
}

func (p *PartialKV) SetMaxBigInt(ord uint64, key string, value *big.Int) {
	p.operations.Operations = append(p.operations.Operations, &pbssinternal.Operation{
		Type:  pbssinternal.Operation_SET_MAX_BIG_INT,
		Ord:   ord,
		Key:   key,
		Value: value.Bytes(),
	})

	p.baseStore.SetMaxBigInt(ord, key, value)
}

func (p *PartialKV) SetMaxInt64(ord uint64, key string, value int64) {
	big := new(big.Int)
	big.SetInt64(value)

	p.operations.Operations = append(p.operations.Operations, &pbssinternal.Operation{
		Type:  pbssinternal.Operation_SET_MAX_INT64,
		Ord:   ord,
		Key:   key,
		Value: big.Bytes(),
	})
	p.baseStore.SetMaxInt64(ord, key, value)
}

func (p *PartialKV) SetMaxFloat64(ord uint64, key string, value float64) {
	var buf [8]byte
	binary.LittleEndian.PutUint64(buf[:], math.Float64bits(value))
	p.operations.Operations = append(p.operations.Operations, &pbssinternal.Operation{
		Type:  pbssinternal.Operation_SET_MAX_FLOAT64,
		Ord:   ord,
		Key:   key,
		Value: buf[:],
	})

	p.baseStore.SetMaxFloat64(ord, key, value)
}

func (p *PartialKV) SetMaxBigDecimal(ord uint64, key string, value decimal.Decimal) {
	p.operations.Operations = append(p.operations.Operations, &pbssinternal.Operation{
		Type:  pbssinternal.Operation_SET_MAX_BIG_DECIMAL,
		Ord:   ord,
		Key:   key,
		Value: []byte(value.String()),
	})

	p.baseStore.SetMaxBigDecimal(ord, key, value)
}

func (p *PartialKV) SetMinBigInt(ord uint64, key string, value *big.Int) {
	p.operations.Operations = append(p.operations.Operations, &pbssinternal.Operation{
		Type:  pbssinternal.Operation_SET_MIN_BIG_INT,
		Ord:   ord,
		Key:   key,
		Value: value.Bytes(),
	})
	p.baseStore.SetMinBigInt(ord, key, value)
}

func (p *PartialKV) SetMinInt64(ord uint64, key string, value int64) {
	big := new(big.Int)
	big.SetInt64(value)
	p.operations.Operations = append(p.operations.Operations, &pbssinternal.Operation{
		Type:  pbssinternal.Operation_SET_MIN_INT64,
		Ord:   ord,
		Key:   key,
		Value: big.Bytes(),
	})

	p.baseStore.SetMinInt64(ord, key, value)
}

func (p *PartialKV) SetMinFloat64(ord uint64, key string, value float64) {
	var buf [8]byte
	binary.LittleEndian.PutUint64(buf[:], math.Float64bits(value))
	p.operations.Operations = append(p.operations.Operations, &pbssinternal.Operation{
		Type:  pbssinternal.Operation_SET_MIN_FLOAT64,
		Ord:   ord,
		Key:   key,
		Value: buf[:],
	})

	p.baseStore.SetMinFloat64(ord, key, value)
}

func (p *PartialKV) SetMinBigDecimal(ord uint64, key string, value decimal.Decimal) {
	p.operations.Operations = append(p.operations.Operations, &pbssinternal.Operation{
		Type:  pbssinternal.Operation_SET_MIN_BIG_DECIMAL,
		Ord:   ord,
		Key:   key,
		Value: []byte(value.String()),
	})

	p.baseStore.SetMinBigDecimal(ord, key, value)
}

func (p *PartialKV) SumBigInt(ord uint64, key string, value *big.Int) {
	p.operations.Operations = append(p.operations.Operations, &pbssinternal.Operation{
		Type:  pbssinternal.Operation_SUM_BIG_INT,
		Ord:   ord,
		Key:   key,
		Value: value.Bytes(),
	})

	p.baseStore.SumBigInt(ord, key, value)
}

func (p *PartialKV) SumInt64(ord uint64, key string, value int64) {
	big := new(big.Int)
	big.SetInt64(value)

	p.operations.Operations = append(p.operations.Operations, &pbssinternal.Operation{
		Type:  pbssinternal.Operation_SUM_INT64,
		Ord:   ord,
		Key:   key,
		Value: big.Bytes(),
	})

	p.baseStore.SumInt64(ord, key, value)
}

func (p *PartialKV) SumFloat64(ord uint64, key string, value float64) {
	var buf [8]byte
	binary.LittleEndian.PutUint64(buf[:], math.Float64bits(value))
	p.operations.Operations = append(p.operations.Operations, &pbssinternal.Operation{
		Type:  pbssinternal.Operation_SUM_FLOAT64,
		Ord:   ord,
		Key:   key,
		Value: buf[:],
	})

	p.baseStore.SumFloat64(ord, key, value)
}

func (p *PartialKV) SumBigDecimal(ord uint64, key string, value decimal.Decimal) {
	p.operations.Operations = append(p.operations.Operations, &pbssinternal.Operation{
		Type:  pbssinternal.Operation_SUM_BIG_DECIMAL,
		Ord:   ord,
		Key:   key,
		Value: []byte(value.String()),
	})

	p.baseStore.SumBigDecimal(ord, key, value)
}

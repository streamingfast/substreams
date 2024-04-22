package store

import (
	"fmt"

	pbssinternal "github.com/streamingfast/substreams/pb/sf/substreams/intern/v2"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/storage/store/marshaller"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"google.golang.org/protobuf/proto"
)

type baseStore struct {
	*Config

	kv         map[string][]byte        // kv is the state, and assumes all deltas were already applied to it.
	pendingOps *pbssinternal.Operations // operations to the curent block called from the WASM module
	// deltas are always deltas for the given block. they are produced when store is flushed
	// 	and used to read back in the store at different ordinals
	deltas         []*pbsubstreams.StoreDelta
	lastOrdinal    uint64
	marshaller     marshaller.Marshaller
	totalSizeBytes uint64

	logger *zap.Logger
}

func (b *baseStore) Name() string { return b.name }

func (b *baseStore) InitialBlock() uint64 { return b.moduleInitialBlock }

func (b *baseStore) String() string {
	return fmt.Sprintf("%q (%q)", b.name, b.moduleHash)
}

func (b *baseStore) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	enc.AddString("name", b.name)
	enc.AddString("hash", b.moduleHash)
	enc.AddUint64("module_initial_block", b.moduleInitialBlock)
	enc.AddInt("key_count", len(b.kv))
	enc.AddUint64("total_size_bytes", b.totalSizeBytes)

	return nil
}

func (b *baseStore) Reset() {
	if tracer.Enabled() {
		b.logger.Debug("flushing store", zap.Int("delta_count", len(b.deltas)), zap.Int("entry_count", len(b.kv)), zap.Uint64("total_size_bytes", b.totalSizeBytes))
	}
	b.pendingOps = &pbssinternal.Operations{}
	b.deltas = nil
	b.lastOrdinal = 0
}

func (b *baseStore) ReadOps() []byte {
	data, err := proto.Marshal(b.pendingOps)
	if err != nil {
		panic(err)
	}
	return data
}

func (b *baseStore) ValueType() string {
	return b.valueType
}

func (b *baseStore) UpdatePolicy() pbsubstreams.Module_KindStore_UpdatePolicy {
	return b.updatePolicy
}

func (b *baseStore) ApplyOps(in []byte) error {
	ops := &pbssinternal.Operations{}
	if err := proto.Unmarshal(in, ops); err != nil {
		return err
	}
	b.pendingOps = ops
	return b.Flush()
}

func (b *baseStore) Flush() error {
	b.pendingOps.Sort()
	for _, op := range b.pendingOps.Operations {
		switch op.Type {
		case pbssinternal.Operation_SET:
			b.set(op.Ord, op.Key, op.Value)
		case pbssinternal.Operation_SET_BYTES:
			b.set(op.Ord, op.Key, op.Value)
		case pbssinternal.Operation_SET_IF_NOT_EXISTS:
			b.setIfNotExists(op.Ord, op.Key, op.Value)
		case pbssinternal.Operation_SET_BYTES_IF_NOT_EXISTS:
			b.setIfNotExists(op.Ord, op.Key, op.Value)
		case pbssinternal.Operation_APPEND:
			if err := b.append(op.Ord, op.Key, op.Value); err != nil {
				return err
			}
		case pbssinternal.Operation_DELETE_PREFIX:
			b.deletePrefix(op.Ord, op.Key)
		case pbssinternal.Operation_SET_MAX_BIG_INT:
			b.setMaxBigInt(op.Ord, op.Key, valueToBigInt(op.Value))
		case pbssinternal.Operation_SET_MAX_INT64:
			b.setMaxInt64(op.Ord, op.Key, valueToInt64(op.Value))
		case pbssinternal.Operation_SET_MAX_FLOAT64:
			b.setMaxFloat64(op.Ord, op.Key, valueToFloat64(op.Value))
		case pbssinternal.Operation_SET_MAX_BIG_DECIMAL:
			val, err := valueToBigDecimal(op.Value)
			if err != nil {
				return err
			}
			b.setMaxBigDecimal(op.Ord, op.Key, val)
		case pbssinternal.Operation_SET_MIN_BIG_INT:
			b.setMinBigInt(op.Ord, op.Key, valueToBigInt(op.Value))
		case pbssinternal.Operation_SET_MIN_INT64:
			b.setMinInt64(op.Ord, op.Key, valueToInt64(op.Value))
		case pbssinternal.Operation_SET_MIN_FLOAT64:
			b.setMinFloat64(op.Ord, op.Key, valueToFloat64(op.Value))
		case pbssinternal.Operation_SET_MIN_BIG_DECIMAL:
			val, err := valueToBigDecimal(op.Value)
			if err != nil {
				return err
			}
			b.setMinBigDecimal(op.Ord, op.Key, val)
		case pbssinternal.Operation_SUM_BIG_INT:
			b.sumBigInt(op.Ord, op.Key, valueToBigInt(op.Value))
		case pbssinternal.Operation_SUM_INT64:
			b.sumInt64(op.Ord, op.Key, valueToInt64(op.Value))
		case pbssinternal.Operation_SUM_FLOAT64:
			b.sumFloat64(op.Ord, op.Key, valueToFloat64(op.Value))
		case pbssinternal.Operation_SUM_BIG_DECIMAL:
			val, err := valueToBigDecimal(op.Value)
			if err != nil {
				return err
			}
			b.sumBigDecimal(op.Ord, op.Key, val)
		}
		b.lastOrdinal = op.Ord
	}
	return nil
}

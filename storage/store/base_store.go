package store

import (
	"fmt"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/storage/store/marshaller"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type baseStore struct {
	*Config

	kv             map[string][]byte          // kv is the state, and assumes all deltas were already applied to it.
	deltas         []*pbsubstreams.StoreDelta // deltas are always deltas for the given block.
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
	b.deltas = nil
	b.lastOrdinal = 0
}

func (b *baseStore) bumpOrdinal(ord uint64) {
	if b.lastOrdinal > ord {
		panic("cannot Set or Del a value on a state.Builder with an ordinal lower than the previous")
	}
	b.lastOrdinal = ord
}

func (b *baseStore) ValueType() string {
	return b.valueType
}

func (b *baseStore) UpdatePolicy() pbsubstreams.Module_KindStore_UpdatePolicy {
	return b.updatePolicy
}

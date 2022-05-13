package state

import (
	"fmt"
	"sync"

	"github.com/streamingfast/dstore"
	"github.com/streamingfast/substreams/block"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type BuilderOption func(b *Builder)

type Builder struct {
	Name         string
	Store        dstore.Store
	saveInterval uint64

	ModuleStartBlock uint64
	BlockRange       *block.Range

	ModuleHash string

	info     *Info
	infoLock sync.RWMutex

	complete bool

	KV              map[string][]byte          // KV is the state, and assumes all Deltas were already applied to it.
	Deltas          []*pbsubstreams.StoreDelta // Deltas are always deltas for the given block.
	DeletedPrefixes []string

	updatePolicy pbsubstreams.Module_KindStore_UpdatePolicy
	valueType    string
	lastOrdinal  uint64
	partialMode  bool
}

func (b *Builder) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	enc.AddString("builder_name", b.Name)
	enc.AddObject("block_range", b.BlockRange)
	enc.AddBool("partial", b.partialMode)

	return nil
}

func NewBuilder(name string, saveInterval uint64, moduleStartBlock uint64, moduleHash string, updatePolicy pbsubstreams.Module_KindStore_UpdatePolicy, valueType string, store dstore.Store, opts ...BuilderOption) (*Builder, error) {
	subStore, err := store.SubStore(fmt.Sprintf("%s/states", moduleHash))
	if err != nil {
		return nil, fmt.Errorf("creating sub store: %w", err)
	}

	b := &Builder{
		Name:             name,
		KV:               make(map[string][]byte),
		updatePolicy:     updatePolicy,
		valueType:        valueType,
		Store:            subStore,
		saveInterval:     saveInterval,
		ModuleStartBlock: moduleStartBlock,
		BlockRange:       &block.Range{},
	}

	for _, opt := range opts {
		opt(b)
	}

	return b, nil
}

func (b *Builder) Print() {
	if len(b.Deltas) == 0 {
		return
	}

	fmt.Printf("State deltas for %q\n", b.Name)
	for _, delta := range b.Deltas {
		b.PrintDelta(delta)
	}
}

func (b *Builder) PrintDelta(delta *pbsubstreams.StoreDelta) {
	fmt.Printf("  %s (%d) KEY: %q\n", delta.Operation.String(), delta.Ordinal, delta.Key)
	fmt.Printf("    OLD: %s\n", string(delta.OldValue))
	fmt.Printf("    NEW: %s\n", string(delta.NewValue))
}

func (b *Builder) ApplyDelta(delta *pbsubstreams.StoreDelta) {
	// Keys need to have at least one character, and mustn't start with 0xFF
	// 0xFF is reserved for internal use.
	if delta.Key[0] == byte(255) {
		panic(fmt.Sprintf("key %q invalid, must be at least 1 character and not start with 0xFF", delta.Key))
	}

	switch delta.Operation {
	case pbsubstreams.StoreDelta_UPDATE, pbsubstreams.StoreDelta_CREATE:
		b.KV[delta.Key] = delta.NewValue
	case pbsubstreams.StoreDelta_DELETE:
		delete(b.KV, delta.Key)
	}
}

func (b *Builder) Flush() {
	zlog.Debug("flushing store", zap.String("name", b.Name), zap.Int("delta_count", len(b.Deltas)), zap.Int("entry_count", len(b.KV)))
	b.Deltas = nil
	b.lastOrdinal = 0
}

func (b *Builder) UpdateBlockRange(startBlock *uint64, endblock *uint64) {
	if startBlock != nil {
		b.BlockRange.StartBlock = *startBlock
	}

	if endblock != nil {
		b.BlockRange.ExclusiveEndBlock = *endblock
	}
}

func (b *Builder) bumpOrdinal(ord uint64) {
	if b.lastOrdinal > ord {
		panic("cannot Set or Del a value on a state.Builder with an ordinal lower than the previous")
	}
	b.lastOrdinal = ord
}

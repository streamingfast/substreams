package store

import (
	"fmt"

	"github.com/streamingfast/dstore"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type BaseStore struct {
	Config

	kv          map[string][]byte          // kv is the state, and assumes all deltas were already applied to it.
	deltas      []*pbsubstreams.StoreDelta // deltas are always deltas for the given block.
	lastOrdinal uint64

	logger *zap.Logger
}

func NewConfig(name string, moduleInitialBlock uint64, moduleHash string, updatePolicy pbsubstreams.Module_KindStore_UpdatePolicy, valueType string, store dstore.Store) (Config, error) {
	subStore, err := store.SubStore(fmt.Sprintf("%s/states", moduleHash))
	if err != nil {
		return Config{}, fmt.Errorf("creating sub store: %w", err)
	}

	return Config{
		name:               name,
		updatePolicy:       updatePolicy,
		valueType:          valueType,
		store:              subStore,
		moduleInitialBlock: moduleInitialBlock,
		moduleHash:         moduleHash,
	}, nil
}

func (c Config) NewBaseStore(logger *zap.Logger) *BaseStore {
	return &BaseStore{
		Config: c,
		kv:     make(map[string][]byte),
		logger: logger.Named("store").With(zap.String("store_name", c.name)),
	}
}

func (s *BaseStore) Name() string { return s.name }

func (s *BaseStore) InitialBlock() uint64 { return s.moduleInitialBlock }

func (s *BaseStore) String() string {
	return fmt.Sprintf("%s (%s)", s.name, s.moduleHash)
}

func (s *BaseStore) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	enc.AddString("name", s.name)
	enc.AddString("hash", s.moduleHash)
	enc.AddUint64("module_initial_block", s.moduleInitialBlock)
	enc.AddInt("key_count", len(s.kv))

	return nil
}

func (s *BaseStore) Reset() {
	if tracer.Enabled() {
		s.logger.Debug("flushing store", zap.String("name", s.name), zap.Int("delta_count", len(s.deltas)), zap.Int("entry_count", len(s.kv)))
	}
	s.deltas = nil
	s.lastOrdinal = 0
}

func (s *BaseStore) bumpOrdinal(ord uint64) {
	if s.lastOrdinal > ord {
		panic("cannot Set or Del a value on a state.Builder with an ordinal lower than the previous")
	}
	s.lastOrdinal = ord
}

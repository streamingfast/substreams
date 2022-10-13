package store

import (
	"fmt"
	"github.com/streamingfast/dstore"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type BaseStore struct {
	name       string
	moduleHash string
	store      dstore.Store

	//SaveInterval       uint64
	moduleInitialBlock uint64
	//storeInitialBlock  uint64 // block at which we initialized this store

	kv     map[string][]byte          // kv is the state, and assumes all deltas were already applied to it.
	deltas []*pbsubstreams.StoreDelta // deltas are always deltas for the given block.

	updatePolicy pbsubstreams.Module_KindStore_UpdatePolicy
	valueType    string
	lastOrdinal  uint64
	logger       *zap.Logger
}

func NewBaseStore(name string, moduleInitialBlock uint64, moduleHash string, updatePolicy pbsubstreams.Module_KindStore_UpdatePolicy, valueType string, store dstore.Store, logger *zap.Logger) (*BaseStore, error) {
	subStore, err := store.SubStore(fmt.Sprintf("%s/states", moduleHash))
	if err != nil {
		return nil, fmt.Errorf("creating sub store: %w", err)
	}

	return &BaseStore{
		name:               name,
		kv:                 make(map[string][]byte),
		updatePolicy:       updatePolicy,
		valueType:          valueType,
		store:              subStore,
		moduleHash:         moduleHash,
		moduleInitialBlock: moduleInitialBlock,
		logger:             logger.Named("store").With(zap.String("store_name", name)),
	}, nil

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

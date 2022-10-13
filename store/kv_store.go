package store

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/streamingfast/dstore"
	"github.com/streamingfast/substreams/block"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Cloneable Only the KVStore and not the partial is cloneable. We use this interface
// to make the distinction when setting up the squasher.
type Cloneable interface {
	Clone() *KVStore
}

//compile-time check that KVStore implements all interfaces
var _ Store = (*KVStore)(nil)
var _ Cloneable = (*KVStore)(nil)

type KVStore struct {
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

func NewKVStore(name string, moduleInitialBlock uint64, moduleHash string, updatePolicy pbsubstreams.Module_KindStore_UpdatePolicy, valueType string, store dstore.Store, logger *zap.Logger) (*KVStore, error) {
	subStore, err := store.SubStore(fmt.Sprintf("%s/states", moduleHash))
	if err != nil {
		return nil, fmt.Errorf("creating sub store: %w", err)
	}

	b := &KVStore{
		name:               name,
		kv:                 make(map[string][]byte),
		updatePolicy:       updatePolicy,
		valueType:          valueType,
		store:              subStore,
		moduleHash:         moduleHash,
		moduleInitialBlock: moduleInitialBlock,
		logger:             logger.Named("store").With(zap.String("store_name", name)),
	}

	return b, nil
}

func (s *KVStore) Clone() *KVStore {
	return &KVStore{
		name:               s.name,
		store:              s.store,
		moduleInitialBlock: s.moduleInitialBlock,
		moduleHash:         s.moduleHash,
		kv:                 map[string][]byte{},
		updatePolicy:       s.updatePolicy,
		valueType:          s.valueType,
		logger:             s.logger,
	}
}
func (s *KVStore) Name() string         { return s.name }
func (s *KVStore) InitialBlock() uint64 { return s.moduleInitialBlock }

func (s *KVStore) String() string {
	return fmt.Sprintf("%s (%s)", s.name, s.moduleHash)
}
func (s *KVStore) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	enc.AddString("name", s.name)
	enc.AddString("hash", s.moduleHash)
	enc.AddUint64("module_initial_block", s.moduleInitialBlock)
	enc.AddInt("key_count", len(s.kv))

	return nil
}

func (s *KVStore) storageFilename(exclusiveEndBlock uint64) string {
	return fullStateFileName(block.NewRange(s.moduleInitialBlock, exclusiveEndBlock))
}

func (s *KVStore) Load(ctx context.Context, exclusiveEndBlock uint64) error {
	fileName := s.storageFilename(exclusiveEndBlock)
	s.logger.Debug("loading full store state from file", zap.String("module_name", s.name), zap.String("fileName", fileName))

	data, err := loadStore(ctx, s.store, fileName)
	if err != nil {
		return fmt.Errorf("load full store %s at %s: %w", s.name, fileName, err)
	}

	kv := map[string][]byte{}
	if err = json.Unmarshal(data, &kv); err != nil {
		return fmt.Errorf("unmarshal data: %w", err)
	}
	s.kv = kv

	s.logger.Debug("full store loaded", zap.String("store_name", s.name), zap.String("fileName", fileName))
	return nil
}

// Save is to be called ONLY when we just passed the
// `nextExpectedBoundary` and processed nothing more after that
// boundary.
func (s *KVStore) Save(ctx context.Context, endBoundaryBlock uint64) (*block.Range, error) {
	s.logger.Debug("writing full store state", zap.Object("store", s))

	content, err := json.MarshalIndent(s.kv, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal kv state: %w", err)
	}

	filename := s.storageFilename(endBoundaryBlock)
	if err := saveStore(ctx, s.store, filename, content); err != nil {
		return nil, fmt.Errorf("write fill store %q in file %q: %w", s.name, filename, err)
	}

	brange := block.NewRange(s.moduleInitialBlock, endBoundaryBlock)
	s.logger.Info("full store state written",
		zap.String("store", s.name),
		zap.String("file_name", filename),
		zap.Object("block_range", brange),
	)

	return brange, nil
}

func (s *KVStore) DeleteStore(ctx context.Context, exclusiveEndBlock uint64) (err error) {
	filename := s.storageFilename(exclusiveEndBlock)
	zlog.Debug("deleting full store file", zap.String("file_name", filename))

	if err = s.store.DeleteObject(ctx, filename); err != nil {
		zlog.Warn("deleting  file", zap.String("file_name", filename), zap.Error(err))
	}
	return err
}

func (s *KVStore) Reset() {
	if tracer.Enabled() {
		s.logger.Debug("flushing store", zap.String("name", s.name), zap.Int("delta_count", len(s.deltas)), zap.Int("entry_count", len(s.kv)))
	}
	s.deltas = nil
	s.lastOrdinal = 0
}

func (s *KVStore) bumpOrdinal(ord uint64) {
	if s.lastOrdinal > ord {
		panic("cannot Set or Del a value on a state.Builder with an ordinal lower than the previous")
	}
	s.lastOrdinal = ord
}

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
	Name       string
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
		Name:               name,
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
		Name:               s.Name,
		store:              s.store,
		moduleInitialBlock: s.moduleInitialBlock,
		moduleHash:         s.moduleHash,
		kv:                 map[string][]byte{},
		updatePolicy:       s.updatePolicy,
		valueType:          s.valueType,
		logger:             s.logger,
	}
}

func (s *KVStore) InitialBlock() uint64 { return s.moduleInitialBlock }

func (s *KVStore) String() string {
	return fmt.Sprintf("%s (%s)", s.Name, s.moduleHash)
}
func (s *KVStore) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	enc.AddString("name", s.Name)
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
	s.logger.Debug("loading state from file", zap.String("module_name", s.Name), zap.String("fileName", fileName))

	data, err := loadStore(ctx, s.store, fileName)
	if err != nil {
		return fmt.Errorf("load store %s at %s: %w", s.Name, fileName, err)
	}

	kv := map[string][]byte{}
	if err = json.Unmarshal(data, &kv); err != nil {
		return fmt.Errorf("unmarshal data: %w", err)
	}
	s.kv = kv

	s.logger.Debug("store loaded", zap.String("store_name", s.Name), zap.String("fileName", fileName))
	return nil
}

// Save is to be called ONLY when we just passed the
// `nextExpectedBoundary` and processed nothing more after that
// boundary.
func (s *KVStore) Save(ctx context.Context, endBoundaryBlock uint64) (*block.Range, error) {
	s.logger.Debug("writing state", zap.Object("store", s))

	content, err := json.MarshalIndent(s.kv, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal kv state: %w", err)
	}

	filename := s.storageFilename(endBoundaryBlock)
	s.logger.Info("about to write state",
		zap.String("store", s.Name),
		zap.String("file_name", filename),
	)

	if err := saveStore(ctx, s.store, filename, content); err != nil {
		return nil, fmt.Errorf("write store %q in file %q: %w", s.Name, filename, err)
	}

	return block.NewRange(s.moduleInitialBlock, endBoundaryBlock), nil
}

func (s *KVStore) DeleteStore(ctx context.Context, exclusiveEndBlock uint64) *storeDeleter {
	filename := s.storageFilename(exclusiveEndBlock)

	return &storeDeleter{
		objStore: s.store,
		filename: filename,
		ctx:      ctx,
	}
}

type storeDeleter struct {
	objStore dstore.Store
	filename string
	ctx      context.Context
}

func (d *storeDeleter) Delete() error {
	zlog.Debug("deleting store file", zap.String("file_name", d.filename))
	if err := d.objStore.DeleteObject(d.ctx, d.filename); err != nil {
		zlog.Warn("deleting partial file", zap.String("filename", d.filename), zap.Error(err))
	}
	return nil
}

func (s *KVStore) Reset() {
	if tracer.Enabled() {
		s.logger.Debug("flushing store", zap.String("name", s.Name), zap.Int("delta_count", len(s.deltas)), zap.Int("entry_count", len(s.kv)))
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

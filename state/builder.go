package state

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/streamingfast/derr"
	"github.com/streamingfast/dstore"
	"github.com/streamingfast/substreams/block"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type BuilderOption func(b *Store)

type Store struct {
	Name         string
	ModuleHash   string
	Store        dstore.Store
	SaveInterval uint64

	ModuleInitialBlock   uint64
	storeInitialBlock    uint64 // block at which we initialized this store
	nextExpectedBoundary uint64 // nextExpectedBoundary is used ONLY UPON WRITING store snapshots, reading boundaries are always explicitly passed. The Squasher does NOT use this variable.

	// FIXME(abourget): rename `nextExpectedBoundary` to
	// `nextLiveBoundary`? This, in the end, is ONLY USED to write
	// snapshots while doing live processing, not in the Squasher,
	// which has its own boundary checker, and wants to handle bounds
	// that are off of its own local `saveInterval` configuration.

	KV              map[string][]byte          // KV is the state, and assumes all Deltas were already applied to it.
	Deltas          []*pbsubstreams.StoreDelta // Deltas are always deltas for the given block.
	DeletedPrefixes []string

	UpdatePolicy pbsubstreams.Module_KindStore_UpdatePolicy
	ValueType    string

	lastOrdinal uint64
}

func NewBuilder(name string, saveInterval uint64, moduleInitialBlock uint64, moduleHash string, updatePolicy pbsubstreams.Module_KindStore_UpdatePolicy, valueType string, store dstore.Store, opts ...BuilderOption) (*Store, error) {
	subStore, err := store.SubStore(fmt.Sprintf("%s/states", moduleHash))
	if err != nil {
		return nil, fmt.Errorf("creating sub store: %w", err)
	}

	b := &Store{
		Name:               name,
		KV:                 make(map[string][]byte),
		UpdatePolicy:       updatePolicy,
		ValueType:          valueType,
		Store:              subStore,
		SaveInterval:       saveInterval,
		ModuleInitialBlock: moduleInitialBlock,
		storeInitialBlock:  moduleInitialBlock,
	}
	b.resetNextBoundary()

	for _, opt := range opts {
		opt(b)
	}

	zlog.Info("store created", zap.Object("store", b))
	return b, nil
}

func (s *Store) CloneStructure(newStoreStartBlock uint64) *Store {
	store := &Store{
		Name:               s.Name,
		Store:              s.Store,
		SaveInterval:       s.SaveInterval,
		ModuleInitialBlock: s.ModuleInitialBlock,
		storeInitialBlock:  newStoreStartBlock,
		ModuleHash:         s.ModuleHash,
		KV:                 map[string][]byte{},
		UpdatePolicy:       s.UpdatePolicy,
		ValueType:          s.ValueType,
	}
	store.resetNextBoundary()
	zlog.Info("store cloned", zap.Object("store", store))
	return store
}

func (s *Store) StoreInitBlock() uint64 { return s.storeInitialBlock }
func (s *Store) NextBoundary() uint64   { return s.nextExpectedBoundary }

func (s *Store) IsPartial() bool {
	zlog.Debug("module and store initial blocks", zap.Uint64("module_initial_block", s.ModuleInitialBlock), zap.Uint64("store_initial_block", s.storeInitialBlock))
	return s.ModuleInitialBlock != s.storeInitialBlock
}

func (s *Store) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	enc.AddString("name", s.Name)
	enc.AddString("hash", s.ModuleHash)
	enc.AddUint64("store_initial_block", s.storeInitialBlock)
	enc.AddUint64("next_expected_boundary", s.nextExpectedBoundary)
	enc.AddBool("partial", s.IsPartial())

	return nil
}

func (s *Store) LoadFrom(ctx context.Context, blockRange *block.Range) (*Store, error) {
	newStore := s.CloneStructure(blockRange.StartBlock)

	if err := newStore.Fetch(ctx, blockRange.ExclusiveEndBlock); err != nil {
		return nil, err
	}

	zlog.Info("store loaded from", zap.Object("store", newStore))
	return newStore, nil
}

func (s *Store) storageFilename(exclusiveEndBlock uint64) string {
	if s.IsPartial() {
		return fmt.Sprintf("%010d-%010d.partial", exclusiveEndBlock, s.storeInitialBlock)
	} else {
		return fmt.Sprintf("%010d-%010d.kv", exclusiveEndBlock, s.storeInitialBlock)
	}
}

func (s *Store) Fetch(ctx context.Context, exclusiveEndBlock uint64) error {
	fileName := s.storageFilename(exclusiveEndBlock)
	return s.loadState(ctx, fileName)
}

func (b *Store) loadState(ctx context.Context, stateFileName string) error {
	zlog.Debug("loading state from file", zap.String("module_name", b.Name), zap.String("file_name", stateFileName))
	err := derr.RetryContext(ctx, 3, func(ctx context.Context) error {
		r, err := b.Store.OpenObject(ctx, stateFileName)
		if err != nil {
			return fmt.Errorf("openning file: %w", err)
		}
		data, err := io.ReadAll(r)
		if err != nil {
			return fmt.Errorf("reading data: %w", err)
		}
		defer r.Close()

		kv := map[string]string{}
		if err = json.Unmarshal(data, &kv); err != nil {
			return fmt.Errorf("unmarshal data: %w", err)
		}
		b.KV = byteMap(kv)
		return nil
	})
	if err != nil {
		return fmt.Errorf("storage file %s: %w", stateFileName, err)
	}

	zlog.Debug("state loaded", zap.String("builder_name", b.Name), zap.String("file_name", stateFileName))
	return nil
}

// WriteState is to be called ONLY when we just passed the
// `nextExpectedBoundary` and processed nothing more after that
// boundary.
func (s *Store) WriteState(ctx context.Context, endBoundaryBlock uint64) (err error) {
	zlog.Debug("writing state", zap.Object("builder", s))

	kv := stringMap(s.KV) // FOR READABILITY ON DISK

	content, err := json.MarshalIndent(kv, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal kv state: %w", err)
	}

	zlog.Info("about to write state",
		zap.String("store", s.Name),
		zap.Bool("partial", s.IsPartial()),
	)

	if _, err = s.writeState(ctx, content, endBoundaryBlock); err != nil {
		return fmt.Errorf("writing %s kv for range %d-%d: %w", s.Name, s.storeInitialBlock, s.nextExpectedBoundary, err)
	}

	return nil
}

func (s *Store) writeState(ctx context.Context, content []byte, endBoundaryBlock uint64) (string, error) {
	filename := s.storageFilename(endBoundaryBlock)
	err := derr.RetryContext(ctx, 3, func(ctx context.Context) error {
		return s.Store.WriteObject(ctx, filename, bytes.NewReader(content))
	})
	if err != nil {
		return filename, fmt.Errorf("writing state %s for range %d-%d: %w", s.Name, s.storeInitialBlock, s.nextExpectedBoundary, err)
	}
	return filename, err
}

func (s *Store) DeleteStore(ctx context.Context, exclusiveEndBlock uint64) error {
	filename := s.storageFilename(exclusiveEndBlock)
	zlog.Debug("deleting store file", zap.String("file_name", filename))

	if err := s.Store.DeleteObject(ctx, filename); err != nil {
		return fmt.Errorf("deleting store file %q: %w", filename, err)
	}
	return nil
}

func (s *Store) ApplyDelta(delta *pbsubstreams.StoreDelta) {
	// Keys need to have at least one character, and mustn't start with 0xFF
	// 0xFF is reserved for internal use.
	if len(delta.Key) == 0 {
		panic(fmt.Sprintf("key invalid, must be at least 1 character for module %q", s.Name))
	}
	if delta.Key[0] == byte(255) {
		panic(fmt.Sprintf("key %q invalid, must be at least 1 character and not start with 0xFF", delta.Key))
	}

	switch delta.Operation {
	case pbsubstreams.StoreDelta_UPDATE, pbsubstreams.StoreDelta_CREATE:
		s.KV[delta.Key] = delta.NewValue
	case pbsubstreams.StoreDelta_DELETE:
		delete(s.KV, delta.Key)
	}
}

func (s *Store) Flush() {
	if tracer.Enabled() {
		zlog.Debug("flushing store", zap.String("name", s.Name), zap.Int("delta_count", len(s.Deltas)), zap.Int("entry_count", len(s.KV)))
	}
	s.Deltas = nil
	s.lastOrdinal = 0
}

func (s *Store) resetNextBoundary() {
	s.nextExpectedBoundary = s.storeInitialBlock - s.storeInitialBlock%s.SaveInterval + s.SaveInterval
}

func (s *Store) Roll(lastBlock uint64) {
	s.storeInitialBlock = lastBlock
	s.KV = map[string][]byte{}
	s.resetNextBoundary()
}

func (s *Store) SetNextLiveBoundary(requestedStartBlock uint64) {
	s.nextExpectedBoundary = requestedStartBlock - requestedStartBlock%s.SaveInterval + s.SaveInterval
}

// PushBoundary to be called when the store has written its snapshot and gets ready for the next.
func (s *Store) PushBoundary() {
	s.nextExpectedBoundary += s.SaveInterval
}

func (s *Store) bumpOrdinal(ord uint64) {
	if s.lastOrdinal > ord {
		panic("cannot Set or Del a value on a state.Builder with an ordinal lower than the previous")
	}
	s.lastOrdinal = ord
}

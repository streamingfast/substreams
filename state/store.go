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

type Store struct {
	Name         string
	ModuleHash   string
	Store        dstore.Store
	SaveInterval uint64

	ModuleInitialBlock uint64
	storeInitialBlock  uint64 // block at which we initialized this store

	KV              map[string][]byte          // KV is the state, and assumes all Deltas were already applied to it.
	Deltas          []*pbsubstreams.StoreDelta // Deltas are always deltas for the given block.
	DeletedPrefixes []string

	UpdatePolicy pbsubstreams.Module_KindStore_UpdatePolicy
	ValueType    string

	lastOrdinal uint64
	logger      *zap.Logger
}

func NewStore(name string, saveInterval uint64, moduleInitialBlock uint64, moduleHash string, updatePolicy pbsubstreams.Module_KindStore_UpdatePolicy, valueType string, store dstore.Store, logger *zap.Logger) (*Store, error) {
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
		ModuleHash:         moduleHash,
		SaveInterval:       saveInterval,
		ModuleInitialBlock: moduleInitialBlock,
		storeInitialBlock:  moduleInitialBlock,
		logger:             logger.Named("store"),
	}

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
		logger:             s.logger,
	}
	//store.resetNextBoundary()
	s.logger.Info("store cloned", zap.Object("store", store))
	return store
}

func (s *Store) StoreInitialBlock() uint64 { return s.storeInitialBlock }

func (s *Store) IsPartial() bool {
	//s.logger.Debug("module and store initial blocks", zap.Uint64("module_initial_block", s.ModuleInitialBlock), zap.Uint64("store_initial_block", s.storeInitialBlock))
	return s.ModuleInitialBlock != s.storeInitialBlock
}

func (s *Store) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	enc.AddString("name", s.Name)
	enc.AddString("hash", s.ModuleHash)
	enc.AddUint64("module_initial_block", s.ModuleInitialBlock)
	enc.AddUint64("store_initial_block", s.storeInitialBlock)
	//enc.AddUint64("next_expected_boundary", s.nextExpectedBoundary)
	enc.AddBool("partial", s.IsPartial())
	enc.AddInt("key_count", len(s.KV))

	return nil
}

func (s *Store) LoadFrom(ctx context.Context, blockRange *block.Range) (*Store, error) {
	newStore := s.CloneStructure(blockRange.StartBlock)

	if err := newStore.Fetch(ctx, blockRange.ExclusiveEndBlock); err != nil {
		return nil, err
	}

	s.logger.Info("store loaded from", zap.Object("store", newStore))
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
	return s.load(ctx, fileName)
}

func (s *Store) load(ctx context.Context, stateFileName string) error {
	s.logger.Debug("loading state from file", zap.String("module_name", s.Name), zap.String("file_name", stateFileName))
	err := derr.RetryContext(ctx, 3, func(ctx context.Context) error {
		r, err := s.Store.OpenObject(ctx, stateFileName)
		if err != nil {
			return fmt.Errorf("openning file: %w", err)
		}
		data, err := io.ReadAll(r)
		if err != nil {
			return fmt.Errorf("reading data: %w", err)
		}
		defer r.Close()

		kv := map[string][]byte{}
		if err = json.Unmarshal(data, &kv); err != nil {
			return fmt.Errorf("unmarshal data: %w", err)
		}
		s.KV = kv

		s.logger.Debug("unmarshalling kv", zap.String("file_name", stateFileName), zap.Object("store", s))
		return nil
	})
	if err != nil {
		return fmt.Errorf("storage file %s: %w", stateFileName, err)
	}

	s.logger.Debug("state loaded", zap.String("store_name", s.Name), zap.String("file_name", stateFileName))
	return nil
}

// WriteState is to be called ONLY when we just passed the
// `nextExpectedBoundary` and processed nothing more after that
// boundary.
func (s *Store) WriteState(ctx context.Context, endBoundaryBlock uint64) (*storeWriter, error) {
	s.logger.Debug("writing state", zap.Object("store", s))

	//kv := stringMap(s.KV) // FOR READABILITY ON DISK

	content, err := json.MarshalIndent(s.KV, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal kv state: %w", err)
	}

	filename := s.storageFilename(endBoundaryBlock)
	s.logger.Info("about to write state",
		zap.String("store", s.Name),
		zap.Bool("partial", s.IsPartial()),
		zap.String("file_name", filename),
	)

	sw := &storeWriter{
		objStore:     s.Store,
		filename:     filename,
		content:      content,
		initialBlock: s.storeInitialBlock,
		endBoundary:  endBoundaryBlock,
		moduleName:   s.Name,
		ctx:          ctx,
	}

	return sw, nil
}

type storeWriter struct {
	objStore     dstore.Store
	filename     string
	content      []byte
	initialBlock uint64
	endBoundary  uint64
	moduleName   string
	ctx          context.Context
}

func (w *storeWriter) Write() error {
	err := derr.RetryContext(w.ctx, 3, func(ctx context.Context) error {
		return w.objStore.WriteObject(ctx, w.filename, bytes.NewReader(w.content))
	})
	if err != nil {
		return fmt.Errorf("writing state %s for range %d-%d: %w", w.moduleName, w.initialBlock, w.endBoundary, err)
	}
	return nil
}

func (s *Store) DeleteStore(ctx context.Context, exclusiveEndBlock uint64) *storeDeleter {
	filename := s.storageFilename(exclusiveEndBlock)

	return &storeDeleter{
		objStore: s.Store,
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

func (s *Store) ApplyDelta(delta *pbsubstreams.StoreDelta) {
	// Keys need to have at least one character, and mustn't start with 0xFF is reserved for internal use.
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

func (s *Store) ApplyDeltaReverse(deltas []*pbsubstreams.StoreDelta) {
	for i := len(deltas) - 1; i >= 0; i-- {
		delta := deltas[i]
		switch delta.Operation {
		case pbsubstreams.StoreDelta_UPDATE, pbsubstreams.StoreDelta_DELETE:
			s.KV[delta.Key] = delta.OldValue
		case pbsubstreams.StoreDelta_CREATE:
			delete(s.KV, delta.Key)
		}
	}
}

func (s *Store) Flush() {
	if tracer.Enabled() {
		s.logger.Debug("flushing store", zap.String("name", s.Name), zap.Int("delta_count", len(s.Deltas)), zap.Int("entry_count", len(s.KV)))
	}
	s.Deltas = nil
	s.lastOrdinal = 0
}

// func (s *Store) resetNextBoundary() {
// 	s.nextExpectedBoundary = s.storeInitialBlock - s.storeInitialBlock%s.SaveInterval + s.SaveInterval
// }

// func (s *Store) NextBoundary() uint64 { return s.nextExpectedBoundary }

func (s *Store) Roll(lastBlock uint64) {
	s.storeInitialBlock = lastBlock
	s.KV = map[string][]byte{}
}

// func (s *Store) SetNextLiveBoundary(requestedStartBlock uint64) {
// 	s.nextExpectedBoundary = requestedStartBlock - requestedStartBlock%s.SaveInterval + s.SaveInterval
// }

// // PushBoundary to be called when the store has written its snapshot and gets ready for the next.
// func (s *Store) PushBoundary() {
// 	s.nextExpectedBoundary += s.SaveInterval
// }

func (s *Store) bumpOrdinal(ord uint64) {
	if s.lastOrdinal > ord {
		panic("cannot Set or Del a value on a state.Builder with an ordinal lower than the previous")
	}
	s.lastOrdinal = ord
}

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
	"github.com/streamingfast/substreams/pipeline/outputs"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"google.golang.org/protobuf/proto"
)

type BuilderOption func(b *Store)

type Store struct {
	Name         string
	ModuleHash   string
	Store        dstore.Store
	SaveInterval uint64

	ModuleInitialBlock uint64
	//StoreInitialBlock  uint64 // block at which we initialized this store
	BlockRange     *block.Range
	ProcessedBlock uint64

	KV              map[string][]byte          // KV is the state, and assumes all Deltas were already applied to it.
	Deltas          []*pbsubstreams.StoreDelta // Deltas are always deltas for the given block.
	DeletedPrefixes []string

	UpdatePolicy pbsubstreams.Module_KindStore_UpdatePolicy
	ValueType    string

	lastOrdinal uint64
}

func (s *Store) IsPartial() bool {
	zlog.Debug("module and store initial blocks", zap.Uint64("module_initial_block", s.ModuleInitialBlock), zap.Object("range", s.BlockRange))
	return s.ModuleInitialBlock != s.BlockRange.StartBlock
}

func (s *Store) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	enc.AddString("name", s.Name)
	enc.AddString("hash", s.ModuleHash)
	enc.AddBool("partial", s.IsPartial())
	err := enc.AddObject("range", s.BlockRange)
	if err != nil {
		return err
	}
	return nil
}

func NewBuilder(name string, saveInterval uint64, moduleInitialBlock uint64, moduleHash string, updatePolicy pbsubstreams.Module_KindStore_UpdatePolicy, valueType string, store dstore.Store, opts ...BuilderOption) (*Store, error) {
	subStore, err := store.SubStore(fmt.Sprintf("%s/states", moduleHash))
	if err != nil {
		return nil, fmt.Errorf("creating sub store: %w", err)
	}
	blockRange := block.NewRange(moduleInitialBlock, moduleInitialBlock+saveInterval)
	b := &Store{
		Name:               name,
		KV:                 make(map[string][]byte),
		UpdatePolicy:       updatePolicy,
		ValueType:          valueType,
		Store:              subStore,
		SaveInterval:       saveInterval,
		ModuleInitialBlock: moduleInitialBlock,
		BlockRange:         blockRange,
	}

	for _, opt := range opts {
		opt(b)
	}

	zlog.Info("store created", zap.Object("store", b))
	return b, nil
}

func (s *Store) CloneStructure(blockRange *block.Range) *Store {
	store := &Store{
		Name:               s.Name,
		Store:              s.Store,
		SaveInterval:       s.SaveInterval,
		ModuleInitialBlock: s.ModuleInitialBlock,
		BlockRange:         blockRange,
		ModuleHash:         s.ModuleHash,
		KV:                 map[string][]byte{},
		UpdatePolicy:       s.UpdatePolicy,
		ValueType:          s.ValueType,
	}
	zlog.Info("store cloned", zap.Object("store", store))
	return store
}

func (s *Store) LoadFrom(ctx context.Context, blockRange *block.Range) (*Store, error) {
	newStore := s.CloneStructure(blockRange)

	if err := newStore.LoadState(ctx); err != nil {
		return nil, err
	}

	zlog.Info("store loaded from", zap.Object("store", newStore))
	return newStore, nil
}

func (s *Store) storageFilename() string {
	if s.IsPartial() {
		return fmt.Sprintf("%010d-%010d.partial", s.BlockRange.ExclusiveEndBlock, s.BlockRange.StartBlock)
	} else {
		return fmt.Sprintf("%010d-%010d.kv", s.BlockRange.ExclusiveEndBlock, s.BlockRange.StartBlock)
	}
}

func (s *Store) LoadState(ctx context.Context) error {
	stateFileName := s.storageFilename()
	zlog.Debug("loading state from file", zap.String("module_name", s.Name), zap.String("file_name", stateFileName))
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

		kv := map[string]string{}
		if err = json.Unmarshal(data, &kv); err != nil {
			return fmt.Errorf("unmarshal data: %w", err)
		}
		s.KV = byteMap(kv)
		return nil
	})
	if err != nil {
		return fmt.Errorf("storage file %s: %w", stateFileName, err)
	}

	zlog.Debug("state loaded", zap.String("builder_name", s.Name), zap.String("file_name", stateFileName))
	return nil
}

func (s *Store) loadDeltas(ctx context.Context, fromBlock, exclusiveStopBlock uint64, outputCacheSaveInterval uint64, outputCacheStore dstore.Store) error {
	if s.IsPartial() {
		panic("cannot load deltas in partial mode")
	}

	startBlockNum := outputs.ComputeStartBlock(fromBlock, outputCacheSaveInterval)
	outputCache := outputs.NewOutputCache(s.Name, outputCacheStore, 0)

	zlog.Debug("loading delta",
		zap.String("builder_name", s.Name),
		zap.Uint64("from_block", fromBlock),
		zap.Uint64("start_block", startBlockNum),
		zap.Uint64("stop_block", exclusiveStopBlock),
		zap.Stringer("output_cache", outputCache),
	)

	found, err := outputCache.Load(ctx, startBlockNum)
	if err != nil {
		return fmt.Errorf("loading init cache for builder %q with start block %d: %w", s.Name, startBlockNum, err)
	}

	for {
		if !found {
			return fmt.Errorf("missing deltas for module %q", s.Name)
		}
		cacheItems := outputCache.SortedCacheItems()

		firstSeenBlockNum := uint64(0)
		lastSeenBlockNum := uint64(0)

		for _, item := range cacheItems {
			deltas := &pbsubstreams.StoreDeltas{}
			err := proto.Unmarshal(item.Payload, deltas)
			if err != nil {
				return fmt.Errorf("unmarshalling output deltas: %w", err)
			}

			for _, delta := range deltas.Deltas {
				//todo: we should check the from block?
				if item.BlockNum >= exclusiveStopBlock {
					return nil //all good we reach the end
				}
				if firstSeenBlockNum == uint64(0) {
					firstSeenBlockNum = item.BlockNum
				}
				lastSeenBlockNum = item.BlockNum
				if delta.Key == "" {
					panic("missing key, invalid delta")
				}
				// FIXME(abourget): this never did anything.. soooo what's the goal here? :)
				s.Deltas = append(s.Deltas, delta)
			}
		}

		zlog.Debug("loaded deltas", zap.String("builder_name", s.Name), zap.Uint64("from_block_num", firstSeenBlockNum), zap.Uint64("to_block_num", lastSeenBlockNum))

		if exclusiveStopBlock <= outputCache.CurrentBlockRange.ExclusiveEndBlock {
			return nil
		}
		found, err = outputCache.Load(ctx, outputCache.CurrentBlockRange.ExclusiveEndBlock)
		if err != nil {
			return fmt.Errorf("loading more deltas: %w", err)
		}
	}
}

func (s *Store) WriteState(ctx context.Context) (err error) {
	zlog.Debug("writing state", zap.Object("builder", s), zap.Object("range", s.BlockRange))

	kv := stringMap(s.KV) // FOR READABILITY ON DISK

	content, err := json.MarshalIndent(kv, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal kv state: %w", err)
	}

	zlog.Info("about to write state",
		zap.String("store", s.Name),
		zap.Bool("partial", s.IsPartial()),
	)

	if _, err = s.writeState(ctx, content); err != nil {
		return fmt.Errorf("writing %s kv for range %d-%d: %w", s.Name, s.BlockRange.StartBlock, s.BlockRange.ExclusiveEndBlock, err)
	}

	return nil
}

func (s *Store) writeState(ctx context.Context, content []byte) (string, error) {
	filename := s.storageFilename()
	zlog.Info("writing state", zap.Object("store", s), zap.Object("range", s.BlockRange), zap.String("file_name", filename))

	err := derr.RetryContext(ctx, 3, func(ctx context.Context) error {
		return s.Store.WriteObject(ctx, filename, bytes.NewReader(content))
	})
	if err != nil {
		return filename, fmt.Errorf("writing state %s for range %d-%d: %w", s.Name, s.BlockRange.StartBlock, s.BlockRange.ExclusiveEndBlock, err)
	}

	// FIXME(abourget): not needed when we don't use that state file anymore
	// endsOnBoundary := lastBlock%s.SaveInterval == 0
	// if !s.IsPartial() && endsOnBoundary {
	// 	if err := s.writeInfoState(ctx, filename, lastBlock); err != nil {
	// 		return filename, fmt.Errorf("writing info state: %w", err)
	// 	}
	// }

	return filename, err
}

func (s *Store) DeleteStore(ctx context.Context) error {
	filename := s.storageFilename()
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

func (s *Store) Roll(rollStart bool) {
	if rollStart {
		s.BlockRange.StartBlock = s.BlockRange.ExclusiveEndBlock
	}
	s.BlockRange.ExclusiveEndBlock += s.SaveInterval
}

func (s *Store) Truncate() {
	s.KV = map[string][]byte{}
}

func (s *Store) bumpOrdinal(ord uint64) {
	if s.lastOrdinal > ord {
		panic("cannot Set or Del a value on a state.Builder with an ordinal lower than the previous")
	}
	s.lastOrdinal = ord
}

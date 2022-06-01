package state

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync"

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
	Store        dstore.Store
	SaveInterval uint64
	Initialized  bool

	ModuleInitialBlock uint64
	StoreInitialBlock  uint64 // block at which we initialized this store
	ProcessedBlock     uint64

	ModuleHash string

	info     *Info
	infoLock sync.RWMutex

	complete bool

	KV              map[string][]byte          // KV is the state, and assumes all Deltas were already applied to it.
	Deltas          []*pbsubstreams.StoreDelta // Deltas are always deltas for the given block.
	DeletedPrefixes []string

	UpdatePolicy pbsubstreams.Module_KindStore_UpdatePolicy
	ValueType    string

	lastOrdinal uint64
}

func (b *Store) IsPartial() bool {
	zlog.Debug("module and store initial blocks", zap.Uint64("module_initial_block", b.ModuleInitialBlock), zap.Uint64("store_initial_block", b.StoreInitialBlock))
	//fixme: this seems to cause an issue when we run ethtokens:tokens -s 6810707 -t +1
	// we will create a subrequest which is from 6810706 to 6810707 and it will not detect
	// that we are in partial mode, because we only check the starting point
	// and will try to get a [...].kv file instead of a [...].partial file
	return b.ModuleInitialBlock != b.StoreInitialBlock
}

func (b *Store) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	enc.AddString("builder_name", b.Name)
	enc.AddBool("partial", b.IsPartial())

	return nil
}

// func (b *Store) FromBlockRange(blockRange *block.Range) *Store {
// 	return &Store{
// 		Name:               b.Name,
// 		Store:              b.Store,
// 		SaveInterval:       b.SaveInterval,
// 		ModuleInitialBlock: b.ModuleInitialBlock,
// 		BlockRange:         blockRange,
// 		ModuleHash:         b.ModuleHash,
// 		KV:                 map[string][]byte{},
// 		Deltas:             []*pbsubstreams.StoreDelta{},
// 		UpdatePolicy:       b.UpdatePolicy,
// 		ValueType:          b.ValueType,
// 	}
// }

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
		StoreInitialBlock:  moduleInitialBlock,
	}

	for _, opt := range opts {
		opt(b)
	}

	return b, nil
}

func (b *Store) Print() {
	if len(b.Deltas) == 0 {
		return
	}

	fmt.Printf("State deltas for %q\n", b.Name)
	for _, delta := range b.Deltas {
		b.PrintDelta(delta)
	}
}

func (b *Store) Clone(newStoreStartBlock uint64) *Store {
	s := &Store{
		Name:               b.Name,
		Store:              b.Store,
		SaveInterval:       b.SaveInterval,
		ModuleInitialBlock: b.ModuleInitialBlock,
		StoreInitialBlock:  newStoreStartBlock,
		ModuleHash:         b.ModuleHash,
		KV:                 map[string][]byte{},
		Deltas:             []*pbsubstreams.StoreDelta{},
		UpdatePolicy:       b.UpdatePolicy,
		ValueType:          b.ValueType,
	}
	return s
}

func (b *Store) LoadFrom(ctx context.Context, blockRange *block.Range) (*Store, error) {
	newStore := b.Clone(blockRange.StartBlock)

	if err := newStore.Fetch(ctx, blockRange.ExclusiveEndBlock); err != nil {
		return nil, err
	}

	return newStore, nil
}

func (s *Store) storageFilename(exclusiveEndBlock uint64) string {
	if s.IsPartial() {
		return fmt.Sprintf("%010d-%010d.partial", exclusiveEndBlock, s.StoreInitialBlock)
	} else {
		return fmt.Sprintf("%010d-%010d.kv", exclusiveEndBlock, s.StoreInitialBlock)
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
		b.Initialized = true
		return nil
	})
	if err != nil {
		return fmt.Errorf("storage file %s: %w", stateFileName, err)
	}

	zlog.Debug("state loaded", zap.String("builder_name", b.Name), zap.String("file_name", stateFileName))
	return nil
}

func (b *Store) loadDeltas(ctx context.Context, fromBlock, exclusiveStopBlock uint64, outputCacheSaveInterval uint64, outputCacheStore dstore.Store) error {
	if b.IsPartial() {
		panic("cannot load deltas in partial mode")
	}

	startBlockNum := outputs.ComputeStartBlock(fromBlock, outputCacheSaveInterval)
	outputCache := outputs.NewOutputCache(b.Name, outputCacheStore, 0)

	zlog.Debug("loading delta",
		zap.String("builder_name", b.Name),
		zap.Uint64("from_block", fromBlock),
		zap.Uint64("start_block", startBlockNum),
		zap.Uint64("stop_block", exclusiveStopBlock),
		zap.Stringer("output_cache", outputCache),
	)

	found, err := outputCache.Load(ctx, startBlockNum)
	if err != nil {
		return fmt.Errorf("loading init cache for builder %q with start block %d: %w", b.Name, startBlockNum, err)
	}

	for {
		if !found {
			return fmt.Errorf("missing deltas for module %q", b.Name)
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
				b.Deltas = append(b.Deltas, delta)
			}
		}

		zlog.Debug("loaded deltas", zap.String("builder_name", b.Name), zap.Uint64("from_block_num", firstSeenBlockNum), zap.Uint64("to_block_num", lastSeenBlockNum))

		if exclusiveStopBlock <= outputCache.CurrentBlockRange.ExclusiveEndBlock {
			return nil
		}
		found, err = outputCache.Load(ctx, outputCache.CurrentBlockRange.ExclusiveEndBlock)
		if err != nil {
			return fmt.Errorf("loading more deltas: %w", err)
		}
	}
}

func (b *Store) WriteState(ctx context.Context, lastBlock uint64) (err error) {
	zlog.Debug("writing state", zap.Object("builder", b), zap.Uint64("last_block", lastBlock))

	err = b.writeMergeData()
	if err != nil {
		return fmt.Errorf("writing merge values: %w", err)
	}

	kv := stringMap(b.KV) // FOR READABILITY ON DISK

	content, err := json.MarshalIndent(kv, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal kv state: %w", err)
	}

	zlog.Info("about to write state",
		zap.String("store", b.Name),
		zap.Bool("partial", b.IsPartial()),
	)

	if _, err = b.writeState(ctx, content, lastBlock); err != nil {
		return fmt.Errorf("writing %s kv for range %d-%d: %w", b.Name, b.StoreInitialBlock, lastBlock, err)
	}

	return nil
}

func (b *Store) writeState(ctx context.Context, content []byte, lastBlock uint64) (string, error) {
	// FIXME(abourget): `lastBlock` is ASSUMED TO BE ON THE BOUNDARY
	filename := b.storageFilename(lastBlock)
	err := b.Store.WriteObject(ctx, filename, bytes.NewReader(content))
	if err != nil {
		return filename, fmt.Errorf("writing state %s for range %d-%d: %w", b.Name, b.StoreInitialBlock, lastBlock, err)
	}

	if !b.IsPartial() {
		if err := b.writeInfoState(ctx, filename, lastBlock); err != nil {
			return filename, fmt.Errorf("writing info state: %w", err)
		}
	}

	return filename, err
}

func (b *Store) writeInfoState(ctx context.Context, filename string, lastBlock uint64) error {
	// FIXME(abourget): `lastBlock` is ASSUMED TO BE ON THE BOUNDARY

	currentInfo, err := b.Info(ctx)
	if err != nil {
		return fmt.Errorf("getting builder info: %w", err)
	}

	if currentInfo != nil && currentInfo.LastKVSavedBlock >= lastBlock {
		zlog.Debug("skipping info save.")
		return nil
	}

	var info = &Info{
		LastKVFile:        filename,
		LastKVSavedBlock:  lastBlock,
		RangeIntervalSize: b.SaveInterval,
	}
	err = writeStateInfo(ctx, b.Store, info)
	if err != nil {
		return fmt.Errorf("writing state info for builder %q: %w", b.Name, err)
	}

	b.info = info
	zlog.Debug("state file written", zap.String("module_name", b.Name), zap.Uint64("last_block", lastBlock), zap.String("file_name", filename))

	return nil
}

// 	zlog.Debug("writing partial state", zap.String("module_name", b.Name), zap.Object("range", b.BlockRange), zap.String("file_name", filename))
// 	return filename, b.Store.WriteObject(ctx, filename, bytes.NewReader(content))
// }

func (b *Store) DeleteStore(ctx context.Context, exclusiveEndBlock uint64) error {
	filename := b.storageFilename(exclusiveEndBlock)
	zlog.Debug("deleting store file", zap.String("file_name", filename))

	if err := b.Store.DeleteObject(ctx, filename); err != nil {
		return fmt.Errorf("deleting store file %q: %w", filename, err)
	}
	return nil
}

func (b *Store) PrintDelta(delta *pbsubstreams.StoreDelta) {
	fmt.Printf("  %s (%d) KEY: %q\n", delta.Operation.String(), delta.Ordinal, delta.Key)
	fmt.Printf("    OLD: %s\n", string(delta.OldValue))
	fmt.Printf("    NEW: %s\n", string(delta.NewValue))
}

func (b *Store) ApplyDelta(delta *pbsubstreams.StoreDelta) {
	// Keys need to have at least one character, and mustn't start with 0xFF
	// 0xFF is reserved for internal use.
	if len(delta.Key) == 0 {
		panic(fmt.Sprintf("key invalid, must be at least 1 character for module %q", b.Name))
	}
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

func (b *Store) Flush() {
	if tracer.Enabled() {
		zlog.Debug("flushing store", zap.String("name", b.Name), zap.Int("delta_count", len(b.Deltas)), zap.Int("entry_count", len(b.KV)))
	}
	b.Deltas = nil
	b.lastOrdinal = 0
}

func (b *Store) Roll(lastBlock uint64) {
	b.StoreInitialBlock = lastBlock
}

func (b *Store) Truncate() {
	b.KV = map[string][]byte{}
}

func (b *Store) bumpOrdinal(ord uint64) {
	if b.lastOrdinal > ord {
		panic("cannot Set or Del a value on a state.Builder with an ordinal lower than the previous")
	}
	b.lastOrdinal = ord
}

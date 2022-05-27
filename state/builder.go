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

type BuilderOption func(b *Builder)

type Builder struct {
	Name         string
	Store        dstore.Store
	SaveInterval uint64
	Initialized  bool

	ModuleStartBlock uint64
	BlockRange       *block.Range

	ModuleHash string

	info     *Info
	infoLock sync.RWMutex

	complete bool

	KV              map[string][]byte          // KV is the state, and assumes all Deltas were already applied to it.
	Deltas          []*pbsubstreams.StoreDelta // Deltas are always deltas for the given block.
	DeletedPrefixes []string

	UpdatePolicy pbsubstreams.Module_KindStore_UpdatePolicy
	ValueType    string
	PartialMode  bool

	lastOrdinal uint64
}

func (b *Builder) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	enc.AddString("builder_name", b.Name)
	enc.AddBool("partial", b.PartialMode)
	err := enc.AddObject("block_range", b.BlockRange)
	if err != nil {
		return err
	}

	return nil
}

func (b *Builder) FromBlockRange(blockRange *block.Range, partialMode bool) *Builder {
	return &Builder{
		Name:             b.Name,
		Store:            b.Store,
		SaveInterval:     b.SaveInterval,
		ModuleStartBlock: b.ModuleStartBlock,
		BlockRange:       blockRange,
		ModuleHash:       b.ModuleHash,
		KV:               map[string][]byte{},
		Deltas:           []*pbsubstreams.StoreDelta{},
		UpdatePolicy:     b.UpdatePolicy,
		ValueType:        b.ValueType,
		PartialMode:      partialMode,
	}
}

func NewBuilder(name string, saveInterval uint64, moduleStartBlock uint64, moduleHash string, updatePolicy pbsubstreams.Module_KindStore_UpdatePolicy, valueType string, store dstore.Store, opts ...BuilderOption) (*Builder, error) {
	subStore, err := store.SubStore(fmt.Sprintf("%s/states", moduleHash))
	if err != nil {
		return nil, fmt.Errorf("creating sub store: %w", err)
	}

	b := &Builder{
		Name:             name,
		KV:               make(map[string][]byte),
		UpdatePolicy:     updatePolicy,
		ValueType:        valueType,
		Store:            subStore,
		SaveInterval:     saveInterval,
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

func (b *Builder) InitializePartial(ctx context.Context, startBlock uint64) error {
	b.PartialMode = true
	floor := startBlock - startBlock%b.SaveInterval
	exclusiveEndBlock := floor + b.SaveInterval
	if startBlock == b.ModuleStartBlock {
		floor = b.ModuleStartBlock
	}
	b.BlockRange = &block.Range{
		StartBlock:        floor,
		ExclusiveEndBlock: exclusiveEndBlock,
	}

	fileName := PartialFileName(b.BlockRange)

	found, err := b.Store.FileExists(ctx, fileName)
	if err != nil {
		return fmt.Errorf("searching for filename %s: %w", fileName, err)
	}

	if !found {
		b.KV = byteMap(map[string]string{})
		return nil
	}
	b.Initialized = true
	return b.loadState(ctx, fileName)
}

func (b *Builder) Initialize(ctx context.Context, requestedStartBlock uint64, outputCacheSaveInterval uint64, outputCacheStore dstore.Store) error {
	b.BlockRange.StartBlock = b.ModuleStartBlock
	b.Initialized = true

	zlog.Debug("initializing builder", zap.String("module_name", b.Name), zap.Uint64("requested_start_block", requestedStartBlock))
	floor := requestedStartBlock - requestedStartBlock%b.SaveInterval
	if requestedStartBlock == b.BlockRange.StartBlock {
		b.BlockRange.StartBlock = requestedStartBlock
		b.BlockRange.ExclusiveEndBlock = floor + b.SaveInterval
		b.KV = map[string][]byte{}
		return nil
	}

	deltasStartBlock := uint64(0)

	zlog.Debug("computed info", zap.String("module_name", b.Name), zap.Uint64("start_block", floor))

	deltasNeeded := true
	deltasStartBlock = b.ModuleStartBlock
	b.BlockRange.ExclusiveEndBlock = floor + b.SaveInterval
	if floor >= b.SaveInterval && floor > b.BlockRange.StartBlock {
		deltasStartBlock = floor
		deltasNeeded = (requestedStartBlock - floor) > 0

		atBlock := floor - b.SaveInterval // get the previous saved range
		b.BlockRange.ExclusiveEndBlock = floor
		fileName := FullStateFileName(&block.Range{
			StartBlock:        b.ModuleStartBlock,
			ExclusiveEndBlock: b.BlockRange.ExclusiveEndBlock,
		}, b.ModuleStartBlock)

		zlog.Info("about to load state", zap.String("module_name", b.Name), zap.Uint64("at_block", atBlock), zap.Uint64("deltas_start_block", deltasStartBlock))
		err := b.loadState(ctx, fileName)
		if err != nil {
			return fmt.Errorf("reading state file for module %q: %w", b.Name, err)
		}
	}
	if deltasNeeded {
		err := b.loadDelta(ctx, deltasStartBlock, requestedStartBlock, outputCacheSaveInterval, outputCacheStore)
		if err != nil {
			return fmt.Errorf("loading delta for builder %q: %w", b.Name, err)
		}
	}

	return nil
}

func (b *Builder) loadState(ctx context.Context, stateFileName string) error {
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
			return fmt.Errorf("json unmarshal of state file %s data: %w", stateFileName, err)
		}
		b.KV = byteMap(kv)
		return nil
	})

	if err != nil {
		return fmt.Errorf("opening file state file %s: %w", stateFileName, err)
	}

	zlog.Debug("state loaded", zap.String("builder_name", b.Name), zap.String("file_name", stateFileName))
	return nil
}

func (b *Builder) loadDelta(ctx context.Context, fromBlock, exclusiveStopBlock uint64, outputCacheSaveInterval uint64, outputCacheStore dstore.Store) error {
	if b.PartialMode {
		panic("cannot load a state in partial mode")
	}

	zlog.Debug("loading delta",
		zap.String("builder_name", b.Name),
		zap.Uint64("from_block", fromBlock),
		zap.Uint64("stop_block", exclusiveStopBlock),
	)

	startBlockNum := outputs.ComputeStartBlock(fromBlock, outputCacheSaveInterval)
	outputCache := outputs.NewOutputCache(b.Name, outputCacheStore, 0)

	err := outputCache.Load(ctx, startBlockNum)
	if err != nil {
		return fmt.Errorf("loading init cache for builder %q with start block %d: %w", b.Name, startBlockNum, err)
	}

	for {
		cacheItems := outputCache.SortedCacheItems()
		if len(cacheItems) == 0 {
			return fmt.Errorf("missing deltas for module %q", b.Name)
		}

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
				b.Deltas = append(b.Deltas, delta)
			}
		}

		zlog.Debug("loaded deltas", zap.String("builder_name", b.Name), zap.Uint64("from_block_num", firstSeenBlockNum), zap.Uint64("to_block_num", lastSeenBlockNum))

		if exclusiveStopBlock <= outputCache.CurrentBlockRange.ExclusiveEndBlock {
			return nil
		}
		err := outputCache.Load(ctx, outputCache.CurrentBlockRange.ExclusiveEndBlock)
		if err != nil {
			return fmt.Errorf("loading more deltas: %w", err)
		}
	}
}

func (b *Builder) WriteState(ctx context.Context) (err error) {
	zlog.Debug("writing state", zap.Object("builder", b))

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
		zap.Bool("partial", b.PartialMode),
		zap.Object("block_range", b.BlockRange),
	)

	if b.PartialMode {
		_, err = b.writePartialState(ctx, content)
	} else {
		_, err = b.writeState(ctx, content)
	}

	if err != nil {
		return fmt.Errorf("writing %s kv for range %s: %w", b.Name, b.BlockRange, err)
	}

	return nil
}

func (b *Builder) writeState(ctx context.Context, content []byte) (string, error) {
	filename := FullStateFileName(b.BlockRange, b.ModuleStartBlock)
	err := b.Store.WriteObject(ctx, filename, bytes.NewReader(content))
	if err != nil {
		return filename, fmt.Errorf("writing state %s for range %s: %w", b.Name, b.BlockRange.String(), err)
	}

	currentInfo, err := b.Info(ctx)
	if err != nil {
		return "", fmt.Errorf("getting builder info: %w", err)
	}

	if currentInfo != nil && currentInfo.LastKVSavedBlock >= b.BlockRange.ExclusiveEndBlock {
		zlog.Debug("skipping info save.")
		return filename, nil
	}

	var info = &Info{
		LastKVFile:        filename,
		LastKVSavedBlock:  b.BlockRange.ExclusiveEndBlock,
		RangeIntervalSize: b.SaveInterval,
	}
	err = writeStateInfo(ctx, b.Store, info)
	if err != nil {
		return "", fmt.Errorf("writing state info for builder %q: %w", b.Name, err)
	}

	b.info = info
	zlog.Debug("state file written", zap.String("module_name", b.Name), zap.Object("block_range", b.BlockRange), zap.String("file_name", filename))

	return filename, err
}

func (b *Builder) writePartialState(ctx context.Context, content []byte) (string, error) {
	filename := PartialFileName(b.BlockRange)
	zlog.Debug("writing partial state", zap.String("module_name", b.Name), zap.Object("range", b.BlockRange), zap.String("file_name", filename))
	return filename, b.Store.WriteObject(ctx, filename, bytes.NewReader(content))
}

func (b *Builder) DeletePartialFile(ctx context.Context) error {
	filename := PartialFileName(b.BlockRange)
	zlog.Debug("deleting partial file", zap.String("file_name", filename))
	err := b.Store.DeleteObject(ctx, filename)
	if err != nil {
		return fmt.Errorf("deleting partial file %q: %w", filename, err)
	}
	return nil
}

func (b *Builder) PrintDelta(delta *pbsubstreams.StoreDelta) {
	fmt.Printf("  %s (%d) KEY: %q\n", delta.Operation.String(), delta.Ordinal, delta.Key)
	fmt.Printf("    OLD: %s\n", string(delta.OldValue))
	fmt.Printf("    NEW: %s\n", string(delta.NewValue))
}

func (b *Builder) ApplyDelta(delta *pbsubstreams.StoreDelta) {
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

func (b *Builder) Flush() {
	if tracer.Enabled() {
		zlog.Debug("flushing store", zap.String("name", b.Name), zap.Int("delta_count", len(b.Deltas)), zap.Int("entry_count", len(b.KV)))
	}
	b.Deltas = nil
	b.lastOrdinal = 0
}

func (b *Builder) Roll() {
	b.BlockRange.ExclusiveEndBlock = b.BlockRange.ExclusiveEndBlock + b.SaveInterval
}
func (b *Builder) RollPartial() {
	b.KV = map[string][]byte{}
	b.BlockRange.StartBlock = b.BlockRange.ExclusiveEndBlock
	b.BlockRange.ExclusiveEndBlock = b.BlockRange.ExclusiveEndBlock + b.SaveInterval
}

func (b *Builder) bumpOrdinal(ord uint64) {
	if b.lastOrdinal > ord {
		panic("cannot Set or Del a value on a state.Builder with an ordinal lower than the previous")
	}
	b.lastOrdinal = ord
}

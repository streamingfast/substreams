package state

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/streamingfast/dstore"
	"github.com/streamingfast/substreams/block"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/pipeline/outputs"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"google.golang.org/protobuf/proto"
)

type Info struct {
	LastKVFile        string `json:"last_kv_file"`
	LastKVSavedBlock  uint64 `json:"last_saved_block"`
	RangeIntervalSize uint64 `json:"range_interval_size"`
}

type Builder struct {
	Name         string
	Store        dstore.Store
	saveInterval uint64

	ModuleStartBlock uint64

	BlockRange block.Range

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
	enc.AddObject("block_range", &b.BlockRange)
	enc.AddBool("partial", b.partialMode)

	return nil
}

type BuilderOption func(b *Builder)

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

func (b *Builder) writeStateInfo(ctx context.Context, info *Info) error {
	b.infoLock.Lock()
	defer b.infoLock.Unlock()

	data, err := json.Marshal(info)
	if err != nil {
		return fmt.Errorf("marshaling state info: %w", err)
	}

	err = b.Store.WriteObject(ctx, StateInfoFileName(), bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("writing file %s: %w", StateInfoFileName(), err)
	}

	b.info = info

	return nil
}

func readStateInfo(ctx context.Context, store dstore.Store) (*Info, error) {
	rc, err := store.OpenObject(ctx, StateInfoFileName())
	if err != nil {
		if err == dstore.ErrNotFound {
			return &Info{}, nil
		}
		return nil, fmt.Errorf("opening object %s: %w", StateInfoFileName(), err)
	}

	defer func(rc io.ReadCloser) {
		err := rc.Close()
		if err != nil {
			zlog.Error("closing object", zap.String("object_name", StateInfoFileName()), zap.Error(err))
		}
	}(rc)

	data, err := io.ReadAll(rc)
	if err != nil {
		return nil, fmt.Errorf("reading data for %s: %w", StateInfoFileName(), err)
	}

	var info *Info
	err = json.Unmarshal(data, &info)
	if err != nil {
		return nil, fmt.Errorf("unmarshaling state info data: %w", err)
	}

	return info, nil
}

func (b *Builder) Info(ctx context.Context) (*Info, error) {
	if b.info == nil {
		b.infoLock.Lock()
		defer b.infoLock.Unlock()

		if info, err := readStateInfo(ctx, b.Store); err != nil {
			return nil, fmt.Errorf("reading state info for builder %q: %w", b.Name, err)
		} else {
			return info, nil
		}

	}

	return b.info, nil
}

func (b *Builder) InitializePartial(ctx context.Context, startBlock uint64) error {
	b.partialMode = true
	b.BlockRange = block.Range{
		StartBlock:        startBlock,
		ExclusiveEndBlock: startBlock + b.saveInterval,
	}

	fileName := PartialFileName(b.BlockRange)
	return b.loadState(ctx, fileName)
}

func (b *Builder) Initialize(ctx context.Context, requestedStartBlock uint64, outputCacheSaveInterval uint64, outputCacheStore dstore.Store) error {
	b.BlockRange.StartBlock = b.ModuleStartBlock

	zlog.Debug("initializing builder", zap.String("module_name", b.Name), zap.Uint64("requested_start_block", requestedStartBlock))
	if requestedStartBlock == b.BlockRange.StartBlock {
		b.BlockRange.StartBlock = requestedStartBlock
		floor := requestedStartBlock - requestedStartBlock%b.saveInterval
		b.BlockRange.ExclusiveEndBlock = floor + b.saveInterval
		b.KV = map[string][]byte{}
		return nil
	}

	startBlockNum := requestedStartBlock - requestedStartBlock%b.saveInterval
	deltasStartBlock := uint64(0)

	zlog.Debug("computed info", zap.String("module_name", b.Name), zap.Uint64("start_block", startBlockNum))

	deltasNeeded := false
	if startBlockNum >= b.saveInterval && startBlockNum > b.BlockRange.StartBlock {
		deltasStartBlock = requestedStartBlock - startBlockNum
		deltasNeeded = deltasStartBlock > 0

		atBlock := startBlockNum - b.saveInterval // get the previous saved range
		b.BlockRange.ExclusiveEndBlock = startBlockNum
		fileName := StateFileName(block.Range{
			StartBlock:        b.ModuleStartBlock,
			ExclusiveEndBlock: b.BlockRange.ExclusiveEndBlock,
		})

		zlog.Info("about to load state", zap.String("module_name", b.Name), zap.Uint64("at_block", atBlock), zap.Uint64("deltas_start_block", deltasStartBlock))
		err := b.loadState(ctx, fileName)
		if err != nil {
			return fmt.Errorf("reading state file for module %q: %w", b.Name, err)
		}
	} else {
		deltasNeeded = true
		deltasStartBlock = b.BlockRange.StartBlock
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

	r, err := b.Store.OpenObject(ctx, stateFileName)
	if err != nil {
		return fmt.Errorf("opening file state file %s: %w", stateFileName, err)
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

	zlog.Debug("state loaded", zap.String("builder_name", b.Name), zap.String("file_name", stateFileName))
	return nil
}

func (b *Builder) loadDelta(ctx context.Context, fromBlock, exclusiveStopBlock uint64, outputCacheSaveInterval uint64, outputCacheStore dstore.Store) error {
	if b.partialMode {
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
		deltas := outputCache.SortedCacheItem()
		if len(deltas) == 0 {
			panic("missing deltas")
		}
		firstSeenBlockNum := uint64(0)
		lastSeenBlockNum := uint64(0)
		for _, delta := range deltas {
			//todo: we should check the from block?
			if delta.BlockNum >= exclusiveStopBlock {
				return nil //all good we reach the end
			}
			if firstSeenBlockNum == uint64(0) {
				firstSeenBlockNum = delta.BlockNum
			}
			lastSeenBlockNum = delta.BlockNum
			pbDelta := &pbsubstreams.StoreDelta{}
			err := proto.Unmarshal(delta.Payload, pbDelta)
			if err != nil {
				return fmt.Errorf("unmarshalling builder %q delta at block %d: %w", b.Name, delta.BlockNum, err)
			}
			b.Deltas = append(b.Deltas, pbDelta)
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

func (b *Builder) WriteState(ctx context.Context, blockNum uint64) (filename string, err error) {
	zlog.Debug("writing state", zap.String("module", b.Name))
	b.writeMergeValues()

	kv := stringMap(b.KV) // FOR READABILITY ON DISK

	content, err := json.MarshalIndent(kv, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal kv state: %w", err)
	}

	zlog.Info("write state mode",
		zap.String("store", b.Name),
		zap.Bool("partial", b.partialMode),
		zap.Object("block_range", &b.BlockRange),
	)

	if b.partialMode {
		filename, err = b.writePartialState(ctx, content)
	} else {
		filename, err = b.writeState(ctx, content)
	}
	if err != nil {
		return "", fmt.Errorf("writing %s kv at block %d: %w", b.Name, blockNum, err)
	}

	return filename, nil
}

func (b *Builder) writeState(ctx context.Context, content []byte) (string, error) {
	filename := StateFileName(b.BlockRange)
	err := b.Store.WriteObject(ctx, filename, bytes.NewReader(content))
	if err != nil {
		return filename, fmt.Errorf("writing state %s for range %s: %w", b.Name, b.BlockRange.String(), err)
	}

	var info = &Info{
		LastKVFile:        filename,
		LastKVSavedBlock:  b.BlockRange.ExclusiveEndBlock,
		RangeIntervalSize: b.saveInterval,
	}
	err = b.writeStateInfo(ctx, info)
	if err != nil {
		return "", fmt.Errorf("writing state info for builder %q: %w", b.Name, err)
	}
	b.info = info
	zlog.Debug("state file written", zap.String("module_name", b.Name), zap.Object("block_range", &b.BlockRange), zap.String("file_name", filename))

	return filename, err
}

func (b *Builder) writePartialState(ctx context.Context, content []byte) (string, error) {
	filename := PartialFileName(b.BlockRange)
	return filename, b.Store.WriteObject(ctx, filename, bytes.NewReader(content))
}

var NotFound = errors.New("state key not found")

func (b *Builder) GetFirst(key string) ([]byte, bool) {
	for _, delta := range b.Deltas {
		if delta.Key == key {
			switch delta.Operation {
			case pbsubstreams.StoreDelta_DELETE, pbsubstreams.StoreDelta_UPDATE:
				return delta.OldValue, true
			case pbsubstreams.StoreDelta_CREATE:
				return nil, false
			default:
				// WARN: is that legit? what if some upstream stream is broken? can we trust all those streams?
				panic(fmt.Sprintf("invalid value %q for pbsubstreams.StoreDelta::Op for key %q", delta.Operation.String(), delta.Key))
			}
		}
	}
	return b.GetLast(key)
}

func (b *Builder) GetLast(key string) ([]byte, bool) {
	val, found := b.KV[key]
	return val, found
}

// GetAt returns the key for the state that includes the processing of `ord`.
func (b *Builder) GetAt(ord uint64, key string) (out []byte, found bool) {
	out, found = b.GetLast(key)

	for i := len(b.Deltas) - 1; i >= 0; i-- {
		delta := b.Deltas[i]
		if delta.Ordinal <= ord {
			break
		}
		if delta.Key == key {
			switch delta.Operation {
			case pbsubstreams.StoreDelta_DELETE, pbsubstreams.StoreDelta_UPDATE:
				out = delta.OldValue
				found = true
			case pbsubstreams.StoreDelta_CREATE:
				out = nil
				found = false
			default:
				// WARN: is that legit? what if some upstream stream is broken? can we trust all those streams?
				panic(fmt.Sprintf("invalid value %q for pbsubstreams.StateDelta::Op for key %q", delta.Operation, delta.Key))
			}
		}
	}
	return
}

func (b *Builder) Del(ord uint64, key string) {
	b.bumpOrdinal(ord)

	val, found := b.GetLast(key)
	if found {
		delta := &pbsubstreams.StoreDelta{
			Operation: pbsubstreams.StoreDelta_DELETE,
			Ordinal:   ord,
			Key:       key,
			OldValue:  val,
			NewValue:  nil,
		}
		b.ApplyDelta(delta)
		b.Deltas = append(b.Deltas, delta)
	}
}

func (b *Builder) DeletePrefix(ord uint64, prefix string) {
	b.bumpOrdinal(ord)

	for key, val := range b.KV {
		if !strings.HasPrefix(key, prefix) {
			continue
		}
		delta := &pbsubstreams.StoreDelta{
			Operation: pbsubstreams.StoreDelta_DELETE,
			Ordinal:   ord,
			Key:       key,
			OldValue:  val,
			NewValue:  nil,
		}
		b.ApplyDelta(delta)
		b.Deltas = append(b.Deltas, delta)

	}

	if b.partialMode {
		b.DeletedPrefixes = append(b.DeletedPrefixes, prefix)
	}
}

func (b *Builder) bumpOrdinal(ord uint64) {
	if b.lastOrdinal > ord {
		panic("cannot Set or Del a value on a state.Builder with an ordinal lower than the previous")
	}
	b.lastOrdinal = ord
}

func (b *Builder) SetBytesIfNotExists(ord uint64, key string, value []byte) {
	b.setIfNotExists(ord, key, value)
}

func (b *Builder) SetIfNotExists(ord uint64, key string, value string) {
	b.setIfNotExists(ord, key, []byte(value))
}

func (b *Builder) SetBytes(ord uint64, key string, value []byte) {
	b.set(ord, key, value)
}
func (b *Builder) Set(ord uint64, key string, value string) {
	b.set(ord, key, []byte(value))
}

func (b *Builder) set(ord uint64, key string, value []byte) {
	b.bumpOrdinal(ord)

	val, found := b.GetLast(key)

	var delta *pbsubstreams.StoreDelta
	if found {
		//Uncomment when finished debugging:
		if bytes.Compare(value, val) == 0 {
			return
		}
		delta = &pbsubstreams.StoreDelta{
			Operation: pbsubstreams.StoreDelta_UPDATE,
			Ordinal:   ord,
			Key:       key,
			OldValue:  val,
			NewValue:  value,
		}
	} else {
		delta = &pbsubstreams.StoreDelta{
			Operation: pbsubstreams.StoreDelta_CREATE,
			Ordinal:   ord,
			Key:       key,
			OldValue:  nil,
			NewValue:  value,
		}
	}
	b.ApplyDelta(delta)
	b.Deltas = append(b.Deltas, delta)
}

func (b *Builder) setIfNotExists(ord uint64, key string, value []byte) {
	b.bumpOrdinal(ord)

	_, found := b.GetLast(key)
	if found {
		return
	}

	delta := &pbsubstreams.StoreDelta{
		Operation: pbsubstreams.StoreDelta_CREATE,
		Ordinal:   ord,
		Key:       key,
		OldValue:  nil,
		NewValue:  value,
	}
	b.ApplyDelta(delta)
	b.Deltas = append(b.Deltas, delta)
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

func stringMap(in map[string][]byte) map[string]string {
	out := map[string]string{}
	for k, v := range in {
		out[k] = string(v)
	}
	return out
}

func byteMap(in map[string]string) map[string][]byte {
	out := map[string][]byte{}
	for k, v := range in {
		out[k] = []byte(v)
	}
	return out
}

func StateFilePrefix(blockNum uint64) string {
	return fmt.Sprintf("%010d", blockNum)
}

func PartialFileName(r block.Range) string {
	return fmt.Sprintf("%010d-%010d.partial", r.ExclusiveEndBlock, r.StartBlock)
}

func StateFileName(r block.Range) string {
	return fmt.Sprintf("%010d-%010d.kv", r.ExclusiveEndBlock, r.StartBlock)
}

func StateInfoFileName() string {
	return "___store-metadata.json"
}

func FilePrefix(endBlockNum uint64) string {
	return fmt.Sprintf("%010d-", endBlockNum)
}

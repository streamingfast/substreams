package state

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/streamingfast/dstore"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"go.uber.org/zap"
)

type Builder struct {
	Name              string
	Store             *Store
	partialMode       bool
	partialStartBlock uint64
	ModuleStartBlock  uint64
	ModuleHash        string

	complete bool

	KV              map[string][]byte          // KV is the state, and assumes all Deltas were already applied to it.
	Deltas          []*pbsubstreams.StoreDelta // Deltas are always deltas for the given block.
	DeletedPrefixes []string

	updatePolicy pbsubstreams.Module_KindStore_UpdatePolicy
	valueType    string
	lastOrdinal  uint64
}

type BuilderOption func(b *Builder)

func WithPartialMode(startBlock uint64) BuilderOption {
	return func(b *Builder) {
		b.partialMode = true
		b.partialStartBlock = startBlock
	}
}

func NewBuilder(name string, moduleStartBlock uint64, updatePolicy pbsubstreams.Module_KindStore_UpdatePolicy, valueType string, store *Store, opts ...BuilderOption) *Builder {
	b := &Builder{
		Name:             name,
		ModuleStartBlock: moduleStartBlock,
		KV:               make(map[string][]byte),
		updatePolicy:     updatePolicy,
		valueType:        valueType,
		Store:            store,
	}

	for _, opt := range opts {
		opt(b)
	}

	return b
}

func BuilderFromFile(ctx context.Context, filename string, store dstore.Store) (*Builder, error) {
	fileinfo, ok := ParseFileName(filename)
	if !ok {
		return nil, fmt.Errorf("could not parse filename %s", filename)
	}

	rc, err := store.OpenObject(ctx, filename)
	if err != nil {
		return nil, fmt.Errorf("opening file %s: %w", filename, err)
	}

	data, err := io.ReadAll(rc)
	if err != nil {
		return nil, fmt.Errorf("reading data: %w", err)
	}
	defer rc.Close()

	kv := map[string]string{}
	if err = json.Unmarshal(data, &kv); err != nil {
		return nil, fmt.Errorf("json unmarshal of data: %w", err)
	}

	updatedkv, updatePolicy, valueType, moduleHash, moduleStartBlock, name := readMergeValues(byteMap(kv))

	stateStore, err := newStoreWithStore(name, moduleHash, moduleStartBlock, store)
	if err != nil {
		return nil, fmt.Errorf("creating store: %w", err)
	}

	b := &Builder{
		KV:               updatedkv,
		Store:            stateStore,
		Name:             name,
		ModuleHash:       moduleHash,
		ModuleStartBlock: moduleStartBlock,
		updatePolicy:     updatePolicy,
		valueType:        valueType,
		partialMode:      fileinfo.Partial,
	}

	if fileinfo.Partial {
		b.partialStartBlock = fileinfo.StartBlock
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

func (b *Builder) Squash(ctx context.Context, baseStore dstore.Store, upToBlock uint64) error {
	files, err := pathToState(ctx, b.Store, upToBlock, b.ModuleStartBlock)
	if err != nil {
		return err
	}
	zlog.Info("squashing files found", zap.Strings("files", files), zap.String("store", b.Name))

	var builders []*Builder
	for _, file := range files {
		builder, err := BuilderFromFile(ctx, file, baseStore)
		if err != nil {
			return fmt.Errorf("creating builder from file %s: %w", file, err)
		}

		builders = append(builders, builder)
	}

	if len(builders) == 0 {
		return fmt.Errorf("len of builders is 0")
	}

	builders = append(builders, b)
	zlog.Info("number of builders", zap.Int("len", len(builders)), zap.String("store", b.Name))

	for i := 0; i < len(builders)-1; i++ {
		prev := builders[i]
		next := builders[i+1]
		err := next.Merge(prev)
		if err != nil {
			return fmt.Errorf("merging state for %s: %w", b.Name, err)
		}
	}

	return nil
}

func (b *Builder) ReadState(ctx context.Context, requestedStartBlock uint64) error {
	files, err := pathToState(ctx, b.Store, requestedStartBlock, b.ModuleStartBlock)
	if err != nil {
		return err
	}
	zlog.Info("read state files found", zap.Strings("files", files), zap.String("store", b.Name))

	var builders []*Builder
	for _, file := range files {
		builder, err := BuilderFromFile(ctx, file, b.Store.Store)
		if err != nil {
			return fmt.Errorf("creating builder from file %s: %w", file, err)
		}

		builders = append(builders, builder)
	}

	zlog.Info("number of builders", zap.Int("len", len(builders)), zap.String("store", b.Name))
	switch len(builders) {
	case 0:
		/// nothing to do
	case 1:
		b.KV = builders[0].KV
	default:
		// merge all builders, sequentially from the start.
		for i := 0; i < len(builders)-1; i++ {
			prev := builders[i]
			next := builders[i+1]
			err := next.Merge(prev)
			if err != nil {
				return fmt.Errorf("merging state for %s: %w", b.Name, err)
			}
		}
		b.KV = builders[len(builders)-1].KV

		zlog.Info("writing state", zap.String("store", b.Name))
		f, err := b.WriteState(ctx, requestedStartBlock, false)
		if err != nil {
			return fmt.Errorf("writing merged kv: %w", err)
		}
		zlog.Info("state written", zap.String("filename", f), zap.String("store", b.Name))
	}

	return nil
}

func (b *Builder) WriteState(ctx context.Context, blockNum uint64, partialMode bool) (filename string, err error) {
	b.writeMergeValues()

	kv := stringMap(b.KV) // FOR READABILITY ON DISK

	content, err := json.MarshalIndent(kv, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal kv state: %w", err)
	}

	if partialMode && b.partialStartBlock <= b.ModuleStartBlock {
		partialMode = false //this starts at module start block, therefore consider it a full kv
	}

	zlog.Info("write state mode",
		zap.Bool("partial", partialMode),
		zap.String("store", b.Name),
		zap.Uint64("partial_start_block", b.partialStartBlock),
		zap.Uint64("module_start_block", b.ModuleStartBlock),
	)

	if partialMode {
		filename, err = b.Store.WritePartialState(ctx, content, b.partialStartBlock, blockNum)
	} else {
		filename, err = b.Store.WriteState(ctx, content, blockNum)
	}
	if err != nil {
		return "", fmt.Errorf("writing %s kv at block %d: %w", b.Name, blockNum, err)
	}

	return filename, nil
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
		b.applyDelta(delta)
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
		b.applyDelta(delta)
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
	b.applyDelta(delta)
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
	b.applyDelta(delta)
	b.Deltas = append(b.Deltas, delta)
}

func (b *Builder) applyDelta(delta *pbsubstreams.StoreDelta) {
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

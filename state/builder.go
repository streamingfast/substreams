package state

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	pbtransform "github.com/streamingfast/substreams/pb/sf/substreams/transform/v1"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

type Builder struct {
	Name string

	store StoreInterface

	partialMode       bool
	partialStartBlock uint64
	moduleStartBlock  uint64
	disableWriteState bool

	complete bool

	KV          map[string][]byte          // KV is the state, and assumes all Deltas were already applied to it.
	Deltas      []*pbsubstreams.StoreDelta // Deltas are always deltas for the given block.
	DeletedKeys map[string]interface{}

	updatePolicy pbtransform.KindStore_UpdatePolicy
	valueType    string
	lastOrdinal  uint64
}

type BuilderOption func(b *Builder)

func WithPartialMode(partialMode bool, startBlock, moduleStartBlock uint64, outputStream string) BuilderOption {
	return func(b *Builder) {
		b.partialMode = partialMode
		b.partialStartBlock = startBlock
		b.moduleStartBlock = moduleStartBlock
		b.disableWriteState = outputStream != b.Name
	}
}

func NewBuilder(name string, updatePolicy pbtransform.KindStore_UpdatePolicy, valueType string, storageFactory FactoryInterface, opts ...BuilderOption) *Builder {
	b := &Builder{
		Name:         name,
		KV:           make(map[string][]byte),
		updatePolicy: updatePolicy,
		valueType:    valueType,
	}
	if storageFactory != nil {
		b.store = storageFactory.New(name)
	}

	for _, opt := range opts {
		opt(b)
	}

	return b
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

func (b *Builder) Init(ctx context.Context, startBlockNum uint64) error {
	if err := b.ReadState(ctx, startBlockNum); err != nil {
		return err
	}

	return nil
}

func (b *Builder) clone() *Builder {
	o := &Builder{
		Name:         b.Name,
		KV:           make(map[string][]byte),
		updatePolicy: b.updatePolicy,
		valueType:    b.valueType,
	}
	return o
}

func (b *Builder) ReadState(ctx context.Context, blockNumber uint64) error {
	_, files, err := ContiguousFilesToTargetBlock(ctx, b.Name, b.store, b.moduleStartBlock, blockNumber)
	if err != nil {
		return err
	}

	var builders []*Builder
	for _, file := range files {
		data, err := func() ([]byte, error) { //this is an inline func so that we can defer the close call properly
			rc, err := b.store.OpenObject(ctx, file)
			if err != nil {
				return nil, err
			}
			defer rc.Close()

			data, err := io.ReadAll(rc)
			if err != nil {
				return nil, err
			}

			return data, nil
		}()

		if err != nil {
			return fmt.Errorf("reading file %s in store %s: %w", file, b.Name, err)
		}

		builder := b.clone()
		kv := map[string]string{}
		if err = json.Unmarshal(data, &kv); err != nil {
			return fmt.Errorf("unmarshalling kv file %s for %s at block %d: %w", file, b.Name, blockNumber, err)
		}

		builder.KV = byteMap(kv)
		builders = append(builders, builder)
	}

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

		err := b.writeState(ctx, blockNumber, false)
		if err != nil {
			return fmt.Errorf("writing merged kv: %w", err)
		}
	}

	return nil
}

func (b *Builder) WriteState(ctx context.Context, blockNum uint64) error {
	if b.disableWriteState {
		return nil
	}

	return b.writeState(ctx, blockNum, b.partialMode)
}

func (b *Builder) writeState(ctx context.Context, blockNum uint64, partialMode bool) error {
	kv := stringMap(b.KV) // FOR READABILITY ON DISK

	content, err := json.MarshalIndent(kv, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal kv state: %w", err)
	}

	if partialMode && b.partialStartBlock == b.moduleStartBlock {
		partialMode = false //this starts at module start block, therefore consider it a full kv
	}

	var writeFunc func() error
	if partialMode {
		writeFunc = func() error {
			return b.store.WritePartialState(ctx, content, b.partialStartBlock, blockNum)
		}
	} else {
		writeFunc = func() error {
			return b.store.WriteState(ctx, content, blockNum)
		}
	}

	if err = writeFunc(); err != nil {
		return fmt.Errorf("writing %s kv at block %d: %w", b.Name, blockNum, err)
	}

	return nil
}

var NotFound = errors.New("state key not found")

func (b *Builder) GetFirst(key string) ([]byte, bool, error) {
	for _, delta := range b.Deltas {
		if delta.Key == key {
			switch delta.Operation {
			case pbsubstreams.StoreDelta_DELETE, pbsubstreams.StoreDelta_UPDATE:
				return delta.OldValue, true, nil
			case pbsubstreams.StoreDelta_CREATE:
				return nil, false, nil
			default:
				return nil, false, fmt.Errorf("get first: invalid operation %q for key %q of builder %s", delta.Operation.String(), delta.Key, b.Name)
			}
		}
	}
	data, found := b.GetLast(key)
	return data, found, nil
}

func (b *Builder) GetLast(key string) ([]byte, bool) {
	val, found := b.KV[key]
	return val, found
}

// GetAt returns the key for the state that includes the processing of `ord`.
func (b *Builder) GetAt(ord uint64, key string) (out []byte, found bool, err error) {
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
				return nil, false, fmt.Errorf("get at: invalid operation %q for key %q of builder %s", delta.Operation.String(), delta.Key, b.Name)

			}
		}
	}
	return
}

func (b *Builder) Del(ord uint64, key string) error {
	err := b.bumpOrdinal(ord)
	if err != nil {
		return fmt.Errorf("builder delete: %w", err)
	}

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
	return nil
}

func (b *Builder) DeletePrefix(ord uint64, prefix string) error {
	err := b.bumpOrdinal(ord)
	if err != nil {
		return fmt.Errorf("builder delete prefix: %w", err)
	}

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

		//todo: if builder in batch mode. we need to add the deleted key to the b.DeletedKeys
	}
	return nil
}

func (b *Builder) bumpOrdinal(ordinal uint64) error {
	if b.lastOrdinal > ordinal {
		return fmt.Errorf("bump ordinal: ordinal %d is lower then last ordinal %d", ordinal, b.lastOrdinal)
	}
	b.lastOrdinal = ordinal
	return nil
}

func (b *Builder) SetBytesIfNotExists(ord uint64, key string, value []byte) error {
	return b.setIfNotExists(ord, key, value)
}

func (b *Builder) SetIfNotExists(ord uint64, key string, value string) error {
	return b.setIfNotExists(ord, key, []byte(value))
}

func (b *Builder) SetBytes(ord uint64, key string, value []byte) error {
	return b.set(ord, key, value)
}
func (b *Builder) Set(ord uint64, key string, value string) error {
	return b.set(ord, key, []byte(value))
}

func (b *Builder) set(ord uint64, key string, value []byte) error {
	err := b.bumpOrdinal(ord)
	if err != nil {
		return fmt.Errorf("builder set: %w", err)
	}

	val, found := b.GetLast(key)

	var delta *pbsubstreams.StoreDelta
	if found {
		//Uncomment when finished debugging:
		if bytes.Compare(value, val) == 0 {
			return nil
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
	return nil
}

func (b *Builder) setIfNotExists(ord uint64, key string, value []byte) error {
	err := b.bumpOrdinal(ord)
	if err != nil {
		return fmt.Errorf("builder set if not exist: %w", err)
	}

	_, found := b.GetLast(key)
	if found {
		return nil
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
	return nil
}

func (b *Builder) applyDelta(delta *pbsubstreams.StoreDelta) {
	switch delta.Operation {
	case pbsubstreams.StoreDelta_UPDATE, pbsubstreams.StoreDelta_CREATE:
		b.KV[delta.Key] = delta.NewValue
	case pbsubstreams.StoreDelta_DELETE:
		delete(b.KV, delta.Key)
	}
}

func (b *Builder) Flush() {
	for _, delta := range b.Deltas {
		b.applyDelta(delta)
	}
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

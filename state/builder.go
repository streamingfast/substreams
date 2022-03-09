package state

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"

	"github.com/streamingfast/bstream"
	"github.com/streamingfast/merger/bundle"
)

type Builder struct {
	Name string

	bundler *bundle.Bundler
	io      StateIO

	KV           map[string][]byte          // KV is the state, and assumes all Deltas were already applied to it.
	Deltas       []*pbsubstreams.StoreDelta // Deltas are always deltas for the given block.
	DeletedKeys  map[string]interface{}
	updatePolicy string
	valueType    string
	protoType    string
	lastOrdinal  uint64
}

func NewBuilder(name string, updatePolicy, valueType, protoType string, ioFactory IOFactory) *Builder {
	b := &Builder{
		Name:         name,
		KV:           make(map[string][]byte),
		bundler:      nil,
		updatePolicy: updatePolicy,
		valueType:    valueType,
		protoType:    protoType,
	}
	if ioFactory != nil {
		b.io = ioFactory.New(name)
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

func (b *Builder) Init(startBlockNum uint64) error {
	relativeKvStartBlock := (startBlockNum / 100) * 100

	if err := b.ReadState(context.TODO(), relativeKvStartBlock); err != nil {
		return err
	}

	//var deltas []*bundle.OneBlockFile
	//
	//// walk from last kv checkpoint to current start block
	//err := b.io.WalkDeltas(context.TODO(), relativeKvStartBlock+1, startBlockNum-1, func(obf *bundle.OneBlockFile) error {
	//	deltas = append(deltas, obf)
	//	return nil
	//})
	//if err != nil {
	//	return err
	//}
	//
	//sort.Slice(deltas, func(i, j int) bool {
	//	return deltas[i].Num < deltas[j].Num
	//})
	//
	//for _, delta := range deltas {
	//	data, err := b.io.ReadDelta(context.TODO(), delta)
	//	if err != nil {
	//		return err
	//	}
	//	err = json.Unmarshal(data, &b.Deltas)
	//	if err != nil {
	//		return fmt.Errorf("unmarshalling delta for %s at block %d: %w", b.Name, relativeKvStartBlock, err)
	//	}
	//	b.Flush()
	//}

	return nil
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

		//todo: if builder in batch mode. we need to add the deleted key to the b.DeletedKeys
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

func (b *Builder) StoreBlock(ctx context.Context, block *bstream.Block) error {
	//TODO: unused func

	//blockNumber := block.Number
	//
	//if b.bundler == nil {
	//	exclusiveHighestBlockLimit := ((blockNumber / 100) * 100) + 100
	//	b.bundler = bundle.NewBundler(100, exclusiveHighestBlockLimit)
	//}
	//
	//bundleCompleted, highestBlockLimit := b.bundler.BundleCompleted()
	//if bundleCompleted {
	//	_ = b.bundler.ToBundle(highestBlockLimit)
	//	b.bundler.Commit(highestBlockLimit)
	//	b.bundler.Purge(func(oneBlockFilesToDelete []*bundle.OneBlockFile) {
	//		return
	//	})
	//
	//}
	//
	//obf := mustBlockToOneBlockFile(b.Name, block)
	//b.bundler.AddOneBlockFile(obf)
	//
	//if err := b.WriteState(ctx, block); err != nil {
	//	return err
	//}
	//
	//return nil
	return nil
}

func (b *Builder) ReadState(ctx context.Context, startBlockNum uint64) error {
	data, err := b.io.ReadState(ctx, startBlockNum)
	if err != nil {
		return err
	}

	kv := map[string]string{}

	if err = json.Unmarshal(data, &kv); err != nil {
		return fmt.Errorf("unmarshalling kv for %s at block %d: %w", b.Name, startBlockNum, err)
	}

	b.KV = byteMap(kv) // FOR READABILITY ON DISK, perhaps should depend on data type.

	fmt.Printf("loading KV from disk for %q: %d entries\n", b.Name, len(b.KV))

	return nil
}

func (b *Builder) WriteState(ctx context.Context, block *bstream.Block) error {
	kv := stringMap(b.KV) // FOR READABILITY ON DISK

	content, err := json.MarshalIndent(kv, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal kv state: %w", err)
	}

	if err = b.io.WriteState(ctx, content, block); err != nil {
		return fmt.Errorf("writing %s kv at block %d: %w", b.Name, block.Num(), err)
	}

	return nil
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

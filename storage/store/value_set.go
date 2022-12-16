package store

import (
	"fmt"
	"strings"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

func (b *baseStore) SetBytesIfNotExists(ord uint64, key string, value []byte) {
	b.setIfNotExists(ord, key, value)
}

func (b *baseStore) SetIfNotExists(ord uint64, key string, value string) {
	b.setIfNotExists(ord, key, []byte(value))
}

func (b *baseStore) SetBytes(ord uint64, key string, value []byte) {
	b.set(ord, key, value)
}

func (b *baseStore) Set(ord uint64, key string, value string) {
	b.set(ord, key, []byte(value))
}

func (b *baseStore) set(ord uint64, key string, value []byte) {
	// FIXME(abourget): these should return an error up the stack instead, would bubble up
	// in the wasm/module.go and fail the query, with proper error propagation.
	if strings.HasPrefix(key, "__!__") {
		panic("key prefix __!__ is reserved for internal system use.")
	}
	if uint64(len(value)) > b.itemSizeLimit {
		panic(fmt.Sprintf("key %q attempted to write %d bytes (capped at %d)", key, len(value), b.itemSizeLimit))
	}

	if len(key) == 0 {
		panic(fmt.Sprintf("invalid key"))
	}

	b.bumpOrdinal(ord)

	val, found := b.GetLast(key)
	cpValue := make([]byte, len(value))
	copy(cpValue, value)

	var delta *pbsubstreams.StoreDelta
	if found {
		delta = &pbsubstreams.StoreDelta{
			Operation: pbsubstreams.StoreDelta_UPDATE,
			Ordinal:   ord,
			Key:       key,
			OldValue:  val,
			NewValue:  cpValue,
		}
	} else {
		delta = &pbsubstreams.StoreDelta{
			Operation: pbsubstreams.StoreDelta_CREATE,
			Ordinal:   ord,
			Key:       key,
			OldValue:  nil,
			NewValue:  cpValue,
		}
	}

	b.ApplyDelta(delta)
	b.deltas = append(b.deltas, delta)
}

func (b *baseStore) setIfNotExists(ord uint64, key string, value []byte) {
	_, found := b.GetLast(key)
	if found {
		return
	}

	b.bumpOrdinal(ord)

	cpValue := make([]byte, len(value))
	copy(cpValue, value)

	delta := &pbsubstreams.StoreDelta{
		Operation: pbsubstreams.StoreDelta_CREATE,
		Ordinal:   ord,
		Key:       key,
		OldValue:  nil,
		NewValue:  cpValue,
	}

	b.ApplyDelta(delta)
	b.deltas = append(b.deltas, delta)
}

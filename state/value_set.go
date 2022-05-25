package state

import (
	"bytes"
	"strings"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

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
	if strings.HasPrefix(key, "__!__") {
		panic("key prefix __!__ is reserved for internal system use.")
	}
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
	if delta.Key == "" {
		panic("Grrr4")
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
	if delta.Key == "" {
		panic("Grrr4")
	}
	b.ApplyDelta(delta)
	b.Deltas = append(b.Deltas, delta)
}

package store

import (
	"fmt"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

func (b *baseStore) ApplyDelta(delta *pbsubstreams.StoreDelta) {
	// Keys need to have at least one character, and mustn't start with 0xFF is reserved for internal use.
	if len(delta.Key) == 0 {
		panic(fmt.Sprintf("key invalid, must be at least 1 character for module %q", b.name))
	}
	if delta.Key[0] == byte(255) {
		panic(fmt.Sprintf("key %q invalid, must be at least 1 character and not start with 0xFF", delta.Key))
	}

	newSize := uint64(len(delta.NewValue))
	oldSize := uint64(len(delta.OldValue))
	keySize := uint64(len(delta.Key))
	switch delta.Operation {
	case pbsubstreams.StoreDelta_UPDATE:
		b.kv[delta.Key] = delta.NewValue
		switch {
		case newSize > oldSize:
			b.totalSizeBytes += (newSize - oldSize)
		case newSize < oldSize:
			b.totalSizeBytes -= (oldSize - newSize)
		}

	case pbsubstreams.StoreDelta_CREATE:
		b.kv[delta.Key] = delta.NewValue
		b.totalSizeBytes += newSize
		b.totalSizeBytes += keySize

	case pbsubstreams.StoreDelta_DELETE:
		delete(b.kv, delta.Key)
		b.totalSizeBytes -= oldSize
		b.totalSizeBytes -= keySize
		return
	}

	if b.totalSizeBytes > b.totalSizeLimit {
		panic(fmt.Sprintf("store %q became too big at %d, maximum size: %d", b.Name(), b.totalSizeBytes, b.totalSizeLimit))
	}
}

func (b *baseStore) ApplyDeltasReverse(deltas []*pbsubstreams.StoreDelta) {
	for i := len(deltas) - 1; i >= 0; i-- {
		delta := deltas[i]

		newSize := uint64(len(delta.NewValue))
		oldSize := uint64(len(delta.OldValue))
		keySize := uint64(len(delta.Key))
		switch delta.Operation {
		case pbsubstreams.StoreDelta_UPDATE:
			b.kv[delta.Key] = delta.OldValue
			switch {
			case newSize > oldSize:
				b.totalSizeBytes -= (newSize - oldSize)
			case newSize < oldSize:
				b.totalSizeBytes += (oldSize - newSize)
			}

		case pbsubstreams.StoreDelta_CREATE:
			delete(b.kv, delta.Key)
			b.totalSizeBytes -= newSize
			b.totalSizeBytes -= keySize

		case pbsubstreams.StoreDelta_DELETE:
			b.kv[delta.Key] = delta.OldValue
			b.totalSizeBytes += oldSize
			b.totalSizeBytes += keySize
			return
		}
	}
}

func (b *baseStore) GetDeltas() []*pbsubstreams.StoreDelta {
	return b.deltas
}

func (b *baseStore) SetDeltas(deltas []*pbsubstreams.StoreDelta) {
	b.deltas = deltas
	for _, delta := range deltas {
		b.ApplyDelta(delta)
	}
}

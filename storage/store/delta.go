package store

import (
	"errors"
	"fmt"
	"strings"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

// DeletedPrefixes is a specialized map to track deleted prefixes
type DeletedPrefixes map[string]struct{}

// Clear removes all entries in the DeletedPrefixes map
func (dp DeletedPrefixes) Clear() {
	for k := range dp {
		delete(dp, k)
	}
}

// RemoveMatching removes all prefixes of the given string
func (dp DeletedPrefixes) RemoveMatching(key string) {
	for prefix := range dp {
		if strings.HasPrefix(key, prefix) {
			delete(dp, prefix)
		}
	}
}

// Add adds a key to the DeletedPrefixes map
func (dp DeletedPrefixes) Add(prefix string) {
	if len(dp) > 100 {
		dp.Clear() // keep this under reasonable size
	}
	dp[prefix] = struct{}{}
}

func (dp DeletedPrefixes) Exists(prefix string) bool {
	_, ok := dp[prefix]
	return ok
}

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
		b.recentlyDeletedPrefixes.RemoveMatching(delta.Key)

		if err := b.kvImpl.Set(delta.Key, delta.NewValue); err != nil {
			panic(fmt.Sprintf("failed to set key %q: %v", delta.Key, err))
		}
		switch {
		case newSize > oldSize:
			b.totalSizeBytes += (newSize - oldSize)
		case newSize < oldSize:
			b.totalSizeBytes -= (oldSize - newSize)
		}

	case pbsubstreams.StoreDelta_CREATE:
		b.recentlyDeletedPrefixes.RemoveMatching(delta.Key)

		if err := b.kvImpl.Set(delta.Key, delta.NewValue); err != nil {
			panic(fmt.Sprintf("failed to set key %q: %v", delta.Key, err))
		}
		b.totalSizeBytes += newSize
		b.totalSizeBytes += keySize

	case pbsubstreams.StoreDelta_DELETE:
		if err := b.kvImpl.Delete(delta.Key); err != nil {
			panic(fmt.Sprintf("failed to delete key %q: %v", delta.Key, err))
		}
		b.totalSizeBytes -= oldSize
		b.totalSizeBytes -= keySize
		return
	}

	if b.totalSizeBytes > b.totalSizeLimit {
		panic(storeTooBigError(b.Name(), b.totalSizeBytes, b.totalSizeLimit))
	}
}

var ErrStoreAboveMaxSize = errors.New("store above max size")

func storeTooBigError(storeName string, size, limit uint64) error {
	return fmt.Errorf("store %q became too big at %d, maximum size: %d, %w", storeName, size, limit, ErrStoreAboveMaxSize)
}

func (b *baseStore) ApplyDeltasReverse(deltas []*pbsubstreams.StoreDelta) {
	b.recentlyDeletedPrefixes.Clear() // whenever we have an undo block, we clear this cache to avoid any bug

	for i := len(deltas) - 1; i >= 0; i-- {
		delta := deltas[i]

		newSize := uint64(len(delta.NewValue))
		oldSize := uint64(len(delta.OldValue))
		keySize := uint64(len(delta.Key))
		switch delta.Operation {
		case pbsubstreams.StoreDelta_UPDATE:
			if err := b.kvImpl.Set(delta.Key, delta.OldValue); err != nil {
				panic(fmt.Sprintf("failed to set key %q: %v", delta.Key, err))
			}
			switch {
			case newSize > oldSize:
				b.totalSizeBytes -= (newSize - oldSize)
			case newSize < oldSize:
				b.totalSizeBytes += (oldSize - newSize)
			}

		case pbsubstreams.StoreDelta_CREATE:
			if err := b.kvImpl.Delete(delta.Key); err != nil {
				panic(fmt.Sprintf("failed to delete key %q: %v", delta.Key, err))
			}
			b.totalSizeBytes -= newSize
			b.totalSizeBytes -= keySize

		case pbsubstreams.StoreDelta_DELETE:
			if err := b.kvImpl.Set(delta.Key, delta.OldValue); err != nil {
				panic(fmt.Sprintf("failed to set key %q: %v", delta.Key, err))
			}
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

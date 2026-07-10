package store

import (
	"fmt"
	"sort"

	pbssinternal "github.com/streamingfast/substreams/pb/sf/substreams/intern/v2"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

func (b *baseStore) DeletePrefix(ord uint64, prefix string) {
	b.kvOps.Add(&pbssinternal.Operation{
		Type: pbssinternal.Operation_DELETE_PREFIX,
		Ord:  ord,
		Key:  prefix,
	})
}

func (b *baseStore) deletePrefix(ord uint64, prefix string) {
	if b.recentlyDeletedPrefixes.Exists(prefix) {
		return
	}
	b.recentlyDeletedPrefixes.Add(prefix)

	var deltas []*pbsubstreams.StoreDelta
	
	// Scan all keys with the prefix and collect them
	// We must NOT delete during the scan as that would deadlock (scan holds read lock, delete needs write lock)
	err := b.kvImpl.Scan(prefix, func(key string, val []byte) bool {
		// Copy val: on the mmap backend Scan yields a slice that aliases bbolt's
		// page buffer, valid only for the duration of the read transaction. This
		// delta outlives the scan (it is held in `deltas` and consumed later), so
		// keeping the alias lets a subsequent write reuse the page and corrupt
		// OldValue — surfacing downstream as garbage-prefixed values (e.g. a
		// neighbouring key's bytes bleeding into a BigDecimal).
		oldValue := make([]byte, len(val))
		copy(oldValue, val)
		delta := &pbsubstreams.StoreDelta{
			Operation: pbsubstreams.StoreDelta_DELETE,
			Ordinal:   ord,
			Key:       key,
			OldValue:  oldValue,
			NewValue:  nil,
		}
		deltas = append(deltas, delta)
		return true // continue iteration
	})
	
	if err != nil {
		panic(fmt.Sprintf("failed to scan prefix %q: %v", prefix, err))
	}
	
	// Now apply all the deletes after the scan is complete
	for _, delta := range deltas {
		b.ApplyDelta(delta)
	}
	
	sort.Slice(deltas, func(i, j int) bool {
		return deltas[i].Key < deltas[j].Key
	})
	b.deltas = append(b.deltas, deltas...)
}

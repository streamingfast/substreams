package store

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"go.uber.org/zap"
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

	// Enforced on every backend so a module behaves identically on the memory
	// and mmap stores: the mmap (bbolt) backend rejects keys above MaxKeySize,
	// the memory backend has no limit. Without this an oversized key would
	// succeed on memory and panic only on mmap. This is a function of the
	// module's own output, hence a deterministic error.
	//
	// Values need no equivalent guard here: every write is already capped well
	// below bbolt's MaxValueSize (2GiB) by itemSizeLimit (10MiB, value_set.go)
	// and appendLimit (8MiB, value_append.go).
	if len(delta.Key) > maxStoreKeySize {
		b.logger.Warn("store key exceeds backend size limit, failing block deterministically",
			zap.Int("key_bytes", len(delta.Key)),
			zap.Int("limit_bytes", maxStoreKeySize))
		panic(entryTooLargeError(b.name, "key", delta.Key, len(delta.Key), maxStoreKeySize))
	}

	newSize := uint64(len(delta.NewValue))
	oldSize := uint64(len(delta.OldValue))
	keySize := uint64(len(delta.Key))
	switch delta.Operation {
	case pbsubstreams.StoreDelta_UPDATE:
		b.recentlyDeletedPrefixes.RemoveMatching(delta.Key)

		if err := b.kvImpl.Set(delta.Key, delta.NewValue); err != nil {
			panic(backendFailure("set", delta.Key, err))
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
			panic(backendFailure("set", delta.Key, err))
		}
		b.totalSizeBytes += newSize
		b.totalSizeBytes += keySize

	case pbsubstreams.StoreDelta_DELETE:
		if err := b.kvImpl.Delete(delta.Key); err != nil {
			panic(backendFailure("delete", delta.Key, err))
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

// maxStoreKeySize mirrors bbolt's MaxKeySize, the hard key-length cap of the
// mmap backend (not runtime-configurable). Enforced on all backends (see
// ApplyDelta) so behaviour does not diverge between memory and mmap.
const maxStoreKeySize = 32768 // bbolt MaxKeySize

// ErrStoreEntryTooLarge marks a key that exceeds the backend size limit. It is a
// function of the module's output, hence a deterministic error (it is not in the
// non-deterministic ErrStoreBackendFailure family below).
var ErrStoreEntryTooLarge = errors.New("store entry too large")

func entryTooLargeError(storeName, kind, key string, size, limit int) error {
	return fmt.Errorf("store %q %s for key %q is %d bytes, exceeds backend limit of %d: %w", storeName, kind, key, size, limit, ErrStoreEntryTooLarge)
}

// ErrStoreBackendFailure marks a store operation that failed for reasons that
// are NOT a function of the module's input: disk full, mmap grow / ENOMEM, I/O
// error, or writing to a closed store. It only arises on the mmap backend (the
// memory backend's Set/Delete cannot fail). It is non-deterministic — a retry
// on a healthy node can succeed — so callers must NOT cache it as a
// deterministic error (see pipeline/exec.baseexec).
var ErrStoreBackendFailure = errors.New("store backend failure")

func backendFailure(op, key string, err error) error {
	return fmt.Errorf("store backend failed to %s key %q: %w: %w", op, key, err, ErrStoreBackendFailure)
}

func storeTooBigError(storeName string, size, limit uint64) error {
	return fmt.Errorf("store %q became too big at %d, maximum size: %d, %w", storeName, size, limit, ErrStoreAboveMaxSize)
}

func (b *baseStore) ApplyDeltasReverse(deltas []*pbsubstreams.StoreDelta) {
	b.recentlyDeletedPrefixes.Clear() // whenever we have an undo block, we clear this cache to avoid any bug

	for _, delta := range slices.Backward(deltas) {

		newSize := uint64(len(delta.NewValue))
		oldSize := uint64(len(delta.OldValue))
		keySize := uint64(len(delta.Key))
		switch delta.Operation {
		case pbsubstreams.StoreDelta_UPDATE:
			if err := b.kvImpl.Set(delta.Key, delta.OldValue); err != nil {
				panic(backendFailure("set", delta.Key, err))
			}
			switch {
			case newSize > oldSize:
				b.totalSizeBytes -= (newSize - oldSize)
			case newSize < oldSize:
				b.totalSizeBytes += (oldSize - newSize)
			}

		case pbsubstreams.StoreDelta_CREATE:
			if err := b.kvImpl.Delete(delta.Key); err != nil {
				panic(backendFailure("delete", delta.Key, err))
			}
			b.totalSizeBytes -= newSize
			b.totalSizeBytes -= keySize

		case pbsubstreams.StoreDelta_DELETE:
			if err := b.kvImpl.Set(delta.Key, delta.OldValue); err != nil {
				panic(backendFailure("set", delta.Key, err))
			}
			b.totalSizeBytes += oldSize
			b.totalSizeBytes += keySize
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

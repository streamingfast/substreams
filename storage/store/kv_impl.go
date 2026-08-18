package store

import (
	"io"
	"iter"

	"github.com/streamingfast/substreams/storage/store/marshaller"
)

// KVImpl is the storage implementation for key-value pairs in baseStore.
// It abstracts over in-memory (map) and mmap-backed (bbolt) storage.
// bbolt is the default to prevent OOM kills on large stores.
type KVImpl interface {
	// Get retrieves the value for a key. Returns (value, true) if found, (nil, false) otherwise.
	Get(key string) ([]byte, bool)

	// Set writes a key-value pair. The value slice must not be modified after this call.
	Set(key string, value []byte) error

	// Delete removes a key. No error if key doesn't exist.
	Delete(key string) error

	// Scan iterates over all keys with a given prefix in lexicographic order.
	// The callback receives (key, value). Return false from the callback to stop iteration.
	// The value slice is only valid during the callback and must not be retained.
	Scan(prefix string, fn func(key string, value []byte) bool) error

	// Iter iterates over all keys and allows the callback to return an error.
	// If the callback returns an error, iteration stops and that error is returned.
	// The value slice is only valid during the callback and must not be retained.
	Iter(fn func(key string, value []byte) error) error

	// Clear removes all key-value pairs, resetting the store to an empty state.
	// For mmap backends this drops and recreates the bbolt bucket in a single transaction.
	// For memory backends this replaces the map with a fresh allocation.
	Clear() error

	// Load replaces the impl's contents from an iterator of StoreDataEntry.
	// This is called during store deserialization (e.g., loading a FullKV snapshot from object storage).
	Load(it iter.Seq2[marshaller.StoreDataEntry, error]) (*marshaller.StoreDataTrailer, error)

	// Save returns an iterator yielding all key-value pairs in sorted order.
	// This is the inverse of Load, streams data out without materializing a map.
	Save() iter.Seq2[marshaller.StoreDataEntry, error]

	// Snapshot returns a pull-based iterator over a stable view of the store in
	// lexicographic key order, used to stream store serialization without
	// materializing it. For the mmap backend, writes to the store are blocked
	// while a snapshot is open so pages read across internal batches stay
	// consistent. The returned iterator MUST be closed.
	Snapshot() (marshaller.KVSnapshotIter, error)

	// BatchSet writes multiple key-value pairs in a single atomic operation.
	// This is significantly faster than calling Set() in a loop for mmap backends
	// because it commits a single transaction instead of one per key.
	// For memory backends it is equivalent to calling Set() in a loop.
	BatchSet(kv map[string][]byte) error

	// GetMany retrieves multiple keys in a single operation.
	// For mmap backends this uses a single read transaction, avoiding the per-call
	// transaction overhead of calling Get() in a loop.
	// Keys not found are omitted from the result map.
	GetMany(keys []string) (map[string][]byte, error)

	// GetManySizes retrieves, in a single operation, the byte length of the
	// value stored at each of the given keys. Keys not found are omitted from
	// the result map (so the map doubles as a presence test). It is the
	// value-free counterpart of GetMany: for the mmap backend it reads value
	// lengths straight out of the bbolt pages without copying the values onto
	// the heap, which the merge path uses for SET/SET_IF_NOT_EXISTS where only
	// prior existence and size (for accounting) matter, not the old bytes.
	GetManySizes(keys []string) (map[string]int, error)

	// KeyCount returns the number of keys currently stored.
	KeyCount() int

	// Close releases any resources held by the implementation (e.g., file handles, mmap regions).
	io.Closer
}

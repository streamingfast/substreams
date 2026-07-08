package store

import (
	"iter"
	"sort"
	"strings"

	"github.com/streamingfast/substreams/metrics"
	"github.com/streamingfast/substreams/storage/store/marshaller"
)

// memoryKVImpl is an in-memory KVImpl backed by a Go map.
// All data lives in the Go heap. This is the original behavior and is kept for backward compatibility.
type memoryKVImpl struct {
	kv        map[string][]byte
	storeName string // for metrics labels
}

// newMemoryKVImpl creates a new in-memory KVImpl.
func newMemoryKVImpl() *memoryKVImpl {
	return &memoryKVImpl{
		kv:        make(map[string][]byte),
		storeName: "unknown",
	}
}

// newMemoryKVImplWithStoreName creates a new in-memory KVImpl with a store name for metrics.
func newMemoryKVImplWithStoreName(storeName string) *memoryKVImpl {
	impl := newMemoryKVImpl()
	impl.storeName = storeName

	// Record backend type metric
	if metrics.StoreBackendType != nil {
		metrics.StoreBackendType.SetFloat64(2.0, "memory", storeName)
	}

	return impl
}

func (m *memoryKVImpl) Get(key string) ([]byte, bool) {
	val, found := m.kv[key]
	return val, found
}

func (m *memoryKVImpl) Set(key string, value []byte) error {
	m.kv[key] = value
	return nil
}

func (m *memoryKVImpl) Delete(key string) error {
	delete(m.kv, key)
	return nil
}

func (m *memoryKVImpl) Scan(prefix string, fn func(key string, value []byte) bool) error {
	// Collect matching keys first to ensure deterministic iteration order
	var keys []string
	for key := range m.kv {
		if strings.HasPrefix(key, prefix) {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)

	// Iterate in sorted order
	for _, key := range keys {
		if !fn(key, m.kv[key]) {
			break
		}
	}
	return nil
}

func (m *memoryKVImpl) Iter(fn func(key string, value []byte) error) error {
	// Collect all keys for deterministic iteration order
	keys := make([]string, 0, len(m.kv))
	for key := range m.kv {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	// Iterate in sorted order
	for _, key := range keys {
		if err := fn(key, m.kv[key]); err != nil {
			return err
		}
	}
	return nil
}

func (m *memoryKVImpl) Clear() error {
	m.kv = make(map[string][]byte)
	return nil
}

func (m *memoryKVImpl) Load(it iter.Seq2[marshaller.StoreDataEntry, error]) (*marshaller.StoreDataTrailer, error) {
	m.kv = make(map[string][]byte)

	var trailer marshaller.StoreDataTrailer
	for entry, err := range it {
		if err != nil {
			return nil, err
		}
		if t := entry.Trailer(); t != nil {
			trailer.DeletePrefixes = append(trailer.DeletePrefixes, t.DeletePrefixes...)
			if t.TotalSizeBytes > 0 {
				trailer.TotalSizeBytes = t.TotalSizeBytes
			}
			continue
		}
		kv := entry.KV()
		// TODO: Investigate this operation
		// Copy the key — UnmarshalIter uses unsafeGetString from a reused buffer,
		// so the string's backing bytes are only valid during this iteration.
		m.kv[strings.Clone(kv.Key)] = kv.Value
	}
	return &trailer, nil
}

func (m *memoryKVImpl) BatchSet(kv map[string][]byte) error {
	for k, v := range kv {
		m.kv[k] = v
	}
	return nil
}

func (m *memoryKVImpl) GetMany(keys []string) (map[string][]byte, error) {
	result := make(map[string][]byte, len(keys))
	for _, k := range keys {
		if v, ok := m.kv[k]; ok {
			result[k] = v
		}
	}
	return result, nil
}

func (m *memoryKVImpl) Save() iter.Seq2[marshaller.StoreDataEntry, error] {
	return func(yield func(marshaller.StoreDataEntry, error) bool) {
		keys := make([]string, 0, len(m.kv))
		for k := range m.kv {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		for _, k := range keys {
			if !yield(marshaller.NewKVEntry(k, m.kv[k]), nil) {
				return
			}
		}
	}
}

// Snapshot returns a pull-based iterator over the store in sorted key order.
// Only the key slice is copied; values are read from the live map, matching
// the memory-backend assumption that the store is not mutated during a save.
func (m *memoryKVImpl) Snapshot() (marshaller.KVSnapshotIter, error) {
	keys := make([]string, 0, len(m.kv))
	for k := range m.kv {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return &memorySnapshotIter{kv: m.kv, keys: keys}, nil
}

type memorySnapshotIter struct {
	kv   map[string][]byte
	keys []string
	idx  int
}

func (it *memorySnapshotIter) Next() (string, []byte, bool, error) {
	if it.idx >= len(it.keys) {
		return "", nil, false, nil
	}
	k := it.keys[it.idx]
	it.idx++
	return k, it.kv[k], true, nil
}

func (it *memorySnapshotIter) Close() error {
	it.kv = nil
	it.keys = nil
	return nil
}

func (m *memoryKVImpl) KeyCount() int {
	return len(m.kv)
}

func (m *memoryKVImpl) Close() error {
	// Drop the backing map so a lingering reference to this store (e.g. a
	// deferred close before GC, or an in-flight metadata goroutine) retains an
	// empty shell instead of the full multi-GB dataset.
	m.kv = nil
	return nil
}

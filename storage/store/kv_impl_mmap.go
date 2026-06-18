package store

import (
	"fmt"
	"iter"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/streamingfast/substreams/metrics"
	"github.com/streamingfast/substreams/storage/store/marshaller"
	"go.etcd.io/bbolt"
)

var (
	// defaultBucket is the single bucket name used in the bbolt database
	defaultBucket = []byte("store")
)

// mmapBatchSize is the maximum number of keys written per bbolt transaction
// during bulk operations (Load, BatchSet).
const mmapBatchSize = 10_000

// mmapKVImpl is a memory-mapped KVImpl backed by bbolt.
// Data lives on disk and is paged into memory by the OS as needed.
type mmapKVImpl struct {
	db        *bbolt.DB
	path      string
	storeName string
	noSync    bool // if true, bbolt skips fsync on each write
	keyCount  int  // tracked in-memory to avoid O(N) bucket.Stats() traversal
}

func openMmapDB(path string, noSync bool, initialMmapSize int) (*bbolt.DB, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create mmap directory %q: %w", dir, err)
	}

	db, err := bbolt.Open(path, 0600, &bbolt.Options{
		NoSync:          noSync,
		NoGrowSync:      noSync, // also skip truncate+fsync on file growth when nosync
		NoFreelistSync:  true,
		FreelistType:    bbolt.FreelistMapType,
		InitialMmapSize: initialMmapSize,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to open mmap database at %q: %w", path, err)
	}

	err = db.Update(func(tx *bbolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(defaultBucket)
		return err
	})
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("creating default bucket: %w", err)
	}

	return db, nil
}

func newMmapKVImplFromDB(db *bbolt.DB, path, storeName string, noSync bool) (*mmapKVImpl, error) {
	return &mmapKVImpl{
		db:        db,
		path:      path,
		storeName: storeName,
		noSync:    noSync,
	}, nil
}

func newMmapKVImpl(path string) (*mmapKVImpl, error) {
	if path == "" {
		tmpDir := os.TempDir()
		path = filepath.Join(tmpDir, fmt.Sprintf("substreams-store-%d.db", os.Getpid()))
	}
	db, err := openMmapDB(path, false, 0)
	if err != nil {
		return nil, err
	}
	return newMmapKVImplFromDB(db, path, "unknown", false)
}

// newMmapKVImplNoSync creates a mmap-backed KVImpl with NoSync=true.
// Writes skip fsync — faster but less durable. Acceptable for Substreams stores
// since they can always be rebuilt from object storage on crash.
func newMmapKVImplNoSync(path string) (*mmapKVImpl, error) {
	if path == "" {
		tmpDir := os.TempDir()
		path = filepath.Join(tmpDir, fmt.Sprintf("substreams-store-nosync-%d.db", os.Getpid()))
	}
	db, err := openMmapDB(path, true, 0)
	if err != nil {
		return nil, err
	}
	return newMmapKVImplFromDB(db, path, "unknown", true)
}

// newMmapKVImplWithConfig creates a new mmap-backed KVImpl with configuration.
func newMmapKVImplWithConfig(storeName, moduleHash string, cfg *MmapBackendConfig) (*mmapKVImpl, error) {
	hashPrefix := "nohash"
	if len(moduleHash) > 0 {
		hashPrefix = moduleHash[:min(8, len(moduleHash))]
	}

	baseDir := os.TempDir()
	if cfg.ScratchSpace != "" {
		baseDir = cfg.ScratchSpace
	}
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create scratch space directory %q: %w", baseDir, err)
	}

	pattern := fmt.Sprintf("substreams-store-%s-%s-*.db", storeName, hashPrefix)
	f, err := os.CreateTemp(baseDir, pattern)
	if err != nil {
		return nil, fmt.Errorf("failed to create mmap file for store %q: %w", storeName, err)
	}
	path := f.Name()
	f.Close()

	db, err := openMmapDB(path, true, 0) // NoSync: substreams stores are ephemeral and rebuilt from object storage on crash
	if err != nil {
		return nil, fmt.Errorf("failed to open mmap KVImpl for store %q at %q: %w", storeName, path, err)
	}

	impl, err := newMmapKVImplFromDB(db, path, storeName, true)
	if err != nil {
		return nil, err
	}

	if metrics.StoreBackendType != nil {
		metrics.StoreBackendType.SetFloat64(1.0, "mmap", storeName)
	}

	return impl, nil
}

func (b *mmapKVImpl) Get(key string) ([]byte, bool) {
	if b == nil || b.db == nil {
		panic(fmt.Sprintf("mmapKVImpl.Get called on nil instance (storeName=%s)", b.storeName))
	}
	if metrics.StoreMmapOperationsTotal != nil {
		metrics.StoreMmapOperationsTotal.Inc("get", b.storeName)
	}

	var value []byte
	err := b.db.View(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket(defaultBucket)
		if bucket == nil {
			return nil
		}
		v := bucket.Get([]byte(key))
		if v != nil {
			value = make([]byte, len(v))
			copy(value, v)
		}
		return nil
	})
	if err != nil || value == nil {
		return nil, false
	}
	return value, true
}

// Set writes a single key-value pair.
func (b *mmapKVImpl) Set(key string, value []byte) error {
	if metrics.StoreMmapOperationsTotal != nil {
		metrics.StoreMmapOperationsTotal.Inc("set", b.storeName)
	}
	return b.db.Update(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket(defaultBucket)
		if bucket == nil {
			return fmt.Errorf("internal error: default bucket not found")
		}
		if bucket.Get([]byte(key)) == nil {
			b.keyCount++
		}
		return bucket.Put([]byte(key), value)
	})
}

func (b *mmapKVImpl) Delete(key string) error {
	if metrics.StoreMmapOperationsTotal != nil {
		metrics.StoreMmapOperationsTotal.Inc("delete", b.storeName)
	}
	return b.db.Update(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket(defaultBucket)
		if bucket == nil {
			return nil
		}
		if bucket.Get([]byte(key)) != nil {
			b.keyCount--
		}
		return bucket.Delete([]byte(key))
	})
}

// Scan iterates over all keys with a given prefix.
func (b *mmapKVImpl) Scan(prefix string, fn func(key string, value []byte) bool) error {
	if metrics.StoreMmapOperationsTotal != nil {
		metrics.StoreMmapOperationsTotal.Inc("scan", b.storeName)
	}
	return b.db.View(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket(defaultBucket)
		if bucket == nil {
			return nil
		}
		c := bucket.Cursor()
		prefixBytes := []byte(prefix)
		for k, v := c.Seek(prefixBytes); k != nil && strings.HasPrefix(string(k), prefix); k, v = c.Next() {
			if !fn(string(k), v) {
				break
			}
		}
		return nil
	})
}

// Iter iterates over all keys in lexicographic order.
func (b *mmapKVImpl) Iter(fn func(key string, value []byte) error) error {
	if metrics.StoreMmapOperationsTotal != nil {
		metrics.StoreMmapOperationsTotal.Inc("iter", b.storeName)
	}
	return b.db.View(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket(defaultBucket)
		if bucket == nil {
			return nil
		}
		c := bucket.Cursor()
		for k, v := c.First(); k != nil; k, v = c.Next() {
			if err := fn(string(k), v); err != nil {
				return err
			}
		}
		return nil
	})
}

// GetMany retrieves multiple keys in a single db.View() transaction.
// This avoids the per-call transaction overhead of calling Get() in a loop,
// which is critical for Merge where we need to read all existing FullKV values.
func (b *mmapKVImpl) GetMany(keys []string) (map[string][]byte, error) {
	result := make(map[string][]byte, len(keys))
	err := b.db.View(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket(defaultBucket)
		if bucket == nil {
			return nil
		}
		for _, k := range keys {
			v := bucket.Get([]byte(k))
			if v != nil {
				valueCopy := make([]byte, len(v))
				copy(valueCopy, v)
				result[k] = valueCopy
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("mmap GetMany: %w", err)
	}
	return result, nil
}

func (b *mmapKVImpl) BatchSet(kv map[string][]byte) error {
	if metrics.StoreMmapOperationsTotal != nil {
		metrics.StoreMmapOperationsTotal.Inc("batch_set", b.storeName)
	}

	keys := make([]string, 0, len(kv))
	for k := range kv {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for i := 0; i < len(keys); i += mmapBatchSize {
		end := i + mmapBatchSize
		if end > len(keys) {
			end = len(keys)
		}
		chunk := keys[i:end]

		err := b.db.Update(func(tx *bbolt.Tx) error {
			bucket := tx.Bucket(defaultBucket)
			if bucket == nil {
				return fmt.Errorf("internal error: default bucket not found")
			}
			bucket.FillPercent = 1.0
			for _, k := range chunk {
				if bucket.Get([]byte(k)) == nil {
					b.keyCount++
				}
				if err := bucket.Put([]byte(k), kv[k]); err != nil {
					return fmt.Errorf("writing key %q: %w", k, err)
				}
			}
			return nil
		})
		if err != nil {
			return fmt.Errorf("mmap BatchSet failed for store %q with %d keys: %w", b.storeName, len(kv), err)
		}
	}
	return nil
}

// Clear wipes all data from the store without closing the database.
// Used by PartialKV.Roll() to reset between segments.
func (b *mmapKVImpl) Clear() error {
	b.keyCount = 0
	return b.db.Update(func(tx *bbolt.Tx) error {
		if err := tx.DeleteBucket(defaultBucket); err != nil && err != bbolt.ErrBucketNotFound {
			return fmt.Errorf("deleting existing bucket: %w", err)
		}
		_, err := tx.CreateBucket(defaultBucket)
		return err
	})
}

// Load replaces the store contents from an iterator of StoreDataEntry.
// This is the production hydration path — called by unmarshalIterInto when loading
// store snapshots from object storage.
// Entries are written in batches of mmapBatchSize for efficient B+tree page utilization.
func (b *mmapKVImpl) Load(it iter.Seq2[marshaller.StoreDataEntry, error]) (*marshaller.StoreDataTrailer, error) {
	// Clear existing data
	if err := b.Clear(); err != nil {
		return nil, fmt.Errorf("mmap Load: clearing bucket for store %q: %w", b.storeName, err)
	}

	// Write KV entries in sorted batches, accumulate trailer info
	type kv struct{ k, v []byte }
	batch := make([]kv, 0, mmapBatchSize)

	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		err := b.db.Update(func(tx *bbolt.Tx) error {
			bucket := tx.Bucket(defaultBucket)
			if bucket == nil {
				return fmt.Errorf("internal error: default bucket not found")
			}
			bucket.FillPercent = 1.0
			for _, e := range batch {
				if err := bucket.Put(e.k, e.v); err != nil {
					return err
				}
			}
			return nil
		})
		batch = batch[:0]
		return err
	}

	var trailer marshaller.StoreDataTrailer
	for entry, err := range it {
		if err != nil {
			return nil, fmt.Errorf("mmap Load: iterating entries: %w", err)
		}
		if t := entry.Trailer(); t != nil {
			trailer.DeletePrefixes = append(trailer.DeletePrefixes, t.DeletePrefixes...)
			if t.TotalSizeBytes > 0 {
				trailer.TotalSizeBytes = t.TotalSizeBytes
			}
			continue
		}
		kve := entry.KV()
		batch = append(batch, kv{[]byte(kve.Key), kve.Value})
		b.keyCount++
		if len(batch) >= mmapBatchSize {
			if err := flush(); err != nil {
				return nil, fmt.Errorf("mmap Load: flushing batch: %w", err)
			}
		}
	}

	if err := flush(); err != nil {
		return nil, fmt.Errorf("mmap Load: flushing final batch: %w", err)
	}

	return &trailer, nil
}

func (b *mmapKVImpl) Save() iter.Seq2[marshaller.StoreDataEntry, error] {
	return func(yield func(marshaller.StoreDataEntry, error) bool) {
		err := b.db.View(func(tx *bbolt.Tx) error {
			bucket := tx.Bucket(defaultBucket)
			if bucket == nil {
				return nil
			}
			return bucket.ForEach(func(k, v []byte) error {
				valueCopy := make([]byte, len(v))
				copy(valueCopy, v)
				if !yield(marshaller.NewKVEntry(string(k), valueCopy), nil) {
					return fmt.Errorf("iteration stopped")
				}
				return nil
			})
		})
		if err != nil && err.Error() != "iteration stopped" {
			yield(marshaller.StoreDataEntry{}, fmt.Errorf("iterating bucket: %w", err))
		}
	}
}

func (b *mmapKVImpl) KeyCount() int {
	return b.keyCount
}

func (b *mmapKVImpl) Close() error {
	if b.db != nil {
		if metrics.StoreMmapFileSizeBytes != nil {
			if stat, err := os.Stat(b.path); err == nil {
				metrics.StoreMmapFileSizeBytes.SetFloat64(float64(stat.Size()), b.storeName)
			}
		}
		if err := b.db.Close(); err != nil {
			return fmt.Errorf("closing mmap db: %w", err)
		}
	}
	if b.path != "" {
		if err := os.Remove(b.path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("removing mmap db file %s: %w", b.path, err)
		}
	}
	return nil
}

func (b *mmapKVImpl) Path() string {
	return b.path
}

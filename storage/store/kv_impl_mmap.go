package store

import (
	"bytes"
	"fmt"
	"iter"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

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

// snapshotBatchMaxKeys and snapshotBatchMaxBytes bound how many entries a
// snapshot iterator copies into heap per refill transaction.
const (
	snapshotBatchMaxKeys  = 10_000
	snapshotBatchMaxBytes = 8 << 20 // 8MiB
)

// mmapKVImpl is a memory-mapped KVImpl backed by bbolt.
// Data lives on disk and is paged into memory by the OS as needed.
type mmapKVImpl struct {
	db        *bbolt.DB
	path      string
	storeName string
	noSync    bool // if true, bbolt skips fsync on each write
	keyCount  int  // tracked in-memory to avoid O(N) bucket.Stats() traversal

	// snapMu gates writes against open snapshots. Roles are inverted from the
	// usual RWMutex reading: write operations hold the R side (they may run
	// concurrently with each other — bbolt serializes them internally), while
	// an open Snapshot holds the W side exclusively, blocking all writes (and
	// Close) for its lifetime so its paged reads across separate View
	// transactions observe a stable store.
	// CAUTION: writing to the store from the goroutine that holds an open
	// snapshot deadlocks.
	snapMu sync.RWMutex
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
		os.Remove(path) // openMmapDB may have left the file behind
		return nil, fmt.Errorf("failed to open mmap KVImpl for store %q at %q: %w", storeName, path, err)
	}

	// Unlink the backing file now that it is open. On unix the inode (and its
	// mmap) stays valid for the lifetime of the open db handle, but the
	// directory entry is gone — so no matter how this process dies (SIGKILL,
	// OOM, panic, a Load/Save that outlives a shutdown deadline), it can never
	// leave an orphan bbolt file behind. Space is reclaimed when Close() drops
	// the handle. Best-effort: on platforms that refuse to unlink an open file
	// (e.g. Windows) this is a no-op and Close() falls back to removing by path.
	os.Remove(path)

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
	b.snapMu.RLock()
	defer b.snapMu.RUnlock()
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
	b.snapMu.RLock()
	defer b.snapMu.RUnlock()
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
	b.snapMu.RLock()
	defer b.snapMu.RUnlock()
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
	b.snapMu.RLock()
	defer b.snapMu.RUnlock()
	return b.clearLocked()
}

// clearLocked is Clear without the snapMu gate, for callers already holding it.
// Nested RLock on the same goroutine can deadlock if a Snapshot's Lock is
// queued in between, so write paths must acquire snapMu exactly once.
func (b *mmapKVImpl) clearLocked() error {
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
	b.snapMu.RLock()
	defer b.snapMu.RUnlock()

	// Clear existing data
	if err := b.clearLocked(); err != nil {
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

// Snapshot returns a pull-based iterator over the store in lexicographic key
// order, reading in bounded batches (short View transaction per batch, resumed
// via cursor Seek on the last key). It never holds a bbolt read transaction
// across batches, so it does not pin the mmap or block file growth.
// Consistency across batches is guaranteed by holding snapMu exclusively:
// all writes (Set, Delete, BatchSet, Clear, Load) and Close block until the
// iterator is closed. The caller MUST Close() the returned iterator.
func (b *mmapKVImpl) Snapshot() (marshaller.KVSnapshotIter, error) {
	if metrics.StoreMmapOperationsTotal != nil {
		metrics.StoreMmapOperationsTotal.Inc("snapshot", b.storeName)
	}
	b.snapMu.Lock()
	return &mmapSnapshotIter{db: b.db, unlock: b.snapMu.Unlock}, nil
}

type mmapSnapshotIter struct {
	db     *bbolt.DB
	unlock func()

	buf       []marshaller.KeyValue // current batch, copied out of the mmap
	pos       int                   // read position within buf
	lastKey   []byte                // last key returned by the previous batch
	exhausted bool
	closed    bool
}

func (it *mmapSnapshotIter) Next() (string, []byte, bool, error) {
	if it.closed {
		return "", nil, false, fmt.Errorf("mmap snapshot iterator used after close")
	}
	if it.pos >= len(it.buf) {
		if it.exhausted {
			return "", nil, false, nil
		}
		if err := it.refill(); err != nil {
			return "", nil, false, fmt.Errorf("refilling snapshot batch: %w", err)
		}
		if len(it.buf) == 0 {
			return "", nil, false, nil
		}
	}
	kv := it.buf[it.pos]
	it.pos++
	return kv.Key, kv.Value, true, nil
}

// refill copies the next batch of entries out of a short-lived View
// transaction, bounded by snapshotBatchMaxKeys / snapshotBatchMaxBytes.
func (it *mmapSnapshotIter) refill() error {
	it.buf = it.buf[:0]
	it.pos = 0
	batchBytes := 0

	err := it.db.View(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket(defaultBucket)
		if bucket == nil {
			it.exhausted = true
			return nil
		}
		c := bucket.Cursor()

		var k, v []byte
		if it.lastKey == nil {
			k, v = c.First()
		} else {
			k, v = c.Seek(it.lastKey)
			if k != nil && bytes.Equal(k, it.lastKey) {
				k, v = c.Next()
			}
		}

		for ; k != nil; k, v = c.Next() {
			val := make([]byte, len(v))
			copy(val, v)
			it.buf = append(it.buf, marshaller.KeyValue{Key: string(k), Value: val})
			batchBytes += len(k) + len(v)
			if len(it.buf) >= snapshotBatchMaxKeys || batchBytes >= snapshotBatchMaxBytes {
				return nil
			}
		}
		it.exhausted = true
		return nil
	})
	if err != nil {
		return err
	}
	if n := len(it.buf); n > 0 {
		it.lastKey = []byte(it.buf[n-1].Key)
	}
	return nil
}

func (it *mmapSnapshotIter) Close() error {
	if it.closed {
		return nil
	}
	it.closed = true
	it.buf = nil
	it.unlock()
	return nil
}

func (b *mmapKVImpl) KeyCount() int {
	return b.keyCount
}

func (b *mmapKVImpl) Close() error {
	// Wait for any open snapshot before closing and removing the db file;
	// a leaked snapshot iterator makes this hang, loudly surfacing the bug
	// instead of yanking the file from under an in-flight save.
	b.snapMu.RLock()
	defer b.snapMu.RUnlock()

	var closeErr error
	if b.db != nil {
		if metrics.StoreMmapFileSizeBytes != nil {
			if stat, err := os.Stat(b.path); err == nil {
				metrics.StoreMmapFileSizeBytes.SetFloat64(float64(stat.Size()), b.storeName)
			}
		}
		closeErr = b.db.Close()
	}
	// Always attempt removal, even if db.Close failed: the file is usually
	// already gone (unlinked at open time), but on platforms where the
	// open-file unlink was a no-op this is what reclaims it. Skipping it on a
	// db.Close error would leak the file.
	if b.path != "" {
		if err := os.Remove(b.path); err != nil && !os.IsNotExist(err) {
			if closeErr != nil {
				return fmt.Errorf("closing mmap db: %w (and removing file %s: %v)", closeErr, b.path, err)
			}
			return fmt.Errorf("removing mmap db file %s: %w", b.path, err)
		}
	}
	if closeErr != nil {
		return fmt.Errorf("closing mmap db: %w", closeErr)
	}
	return nil
}

func (b *mmapKVImpl) Path() string {
	return b.path
}

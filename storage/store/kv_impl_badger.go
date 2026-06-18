package store

import (
	"fmt"
	"os"

	"github.com/dgraph-io/badger/v4"
)

// badgerKVImpl is a KVImpl backed by Badger (LSM-tree).
// Badger separates keys from values (value log), making it efficient for
// large values (>1KB) unlike bbolt which stores everything in the B+tree.
type badgerKVImpl struct {
	db        *badger.DB
	path      string
	storeName string
}

func newBadgerKVImpl(path string, storeName string) (*badgerKVImpl, error) {
	if path == "" {
		var err error
		path, err = os.MkdirTemp("", "substreams-store-badger-*")
		if err != nil {
			return nil, fmt.Errorf("creating temp dir for badger: %w", err)
		}
	}

	opts := badger.DefaultOptions(path).
		WithLogger(nil). // suppress badger's internal logging
		WithSyncWrites(false)

	db, err := badger.Open(opts)
	if err != nil {
		return nil, fmt.Errorf("opening badger db at %q: %w", path, err)
	}

	return &badgerKVImpl{db: db, path: path, storeName: storeName}, nil
}

func (b *badgerKVImpl) Get(key string) ([]byte, bool) {
	var value []byte
	err := b.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(key))
		if err == badger.ErrKeyNotFound {
			return nil
		}
		if err != nil {
			return err
		}
		value, err = item.ValueCopy(nil)
		return err
	})
	if err != nil || value == nil {
		return nil, false
	}
	return value, true
}

func (b *badgerKVImpl) GetMany(keys []string) (map[string][]byte, error) {
	result := make(map[string][]byte, len(keys))
	err := b.db.View(func(txn *badger.Txn) error {
		for _, k := range keys {
			item, err := txn.Get([]byte(k))
			if err == badger.ErrKeyNotFound {
				continue
			}
			if err != nil {
				return err
			}
			v, err := item.ValueCopy(nil)
			if err != nil {
				return err
			}
			result[k] = v
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("badger GetMany: %w", err)
	}
	return result, nil
}

func (b *badgerKVImpl) Set(key string, value []byte) error {
	return b.db.Update(func(txn *badger.Txn) error {
		return txn.Set([]byte(key), value)
	})
}

func (b *badgerKVImpl) Delete(key string) error {
	return b.db.Update(func(txn *badger.Txn) error {
		return txn.Delete([]byte(key))
	})
}

func (b *badgerKVImpl) Scan(prefix string, fn func(key string, value []byte) bool) error {
	return b.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Prefix = []byte(prefix)
		it := txn.NewIterator(opts)
		defer it.Close()
		for it.Rewind(); it.Valid(); it.Next() {
			item := it.Item()
			v, err := item.ValueCopy(nil)
			if err != nil {
				return err
			}
			if !fn(string(item.Key()), v) {
				break
			}
		}
		return nil
	})
}

func (b *badgerKVImpl) Iter(fn func(key string, value []byte) error) error {
	return b.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		it := txn.NewIterator(opts)
		defer it.Close()
		for it.Rewind(); it.Valid(); it.Next() {
			item := it.Item()
			v, err := item.ValueCopy(nil)
			if err != nil {
				return err
			}
			if err := fn(string(item.Key()), v); err != nil {
				return err
			}
		}
		return nil
	})
}

func (b *badgerKVImpl) BatchSet(kv map[string][]byte) error {
	wb := b.db.NewWriteBatch()
	for k, v := range kv {
		if err := wb.Set([]byte(k), v); err != nil {
			wb.Cancel()
			return fmt.Errorf("badger BatchSet: %w", err)
		}
	}
	return wb.Flush()
}

func (b *badgerKVImpl) Load(kv map[string][]byte) error {
	// Drop all existing data by dropping and recreating the db.
	// Badger doesn't have a bucket concept so we delete all keys first.
	if err := b.db.DropAll(); err != nil {
		return fmt.Errorf("badger Load: dropping existing data: %w", err)
	}
	return b.BatchSet(kv)
}

func (b *badgerKVImpl) Save() (map[string][]byte, error) {
	result := make(map[string][]byte)
	err := b.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		it := txn.NewIterator(opts)
		defer it.Close()
		for it.Rewind(); it.Valid(); it.Next() {
			item := it.Item()
			v, err := item.ValueCopy(nil)
			if err != nil {
				return err
			}
			result[string(item.Key())] = v
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("badger Snapshot: %w", err)
	}
	return result, nil
}

func (b *badgerKVImpl) KeyCount() int {
	var count int
	b.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false // keys only
		it := txn.NewIterator(opts)
		defer it.Close()
		for it.Rewind(); it.Valid(); it.Next() {
			count++
		}
		return nil
	})
	return count
}

func (b *badgerKVImpl) Close() error {
	if b.db != nil {
		if err := b.db.Close(); err != nil {
			return fmt.Errorf("closing badger db: %w", err)
		}
	}
	if b.path != "" {
		if err := os.RemoveAll(b.path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("removing badger dir %s: %w", b.path, err)
		}
	}
	return nil
}

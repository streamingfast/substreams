package store

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"testing"

	"github.com/streamingfast/bstream"
	"github.com/streamingfast/dstore"
	pbssinternal "github.com/streamingfast/substreams/pb/sf/substreams/intern/v2"
	"github.com/streamingfast/substreams/storage/store/marshaller"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestFullKV_Save_Load_Empty_MapNotNil(t *testing.T) {
	var writtenBytes []byte
	store := dstore.NewMockStore(func(base string, f io.Reader) (err error) {
		writtenBytes, err = io.ReadAll(f)
		return err
	})
	store.OpenObjectFunc = func(ctx context.Context, name string) (out io.ReadCloser, err error) {
		return io.NopCloser(bytes.NewBuffer(writtenBytes)), nil
	}

	kvs := &FullKV{
		baseStore: &baseStore{
			kvImpl: newMemoryKVImpl(),

			kvOps:      &pbssinternal.Operations{},
			logger:     zap.NewNop(),
			marshaller: marshaller.Default(),

			Config: &Config{
				moduleInitialBlock: 0,
				objStore:           store,
			},
		},
	}

	file, writer, err := kvs.Save(123)
	require.NoError(t, err)

	err = writer.Write(context.Background())
	require.NoError(t, err)

	kvl := &FullKV{
		baseStore: &baseStore{
			kvImpl: newMemoryKVImpl(),

			kvOps:      &pbssinternal.Operations{},
			logger:     zap.NewNop(),
			marshaller: marshaller.Default(),

			Config: &Config{
				moduleInitialBlock: 0,
				objStore:           store,
			},
		},
	}

	err = kvl.Load(context.Background(), file)
	require.NoError(t, err)
	require.NotNilf(t, kvl.kvImpl, "kvl.kvImpl is nil")
}

// TestFullKV_Load_CanceledContext_NotCorrupt ensures a load aborted by context
// cancellation reports the cancellation, not ErrInvalidFullKVFile — otherwise
// callers would delete a perfectly valid remote store file.
func TestFullKV_Load_CanceledContext_NotCorrupt(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	store := dstore.NewMockStore(nil)
	store.OpenObjectFunc = func(context.Context, string) (io.ReadCloser, error) {
		cancel() // the stream is torn down mid-read
		return io.NopCloser(&errReaderReadCloser{err: context.Canceled}), nil
	}

	kv := &FullKV{
		baseStore: &baseStore{
			kvImpl:     newMemoryKVImpl(),
			kvOps:      &pbssinternal.Operations{},
			logger:     zap.NewNop(),
			marshaller: marshaller.Default(),
			Config:     &Config{moduleInitialBlock: 0, objStore: store},
		},
	}

	err := kv.Load(ctx, NewCompleteFileInfo("test", 0, 100))
	require.Error(t, err)
	require.ErrorIs(t, err, context.Canceled, "canceled load must surface cancellation")
	require.NotErrorIs(t, err, ErrInvalidFullKVFile, "canceled load must not look like corruption")
}

type errReaderReadCloser struct{ err error }

func (r *errReaderReadCloser) Read([]byte) (int, error) { return 0, r.err }
func (r *errReaderReadCloser) Close() error             { return nil }

func TestFullKV_QuickSave_QuickLoad_Empty(t *testing.T) {
	var writtenBytes []byte
	mockStore := dstore.NewMockStore(func(base string, f io.Reader) (err error) {
		writtenBytes, err = io.ReadAll(f)
		return err
	})
	mockStore.OpenObjectFunc = func(ctx context.Context, name string) (out io.ReadCloser, err error) {
		return io.NopCloser(bytes.NewBuffer(writtenBytes)), nil
	}

	// Create store with empty KV map
	kvs := &FullKV{
		baseStore: &baseStore{
			kvImpl:     newMemoryKVImpl(),
			kvOps:      &pbssinternal.Operations{},
			logger:     zap.NewNop(),
			marshaller: marshaller.Default(),
			Config: &Config{
				moduleInitialBlock: 0,
				quickSaveStore:     mockStore,
			},
		},
	}

	// Test QuickSave
	blockHash := "test_block_hash_123"
	err := kvs.QuickSave(context.Background(), blockHash)
	require.NoError(t, err)

	// Create new store to load into
	kvl := &FullKV{
		baseStore: &baseStore{
			kvImpl:     newMemoryKVImpl(),
			kvOps:      &pbssinternal.Operations{},
			logger:     zap.NewNop(),
			marshaller: marshaller.Default(),
			Config: &Config{
				moduleInitialBlock: 0,
				quickSaveStore:     mockStore,
			},
		},
	}

	// Test QuickLoad
	blockRef := bstream.NewBlockRef(blockHash, 123)
	err = kvl.QuickLoad(context.Background(), blockRef)
	require.NoError(t, err)
	require.NotNil(t, kvl.kvImpl, "kvl.kvImpl should not be nil after QuickLoad")
	require.Equal(t, 0, kvl.kvImpl.KeyCount(), "kvl.kvImpl should be empty")
}

func TestFullKV_QuickSave_QuickLoad_WithData(t *testing.T) {
	var writtenBytes []byte
	mockStore := dstore.NewMockStore(func(base string, f io.Reader) (err error) {
		writtenBytes, err = io.ReadAll(f)
		return err
	})
	mockStore.OpenObjectFunc = func(ctx context.Context, name string) (out io.ReadCloser, err error) {
		return io.NopCloser(bytes.NewBuffer(writtenBytes)), nil
	}

	// Create store with test data
	testData := map[string][]byte{
		"key1": []byte("value1"),
		"key2": []byte("value2"),
		"key3": []byte("value3"),
	}

	kvs := &FullKV{
		baseStore: &baseStore{
			kvImpl: func() KVImpl {
				impl := newMemoryKVImpl()
				impl.Load(mapToIter(testData))
				return impl
			}(),
			kvOps:          &pbssinternal.Operations{},
			logger:         zap.NewNop(),
			marshaller:     marshaller.Default(),
			totalSizeBytes: 100, // Set some size for testing
			Config: &Config{
				moduleInitialBlock: 10,
				quickSaveStore:     mockStore,
			},
		},
	}

	// Test QuickSave
	blockHash := "test_block_hash_with_data"
	err := kvs.QuickSave(context.Background(), blockHash)
	require.NoError(t, err)

	// Create new store to load into
	kvl := &FullKV{
		baseStore: &baseStore{
			kvImpl:     newMemoryKVImpl(),
			kvOps:      &pbssinternal.Operations{},
			logger:     zap.NewNop(),
			marshaller: marshaller.Default(),
			Config: &Config{
				moduleInitialBlock: 10,
				quickSaveStore:     mockStore,
			},
		},
	}

	// Test QuickLoad
	blockRef := bstream.NewBlockRef(blockHash, 456)
	err = kvl.QuickLoad(context.Background(), blockRef)
	require.NoError(t, err)
	require.NotNil(t, kvl.kvImpl, "kvl.kvImpl should not be nil after QuickLoad")
	require.Equal(t, len(testData), kvl.kvImpl.KeyCount(), "kvl.kvImpl should have same number of entries")

	// Verify all data was loaded correctly
	for key, expectedValue := range testData {
		snapshot, err := saveToMap(kvl.kvImpl.Save())
		require.NoError(t, err)
		actualValue, exists := snapshot[key]
		require.True(t, exists, "key %s should exist", key)
		require.Equal(t, expectedValue, actualValue, "value for key %s should match", key)
	}

	// Verify total size was loaded (marshaller calculates actual size)
	require.Greater(t, kvl.totalSizeBytes, uint64(0), "totalSizeBytes should be greater than 0")
}

func TestFullKV_QuickLoad_NoQuickSaveStore(t *testing.T) {
	kvs := &FullKV{
		baseStore: &baseStore{
			kvImpl:     newMemoryKVImpl(),
			kvOps:      &pbssinternal.Operations{},
			logger:     zap.NewNop(),
			marshaller: marshaller.Default(),
			Config: &Config{
				moduleInitialBlock: 0,
				quickSaveStore:     nil, // No quick save store
			},
		},
	}

	blockRef := bstream.NewBlockRef("test_block_hash", 100)
	err := kvs.QuickLoad(context.Background(), blockRef)
	require.Error(t, err)
	require.Equal(t, ErrNoQuickSaveStore, err)
}

func TestFullKV_QuickSave_NoQuickSaveStore(t *testing.T) {
	kvs := &FullKV{
		baseStore: &baseStore{
			kvImpl:     newMemoryKVImpl(),
			kvOps:      &pbssinternal.Operations{},
			logger:     zap.NewNop(),
			marshaller: marshaller.Default(),
			Config: &Config{
				moduleInitialBlock: 0,
				quickSaveStore:     nil, // No quick save store
			},
		},
	}

	err := kvs.QuickSave(context.Background(), "test_block_hash")
	require.Error(t, err)
	require.Equal(t, ErrNoQuickSaveStore, err)
}

func TestFullKV_QuickLoad_FileNotFound(t *testing.T) {
	mockStore := dstore.NewMockStore(func(base string, f io.Reader) (err error) {
		return nil
	})
	mockStore.OpenObjectFunc = func(ctx context.Context, name string) (out io.ReadCloser, err error) {
		return nil, fmt.Errorf("file not found")
	}

	kvs := &FullKV{
		baseStore: &baseStore{
			kvImpl:     newMemoryKVImpl(),
			kvOps:      &pbssinternal.Operations{},
			logger:     zap.NewNop(),
			marshaller: marshaller.Default(),
			Config: &Config{
				moduleInitialBlock: 0,
				moduleHash:         "test_module",
				quickSaveStore:     mockStore,
			},
		},
	}

	blockRef := bstream.NewBlockRef("nonexistent_block_hash", 200)
	err := kvs.QuickLoad(context.Background(), blockRef)
	require.Error(t, err)
	require.Contains(t, err.Error(), "opening file")
	require.Contains(t, err.Error(), "test_module")
}

func TestFullKV_QuickSave_QuickLoad_LargeData(t *testing.T) {
	var writtenBytes []byte
	mockStore := dstore.NewMockStore(func(base string, f io.Reader) (err error) {
		writtenBytes, err = io.ReadAll(f)
		return err
	})
	mockStore.OpenObjectFunc = func(ctx context.Context, name string) (out io.ReadCloser, err error) {
		return io.NopCloser(bytes.NewBuffer(writtenBytes)), nil
	}

	// Create store with large data to test streaming marshaller path
	testData := make(map[string][]byte)
	for i := 0; i < 1000; i++ {
		key := fmt.Sprintf("key_%d", i)
		value := make([]byte, 1024) // 1KB per value
		for j := range value {
			value[j] = byte(i % 256)
		}
		testData[key] = value
	}

	kvs := &FullKV{
		baseStore: &baseStore{
			kvImpl: func() KVImpl {
				impl := newMemoryKVImpl()
				impl.Load(mapToIter(testData))
				return impl
			}(),
			kvOps:          &pbssinternal.Operations{},
			logger:         zap.NewNop(),
			marshaller:     marshaller.Default(),
			totalSizeBytes: 1024 * 1024, // 1MB to trigger streaming marshaller
			Config: &Config{
				moduleInitialBlock: 0,
				quickSaveStore:     mockStore,
			},
		},
	}

	// Test QuickSave
	blockHash := "large_data_block_hash"
	err := kvs.QuickSave(context.Background(), blockHash)
	require.NoError(t, err)

	// Create new store to load into
	kvl := &FullKV{
		baseStore: &baseStore{
			kvImpl:     newMemoryKVImpl(),
			kvOps:      &pbssinternal.Operations{},
			logger:     zap.NewNop(),
			marshaller: marshaller.Default(),
			Config: &Config{
				moduleInitialBlock: 0,
				quickSaveStore:     mockStore,
			},
		},
	}

	// Test QuickLoad
	blockRef := bstream.NewBlockRef(blockHash, 1000)
	err = kvl.QuickLoad(context.Background(), blockRef)
	require.NoError(t, err)
	require.NotNil(t, kvl.kvImpl, "kvl.kvImpl should not be nil after QuickLoad")
	require.Equal(t, len(testData), kvl.kvImpl.KeyCount(), "kvl.kvImpl should have same number of entries")

	// Verify a few entries (not all for performance)
	snapshot, err := saveToMap(kvl.kvImpl.Save())
	require.NoError(t, err)
	for i := 0; i < 10; i++ {
		key := fmt.Sprintf("key_%d", i)
		expectedValue := testData[key]
		actualValue, exists := snapshot[key]
		require.True(t, exists, "key %s should exist", key)
		require.Equal(t, expectedValue, actualValue, "value for key %s should match", key)
	}
}

func TestFullKV_QuickSave_QuickLoad_RoundTrip(t *testing.T) {
	var writtenBytes []byte
	mockStore := dstore.NewMockStore(func(base string, f io.Reader) (err error) {
		writtenBytes, err = io.ReadAll(f)
		return err
	})
	mockStore.OpenObjectFunc = func(ctx context.Context, name string) (out io.ReadCloser, err error) {
		return io.NopCloser(bytes.NewBuffer(writtenBytes)), nil
	}

	// Test multiple save/load cycles
	testCases := []map[string][]byte{
		{
			"initial": []byte("data"),
		},
		{
			"initial": []byte("data"),
			"added":   []byte("new_data"),
		},
		{
			"initial": []byte("modified_data"),
			"added":   []byte("new_data"),
			"third":   []byte("third_data"),
		},
	}

	store := &FullKV{
		baseStore: &baseStore{
			kvImpl:     newMemoryKVImpl(),
			kvOps:      &pbssinternal.Operations{},
			logger:     zap.NewNop(),
			marshaller: marshaller.Default(),
			Config: &Config{
				moduleInitialBlock: 0,
				quickSaveStore:     mockStore,
			},
		},
	}

	for i, testData := range testCases {
		blockHash := fmt.Sprintf("block_hash_%d", i)

		// Update store data
		store.kvImpl.Load(mapToIter(testData))
		store.totalSizeBytes = uint64(len(testData) * 10) // Approximate size

		// Save
		err := store.QuickSave(context.Background(), blockHash)
		require.NoError(t, err)

		// Load into new store
		loadStore := &FullKV{
			baseStore: &baseStore{
				kvImpl:     newMemoryKVImpl(),
				kvOps:      &pbssinternal.Operations{},
				logger:     zap.NewNop(),
				marshaller: marshaller.Default(),
				Config: &Config{
					moduleInitialBlock: 0,
					quickSaveStore:     mockStore,
				},
			},
		}

		blockRef := bstream.NewBlockRef(blockHash, uint64(i+1))
		err = loadStore.QuickLoad(context.Background(), blockRef)
		require.NoError(t, err)

		// Verify data matches
		require.Equal(t, len(testData), loadStore.kvImpl.KeyCount(), "iteration %d: kv length should match", i)
		snapshot, err := saveToMap(loadStore.kvImpl.Save())
		require.NoError(t, err)
		for key, expectedValue := range testData {
			actualValue, exists := snapshot[key]
			require.True(t, exists, "iteration %d: key %s should exist", i, key)
			require.Equal(t, expectedValue, actualValue, "iteration %d: value for key %s should match", i, key)
		}
		require.Greater(t, loadStore.totalSizeBytes, uint64(0), "iteration %d: total size should be greater than 0", i)
	}
}

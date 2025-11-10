package store

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"testing"

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
			kv: map[string][]byte{},

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
			kv: map[string][]byte{},

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
	require.NotNilf(t, kvl.kv, "kvl.kv is nil")
}

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
			kv:         make(map[string][]byte),
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
			kv:         make(map[string][]byte),
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
	err = kvl.QuickLoad(context.Background(), blockHash)
	require.NoError(t, err)
	require.NotNil(t, kvl.kv, "kvl.kv should not be nil after QuickLoad")
	require.Equal(t, 0, len(kvl.kv), "kvl.kv should be empty")
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
			kv:             testData,
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
			kv:         make(map[string][]byte),
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
	err = kvl.QuickLoad(context.Background(), blockHash)
	require.NoError(t, err)
	require.NotNil(t, kvl.kv, "kvl.kv should not be nil after QuickLoad")
	require.Equal(t, len(testData), len(kvl.kv), "kvl.kv should have same number of entries")

	// Verify all data was loaded correctly
	for key, expectedValue := range testData {
		actualValue, exists := kvl.kv[key]
		require.True(t, exists, "key %s should exist", key)
		require.Equal(t, expectedValue, actualValue, "value for key %s should match", key)
	}

	// Verify total size was loaded (marshaller calculates actual size)
	require.Greater(t, kvl.totalSizeBytes, uint64(0), "totalSizeBytes should be greater than 0")
}

func TestFullKV_QuickLoad_NoQuickSaveStore(t *testing.T) {
	kvs := &FullKV{
		baseStore: &baseStore{
			kv:         make(map[string][]byte),
			kvOps:      &pbssinternal.Operations{},
			logger:     zap.NewNop(),
			marshaller: marshaller.Default(),
			Config: &Config{
				moduleInitialBlock: 0,
				quickSaveStore:     nil, // No quick save store
			},
		},
	}

	err := kvs.QuickLoad(context.Background(), "test_block_hash")
	require.Error(t, err)
	require.Equal(t, ErrNoQuickSaveStore, err)
}

func TestFullKV_QuickSave_NoQuickSaveStore(t *testing.T) {
	kvs := &FullKV{
		baseStore: &baseStore{
			kv:         make(map[string][]byte),
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
			kv:         make(map[string][]byte),
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

	err := kvs.QuickLoad(context.Background(), "nonexistent_block_hash")
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
			kv:             testData,
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
			kv:         make(map[string][]byte),
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
	err = kvl.QuickLoad(context.Background(), blockHash)
	require.NoError(t, err)
	require.NotNil(t, kvl.kv, "kvl.kv should not be nil after QuickLoad")
	require.Equal(t, len(testData), len(kvl.kv), "kvl.kv should have same number of entries")

	// Verify a few entries (not all for performance)
	for i := 0; i < 10; i++ {
		key := fmt.Sprintf("key_%d", i)
		expectedValue := testData[key]
		actualValue, exists := kvl.kv[key]
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
			kv:         make(map[string][]byte),
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
		store.kv = make(map[string][]byte)
		for k, v := range testData {
			store.kv[k] = v
		}
		store.totalSizeBytes = uint64(len(testData) * 10) // Approximate size

		// Save
		err := store.QuickSave(context.Background(), blockHash)
		require.NoError(t, err)

		// Load into new store
		loadStore := &FullKV{
			baseStore: &baseStore{
				kv:         make(map[string][]byte),
				kvOps:      &pbssinternal.Operations{},
				logger:     zap.NewNop(),
				marshaller: marshaller.Default(),
				Config: &Config{
					moduleInitialBlock: 0,
					quickSaveStore:     mockStore,
				},
			},
		}

		err = loadStore.QuickLoad(context.Background(), blockHash)
		require.NoError(t, err)

		// Verify data matches
		require.Equal(t, len(testData), len(loadStore.kv), "iteration %d: kv length should match", i)
		for key, expectedValue := range testData {
			actualValue, exists := loadStore.kv[key]
			require.True(t, exists, "iteration %d: key %s should exist", i, key)
			require.Equal(t, expectedValue, actualValue, "iteration %d: value for key %s should match", i, key)
		}
		require.Greater(t, loadStore.totalSizeBytes, uint64(0), "iteration %d: total size should be greater than 0", i)
	}
}

package store

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

// createTestStore creates a baseStore with the specified backend (reads SUBSTREAMS_STORE_BACKEND from env).
// scratchSpace sets the directory for mmap files; empty uses OS temp dir.
func createTestStore(t *testing.T, name string, updatePolicy pbsubstreams.Module_KindStore_UpdatePolicy, scratchSpace string) *baseStore {
	cfg := &Config{
		name:               name,
		updatePolicy:       updatePolicy,
		valueType:          "string",
		moduleInitialBlock: 0,
		moduleHash:         "test_hash_12345",
		appendLimit:        8_388_608,
		totalSizeLimit:     DefaultStoreSizeLimit,
		itemSizeLimit:      10_485_760,
		scratchSpace:       scratchSpace,
	}
	return cfg.newBaseStore(zap.NewNop())
}

// TestKVImplBackends tests both mmap and memory backends with realistic store operations
func TestKVImplBackends(t *testing.T) {
	tests := []struct {
		name         string
		envBackend   string
		scratchSpace string
		expectMmap   bool
	}{
		{
			// mmap is opt-in: with no explicit backend the store must default
			// to the in-memory backend, never mmap.
			name:         "default (memory)",
			envBackend:   "",
			scratchSpace: "",
			expectMmap:   false,
		},
		{
			name:         "explicit mmap",
			envBackend:   "mmap",
			scratchSpace: "",
			expectMmap:   true,
		},
		{
			name:         "explicit memory",
			envBackend:   "memory",
			scratchSpace: "",
			expectMmap:   false,
		},
		{
			name:         "mmap with custom scratch space",
			envBackend:   "mmap",
			scratchSpace: t.TempDir(),
			expectMmap:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set environment variables
			if tt.envBackend != "" {
				os.Setenv("SUBSTREAMS_STORE_BACKEND", tt.envBackend)
				defer os.Unsetenv("SUBSTREAMS_STORE_BACKEND")
			}

			// Create a store
			baseStore := createTestStore(t, "test_store", pbsubstreams.Module_KindStore_UPDATE_POLICY_SET, tt.scratchSpace)
			require.NotNil(t, baseStore)
			require.NotNil(t, baseStore.kvImpl)

			// Verify backend type
			if tt.expectMmap {
				_, isMmap := baseStore.kvImpl.(*mmapKVImpl)
				assert.True(t, isMmap, "expected mmap backend")
			} else {
				_, isMemory := baseStore.kvImpl.(*memoryKVImpl)
				assert.True(t, isMemory, "expected memory backend")
			}

			// Test basic operations
			testBasicKVOperations(t, baseStore)

			// Test large dataset (stress test for mmap) - use a fresh store
			if tt.expectMmap {
				// Close previous store
				closeErr := baseStore.Close()
				require.NoError(t, closeErr)

				// Create fresh store for large dataset test
				baseStore = createTestStore(t, "test_store_large", pbsubstreams.Module_KindStore_UPDATE_POLICY_SET, tt.scratchSpace)
				testLargeDataset(t, baseStore, tt.scratchSpace)
			}

			// Cleanup
			closeErr := baseStore.Close()
			assert.NoError(t, closeErr)
		})
	}
}

func testBasicKVOperations(t *testing.T, s *baseStore) {
	// Set some values
	s.Set(1, "key1", "value1")
	s.Set(2, "key2", "value2")
	s.Set(3, "key3", "value3")
	s.Set(4, "prefix:foo", "foo_value")
	s.Set(5, "prefix:bar", "bar_value")

	// Flush to apply operations to kvImpl
	err := s.Flush()
	require.NoError(t, err)

	// Get values
	val, found := s.GetLast("key1")
	require.True(t, found)
	assert.Equal(t, []byte("value1"), val)

	val, found = s.GetLast("key2")
	require.True(t, found)
	assert.Equal(t, []byte("value2"), val)

	// Test prefix scan
	count := 0
	scanErr := s.kvImpl.Scan("prefix:", func(key string, value []byte) bool {
		count++
		assert.Contains(t, key, "prefix:")
		return true
	})
	require.NoError(t, scanErr)
	assert.Equal(t, 2, count, "should find 2 keys with prefix:")

	// Test delete
	s.DeletePrefix(6, "key2")
	err = s.Flush()
	require.NoError(t, err)

	_, found = s.GetLast("key2")
	assert.False(t, found, "key2 should be deleted")

	// Verify other keys still exist
	_, found = s.GetLast("key1")
	assert.True(t, found, "key1 should still exist")
}

func testLargeDataset(t *testing.T, s *baseStore, baseDir string) {
	// Write 1,000 keys (reduced from 10K for faster testing)
	numKeys := 1_000
	for i := 0; i < numKeys; i++ {
		key := fmt.Sprintf("large_key_%06d", i) // Use proper formatting instead of rune
		value := make([]byte, 1024)             // 1KB per value
		for j := range value {
			value[j] = byte(i % 256)
		}
		s.SetBytes(uint64(i), key, value)
	}

	// Flush all operations
	err := s.Flush()
	require.NoError(t, err)

	// Verify some random keys
	testKeys := []int{0, 100, 500, 999}
	for _, i := range testKeys {
		key := fmt.Sprintf("large_key_%06d", i)
		val, found := s.GetLast(key)
		require.True(t, found, "key %s should exist", key)
		require.Equal(t, 1024, len(val), "value should be 1KB")
		assert.Equal(t, byte(i%256), val[0], "value should match expected pattern")
	}

	// If using mmap with custom base dir, verify file was created
	if baseDir != "" {
		mmapImpl, ok := s.kvImpl.(*mmapKVImpl)
		require.True(t, ok, "should be mmap implementation")

		// The path is derived from the configured base dir and store name...
		assert.Contains(t, mmapImpl.path, baseDir, "mmap file should be in configured base dir")
		filename := filepath.Base(mmapImpl.path)
		assert.Contains(t, filename, "test_store", "filename should contain store name")

		fi, err := os.Stat(mmapImpl.path)
		require.NoError(t, err, "mmap file should exist on disk while the store is live")
		assert.False(t, fi.IsDir(), "mmap path should be a regular file")
	}

	// Test iteration over large dataset
	iterCount := 0
	iterErr := s.Iter(func(key string, value []byte) error {
		iterCount++
		return nil
	})
	require.NoError(t, iterErr)
	assert.Equal(t, numKeys, iterCount, "should iterate over all keys")
}

// TestKVImplFallback tests that mmap gracefully falls back to memory on failure
func TestKVImplFallback(t *testing.T) {
	// Pass an invalid scratch space to trigger fallback to memory
	os.Setenv("SUBSTREAMS_STORE_BACKEND", "mmap")
	defer os.Unsetenv("SUBSTREAMS_STORE_BACKEND")

	baseStore := createTestStore(t, "fallback_test", pbsubstreams.Module_KindStore_UPDATE_POLICY_SET, "/this/path/should/not/exist/or/be/writable")
	require.NotNil(t, baseStore)

	// Should have fallen back to memory
	_, isMemory := baseStore.kvImpl.(*memoryKVImpl)
	assert.True(t, isMemory, "should fall back to memory backend on mmap failure")

	// Should still work
	baseStore.Set(1, "test", "value")
	flushErr := baseStore.Flush()
	require.NoError(t, flushErr)

	val, found := baseStore.GetLast("test")
	assert.True(t, found)
	assert.Equal(t, []byte("value"), val)

	baseStore.Close()
}

// TestKVImplSnapshot tests the Snapshot functionality used by merge operations
func TestKVImplSnapshot(t *testing.T) {
	backends := []struct {
		name       string
		envBackend string
	}{
		{"mmap", "mmap"},
		{"memory", "memory"},
	}

	for _, backend := range backends {
		t.Run(backend.name, func(t *testing.T) {
			os.Setenv("SUBSTREAMS_STORE_BACKEND", backend.envBackend)
			defer os.Unsetenv("SUBSTREAMS_STORE_BACKEND")

			baseStore := createTestStore(t, "snapshot_test", pbsubstreams.Module_KindStore_UPDATE_POLICY_SET, "")
			defer baseStore.Close()

			// Add data
			baseStore.Set(1, "a", "value_a")
			baseStore.Set(2, "b", "value_b")
			baseStore.Set(3, "c", "value_c")

			// Flush operations
			err := baseStore.Flush()
			require.NoError(t, err)

			// Take snapshot
			snapshot, snapshotErr := saveToMap(baseStore.kvImpl.Save())
			require.NoError(t, snapshotErr)
			require.NotNil(t, snapshot)

			// Verify snapshot contents
			assert.Equal(t, 3, len(snapshot))
			assert.Equal(t, []byte("value_a"), snapshot["a"])
			assert.Equal(t, []byte("value_b"), snapshot["b"])
			assert.Equal(t, []byte("value_c"), snapshot["c"])

			// Modify original store
			baseStore.Set(4, "d", "value_d")
			modErr := baseStore.Flush()
			require.NoError(t, modErr)

			// Snapshot should be unchanged
			assert.Equal(t, 3, len(snapshot))
			assert.NotContains(t, snapshot, "d")
		})
	}
}

// TestKVImplLoad tests the Load functionality used when restoring from snapshots
func TestKVImplLoad(t *testing.T) {
	backends := []struct {
		name       string
		envBackend string
	}{
		{"mmap", "mmap"},
		{"memory", "memory"},
	}

	for _, backend := range backends {
		t.Run(backend.name, func(t *testing.T) {
			os.Setenv("SUBSTREAMS_STORE_BACKEND", backend.envBackend)
			defer os.Unsetenv("SUBSTREAMS_STORE_BACKEND")

			baseStore := createTestStore(t, "load_test", pbsubstreams.Module_KindStore_UPDATE_POLICY_SET, "")
			defer baseStore.Close()

			// Prepare data to load (simulating loading from disk on startup)
			loadData := map[string][]byte{
				"loaded1": []byte("loaded_value1"),
				"loaded2": []byte("loaded_value2"),
				"loaded3": []byte("loaded_value3"),
			}

			// Load should populate the kvImpl
			_, loadErr := baseStore.kvImpl.Load(mapToIter(loadData))
			require.NoError(t, loadErr)

			// Verify loaded keys exist in kvImpl
			val, found := baseStore.kvImpl.Get("loaded1")
			require.True(t, found)
			assert.Equal(t, []byte("loaded_value1"), val)

			val, found = baseStore.kvImpl.Get("loaded2")
			require.True(t, found)
			assert.Equal(t, []byte("loaded_value2"), val)

			val, found = baseStore.kvImpl.Get("loaded3")
			require.True(t, found)
			assert.Equal(t, []byte("loaded_value3"), val)

			// Verify count
			assert.Equal(t, 3, baseStore.kvImpl.KeyCount())
		})
	}
}

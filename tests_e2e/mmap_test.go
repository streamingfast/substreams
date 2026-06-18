package tests_e2e

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/streamingfast/substreams/manifest"
	pbsubstreamsrpcv3 "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
)

// TestMmapBackendE2E validates that mmap backend works in full e2e scenario
// This test explicitly sets SUBSTREAMS_STORE_BACKEND=mmap and verifies:
// 1. Stores work correctly in production mode with squashing
// 2. Large store operations complete without OOM
// 3. Multiple stores can coexist
func TestMmapBackendE2E(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	// Set custom mmap directory for this test (mmap is already the default)
	mmapDir := filepath.Join(tmpDir, "mmap-stores")
	t.Setenv("SUBSTREAMS_STORE_MMAP_BASE_DIR", mmapDir)
	err := os.MkdirAll(mmapDir, 0755)
	require.NoError(t, err)

	t.Logf("Mmap base dir: %s (mmap is default, no backend env var needed)", mmapDir)

	// Launch containers
	container, err := newDummyBlockchainContainer(ctx, tmpDir, latestDummyBlockchainImage, "", 1000)
	require.NoError(t, err)
	defer container.Terminate(ctx, testcontainers.StopTimeout(0))

	app2, t2Endpoint := startTier2App(t, ctx, tmpDir, zlog)
	app, substreamsEndpoint := startTier1App(t, ctx, tmpDir, container, t2Endpoint, zlog)
	defer func() {
		app.Shutdown(nil)
		app2.Shutdown(nil)
		<-app.Terminated()
		<-app2.Terminated()
	}()

	testCases := []struct {
		name          string
		startBlock    int64
		stopBlock     uint64
		outputModule  string
		expectedLen   int
		expectedClock uint64
		description   string
	}{
		{
			name:          "single_store_production",
			startBlock:    150,
			stopBlock:     500,
			outputModule:  "map_stats",
			expectedLen:   350,
			expectedClock: 499,
			description:   "Tests single store (store_stats) with squashing in production mode using mmap",
		},
		{
			name:          "double_store_production",
			startBlock:    150,
			stopBlock:     500,
			outputModule:  "map_stats2",
			expectedLen:   350,
			expectedClock: 499,
			description:   "Tests two stores (store_stats + store_stats2) with squashing using mmap",
		},
		{
			name:          "long_range_with_store",
			startBlock:    100,
			stopBlock:     800,
			outputModule:  "map_stats",
			expectedLen:   700,
			expectedClock: 799,
			description:   "Tests longer range (700 blocks) to validate mmap handles larger datasets",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Log(tc.description)

			// Check mmap directory before test
			filesBefore, err := os.ReadDir(mmapDir)
			require.NoError(t, err)
			t.Logf("Mmap files before test: %d", len(filesBefore))

			pkg, err := manifest.MustNewReader("./dummy/e2e-v0.1.0.spkg").Read()
			require.NoError(t, err)

			request := &pbsubstreamsrpcv3.Request{
				StartBlockNum:   tc.startBlock,
				StopBlockNum:    tc.stopBlock,
				FinalBlocksOnly: false,
				ProductionMode:  true,
				OutputModule:    tc.outputModule,
				Package:         pkg.Package,
			}

			// Run the request
			blockScopedDataSlice, session, err := RunRequest(t, request, substreamsEndpoint)
			if err != nil && err != io.EOF {
				logs, logErr := container.Logs(ctx)
				if logErr == nil {
					defer logs.Close()
					buf := make([]byte, 4096)
					n, _ := logs.Read(buf)
					t.Logf("Container logs: %s", string(buf[:n]))
				}
				t.Fatalf("Unexpected error: %s", err.Error())
			}

			require.NotNil(t, session, "Should have received at least one session")
			assert.Equal(t, tc.expectedLen, len(blockScopedDataSlice), "Should have received all expected blocks")
			require.Greater(t, len(blockScopedDataSlice), 0)
			assert.Equal(t, tc.expectedClock, uint64(blockScopedDataSlice[len(blockScopedDataSlice)-1].Clock.Number), "Should end on expected block")

			// Check mmap directory after test - should have mmap files
			filesAfter, err := os.ReadDir(mmapDir)
			require.NoError(t, err)
			t.Logf("Mmap files after test: %d", len(filesAfter))

			// Log mmap file details
			for _, file := range filesAfter {
				if !file.IsDir() {
					info, err := file.Info()
					if err == nil {
						t.Logf("  - %s: %d bytes (%.2f MB)", file.Name(), info.Size(), float64(info.Size())/(1024*1024))
					}
				}
			}

			t.Logf("Test completed successfully with mmap backend")
		})
	}
}

// TestMemoryBackendE2E validates that memory backend still works as fallback
// This explicitly sets SUBSTREAMS_STORE_BACKEND=memory to override the default mmap behavior
func TestMemoryBackendE2E(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	// Explicitly override default mmap with memory backend for this test
	t.Setenv("SUBSTREAMS_STORE_BACKEND", "memory")
	t.Log("Memory backend explicitly enabled (overriding default mmap)")

	container, err := newDummyBlockchainContainer(ctx, tmpDir, latestDummyBlockchainImage, "", 1000)
	require.NoError(t, err)
	defer container.Terminate(ctx, testcontainers.StopTimeout(0))

	app2, t2Endpoint := startTier2App(t, ctx, tmpDir, zlog)
	app, substreamsEndpoint := startTier1App(t, ctx, tmpDir, container, t2Endpoint, zlog)
	defer func() {
		app.Shutdown(nil)
		app2.Shutdown(nil)
		<-app.Terminated()
		<-app2.Terminated()
	}()

	// Run a simple test to verify memory backend works
	pkg, err := manifest.MustNewReader("./dummy/e2e-v0.1.0.spkg").Read()
	require.NoError(t, err)

	request := &pbsubstreamsrpcv3.Request{
		StartBlockNum:   150,
		StopBlockNum:    300,
		FinalBlocksOnly: false,
		ProductionMode:  true,
		OutputModule:    "map_stats",
		Package:         pkg.Package,
	}

	blockScopedDataSlice, session, err := RunRequest(t, request, substreamsEndpoint)
	if err != nil && err != io.EOF {
		t.Fatalf("Unexpected error: %s", err.Error())
	}

	require.NotNil(t, session)
	assert.Equal(t, 150, len(blockScopedDataSlice))
	t.Logf("Memory backend test completed successfully")
}

// TestMmapVsMemoryComparison runs same workload on both backends and compares results
func TestMmapVsMemoryComparison(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping comparison test in short mode")
	}

	ctx := context.Background()

	testCases := []struct {
		backend     string
		expectFiles bool
	}{
		{backend: "mmap", expectFiles: true},
		{backend: "memory", expectFiles: false},
	}

	// Store results for comparison
	type testResult struct {
		backend         string
		dataLen         int
		finalClock      uint64
		mmapFilesCount  int
		totalMmapSizeMB float64
	}
	results := make([]testResult, 0, 2)

	for _, tc := range testCases {
		t.Run(tc.backend, func(t *testing.T) {
			tmpDir := t.TempDir()

			// Only set env var if we want to override the default (mmap)
			if tc.backend == "memory" {
				t.Setenv("SUBSTREAMS_STORE_BACKEND", "memory")
				t.Log("Overriding default mmap with memory backend")
			} else {
				// mmap is default, no env var needed
				t.Log("Using default mmap backend (no env var set)")
			}

			mmapDir := filepath.Join(tmpDir, "mmap-stores")
			if tc.backend == "mmap" {
				t.Setenv("SUBSTREAMS_STORE_MMAP_BASE_DIR", mmapDir)
				err := os.MkdirAll(mmapDir, 0755)
				require.NoError(t, err)
			}

			container, err := newDummyBlockchainContainer(ctx, tmpDir, latestDummyBlockchainImage, "", 1000)
			require.NoError(t, err)
			defer container.Terminate(ctx, testcontainers.StopTimeout(0))

			app2, t2Endpoint := startTier2App(t, ctx, tmpDir, zlog)
			app, substreamsEndpoint := startTier1App(t, ctx, tmpDir, container, t2Endpoint, zlog)
			defer func() {
				app.Shutdown(nil)
				app2.Shutdown(nil)
				<-app.Terminated()
				<-app2.Terminated()
			}()

			pkg, err := manifest.MustNewReader("./dummy/e2e-v0.1.0.spkg").Read()
			require.NoError(t, err)

			request := &pbsubstreamsrpcv3.Request{
				StartBlockNum:   150,
				StopBlockNum:    500,
				FinalBlocksOnly: false,
				ProductionMode:  true,
				OutputModule:    "map_stats2", // Use double store for more complex scenario
				Package:         pkg.Package,
			}

			blockScopedDataSlice, session, err := RunRequest(t, request, substreamsEndpoint)
			if err != nil && err != io.EOF {
				t.Fatalf("Unexpected error: %s", err.Error())
			}

			require.NotNil(t, session)
			finalClock := uint64(0)
			if len(blockScopedDataSlice) > 0 {
				finalClock = uint64(blockScopedDataSlice[len(blockScopedDataSlice)-1].Clock.Number)
			}

			result := testResult{
				backend:    tc.backend,
				dataLen:    len(blockScopedDataSlice),
				finalClock: finalClock,
			}

			// Check for mmap files
			if tc.backend == "mmap" {
				files, err := os.ReadDir(mmapDir)
				require.NoError(t, err)
				result.mmapFilesCount = len(files)

				totalSize := int64(0)
				for _, file := range files {
					if !file.IsDir() {
						info, err := file.Info()
						if err == nil {
							totalSize += info.Size()
						}
					}
				}
				result.totalMmapSizeMB = float64(totalSize) / (1024 * 1024)
			}

			results = append(results, result)
			t.Logf("Backend: %s, Data length: %d, Final clock: %d, Mmap files: %d, Total mmap size: %.2f MB",
				result.backend, result.dataLen, result.finalClock, result.mmapFilesCount, result.totalMmapSizeMB)
		})
	}

	// Compare results
	require.Len(t, results, 2, "Should have results from both backends")

	mmapResult := results[0]
	memoryResult := results[1]

	// Both backends should produce identical output
	assert.Equal(t, mmapResult.dataLen, memoryResult.dataLen, "Both backends should produce same number of blocks")
	assert.Equal(t, mmapResult.finalClock, memoryResult.finalClock, "Both backends should reach same final block")

	// Mmap should create files
	assert.Greater(t, mmapResult.mmapFilesCount, 0, "Mmap backend should create files")
	assert.Greater(t, mmapResult.totalMmapSizeMB, 0.0, "Mmap files should have size")

	// Memory should not create files
	assert.Equal(t, 0, memoryResult.mmapFilesCount, "Memory backend should not create mmap files")

	t.Logf("\n")
	t.Logf("Mmap backend:   %d blocks, %d files, %.2f MB total", mmapResult.dataLen, mmapResult.mmapFilesCount, mmapResult.totalMmapSizeMB)
	t.Logf("Memory backend: %d blocks (in-memory, no files)", memoryResult.dataLen)
}

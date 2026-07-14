package tests_e2e

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/streamingfast/substreams/manifest"
	pbsubstreamsrpcv3 "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
)

// mmapDBFilePoller watches dir for *.db files while a request runs. The mmap
// backend creates a temp .db file per store and removes it on Close (see
// storage/store/kv_impl_mmap.go), so the files only exist transiently during
// processing and are gone by the time a request returns. Poll fast to observe
// the peak count instead of checking the dir after the fact.
type mmapDBFilePoller struct {
	dir     string
	maxDB   int64
	maxSize int64
	stop    chan struct{}
	done    chan struct{}
}

func startMmapDBFilePoller(dir string) *mmapDBFilePoller {
	p := &mmapDBFilePoller{
		dir:  dir,
		stop: make(chan struct{}),
		done: make(chan struct{}),
	}
	go func() {
		defer close(p.done)
		ticker := time.NewTicker(5 * time.Millisecond)
		defer ticker.Stop()
		for {
			p.sample()
			select {
			case <-p.stop:
				p.sample() // final sample to catch anything created just before stop
				return
			case <-ticker.C:
			}
		}
	}()
	return p
}

func (p *mmapDBFilePoller) sample() {
	entries, err := os.ReadDir(p.dir)
	if err != nil {
		return
	}
	var count, size int64
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".db") {
			continue
		}
		count++
		if info, err := e.Info(); err == nil {
			size += info.Size()
		}
	}
	if count > atomic.LoadInt64(&p.maxDB) {
		atomic.StoreInt64(&p.maxDB, count)
	}
	if size > atomic.LoadInt64(&p.maxSize) {
		atomic.StoreInt64(&p.maxSize, size)
	}
}

// Stop halts polling and returns the peak .db file count and total size seen.
func (p *mmapDBFilePoller) Stop() (maxDBFiles int, maxTotalSize int64) {
	var once sync.Once
	once.Do(func() { close(p.stop) })
	<-p.done
	return int(atomic.LoadInt64(&p.maxDB)), atomic.LoadInt64(&p.maxSize)
}

// TestMmapBackendE2E validates that mmap backend works in full e2e scenario.
// mmap is the default backend, so no SUBSTREAMS_STORE_BACKEND env var is set here.
// It verifies:
// 1. Stores work correctly in production mode with squashing
// 2. Large store operations complete without OOM
// 3. Multiple stores can coexist
func TestMmapBackendE2E(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	mmapDir := filepath.Join(tmpDir, "mmap-stores")
	err := os.MkdirAll(mmapDir, 0755)
	require.NoError(t, err)

	t.Logf("Mmap base dir: %s", mmapDir)

	// Launch containers
	container, err := newDummyBlockchainContainer(ctx, tmpDir, latestDummyBlockchainImage, "", 1000)
	require.NoError(t, err)
	defer container.Terminate(ctx, testcontainers.StopTimeout(0))

	app2, t2Endpoint := startTier2App(t, ctx, tmpDir, zlog, mmapDir)
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

			// Watch for the transient mmap .db files while the request runs;
			// the backend removes them on Close, so they won't exist afterward.
			poller := startMmapDBFilePoller(mmapDir)

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

			maxDBFiles, maxTotalSize := poller.Stop()

			require.NotNil(t, session, "Should have received at least one session")
			assert.Equal(t, tc.expectedLen, len(blockScopedDataSlice), "Should have received all expected blocks")
			require.Greater(t, len(blockScopedDataSlice), 0)
			assert.Equal(t, tc.expectedClock, uint64(blockScopedDataSlice[len(blockScopedDataSlice)-1].Clock.Number), "Should end on expected block")

			t.Logf("Peak mmap .db files during test: %d (%.2f MB)", maxDBFiles, float64(maxTotalSize)/(1024*1024))
			assert.Greater(t, maxDBFiles, 0, "mmap backend must create .db files in the scratch dir while running")

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
			var t2ScratchSpace string
			if tc.backend == "mmap" {
				err := os.MkdirAll(mmapDir, 0755)
				require.NoError(t, err)
				t2ScratchSpace = mmapDir
			}

			container, err := newDummyBlockchainContainer(ctx, tmpDir, latestDummyBlockchainImage, "", 1000)
			require.NoError(t, err)
			defer container.Terminate(ctx, testcontainers.StopTimeout(0))

			app2, t2Endpoint := startTier2App(t, ctx, tmpDir, zlog, t2ScratchSpace)
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

			var poller *mmapDBFilePoller
			if tc.backend == "mmap" {
				poller = startMmapDBFilePoller(mmapDir)
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

			// The mmap backend removes each store's .db file on Close, so
			// sample the peak seen during the run instead of after it.
			if tc.backend == "mmap" {
				maxDBFiles, maxTotalSize := poller.Stop()
				result.mmapFilesCount = maxDBFiles
				result.totalMmapSizeMB = float64(maxTotalSize) / (1024 * 1024)
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

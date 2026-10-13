package integration

import (
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/streamingfast/bstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/streamingfast/substreams/manifest"
	"github.com/streamingfast/substreams/orchestrator/stage"
)

// The store of this package starts at block 1. Snapshots are built while the chain starts at
// block 0, then the first streamable block is raised to 20 (chain truncation).
func TestStoreTruncation(t *testing.T) {
	manifest.TestUseSimpleHash = true
	defer func() { bstream.GetProtocolFirstStreamableBlock = 0 }()

	spkg := "./testdata/simple_substreams/substreams-test-v0.1.0.spkg"
	storeHash := hex.EncodeToString([]byte("setup_test_store_add_i64"))
	outputModule := "assert_test_store_add_i64"

	buildSnapshots := func(t *testing.T, tempDir string) {
		bstream.GetProtocolFirstStreamableBlock = 0
		run := newTestRun(t, 25, 30, 30, 0, outputModule, spkg)
		run.ProductionMode = true
		require.NoError(t, run.RunWithTempDir(t, "build_snapshots", tempDir))
		assertFileProduced(t, tempDir, storeHash+"/states/0000000030-0000000001.kv.zst")
	}

	truncatedRun := func(t *testing.T, startBlock int64, allowTruncation bool) *testRun {
		bstream.GetProtocolFirstStreamableBlock = 20
		run := newTestRun(t, startBlock, 40, 40, 20, outputModule, spkg)
		run.ProductionMode = true
		run.AllowStoreTruncation = allowTruncation
		return run
	}

	t.Run("resumes from a snapshot taken before truncation", func(t *testing.T) {
		tempDir := t.TempDir()
		buildSnapshots(t, tempDir)

		run := truncatedRun(t, 35, true)
		var jobSegments []int
		run.JobCallback = func(unit stage.Unit) { jobSegments = append(jobSegments, unit.Segment) }
		require.NoError(t, run.RunWithTempDir(t, "truncated", tempDir))

		assert.Equal(t, []int{3}, jobSegments, "only segment [30, 40) can be processed, resuming from the snapshot at block 30")
		mapOutput := run.MapOutputString(outputModule)
		assert.Equal(t, 5, strings.Count(mapOutput, "assert_test_store_add_i64: 0801"), mapOutput)
		assertFileProduced(t, tempDir, storeHash+"/states/0000000040-0000000001.kv.zst")
	})

	t.Run("refuses a store without snapshot to resume from", func(t *testing.T) {
		run := truncatedRun(t, 35, true)
		err := run.Run(t, "truncated_no_snapshot")
		require.Error(t, err)
		assert.Contains(t, err.Error(), `store "setup_test_store_add_i64" starts at block 1, before the first streamable block 20`)
	})

	t.Run("refuses modules starting before the first streamable block without the flag", func(t *testing.T) {
		tempDir := t.TempDir()
		buildSnapshots(t, tempDir)

		run := truncatedRun(t, 35, false)
		err := run.RunWithTempDir(t, "truncation_not_allowed", tempDir)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "smaller than first streamable block 20")
	})

	t.Run("refuses blocks below the first streamable block", func(t *testing.T) {
		tempDir := t.TempDir()
		buildSnapshots(t, tempDir)

		run := truncatedRun(t, 15, true)
		err := run.RunWithTempDir(t, "below_first_streamable_block", tempDir)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must be >= 20")
	})
}

func assertFileProduced(t *testing.T, tempDir string, wanted string) {
	t.Helper()
	produced, empty := listFiles(t, tempDir)
	var actual []string
	for _, f := range append(produced, empty...) {
		parts := strings.Split(f, string(os.PathSeparator))
		actual = append(actual, filepath.Join(parts[3:]...))
	}
	assert.Contains(t, actual, wanted)
}

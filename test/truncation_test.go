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
func TestTailTruncation(t *testing.T) {
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

	truncatedRun := func(t *testing.T, startBlock int64, allowTailTruncation bool) *testRun {
		bstream.GetProtocolFirstStreamableBlock = 20
		run := newTestRun(t, startBlock, 40, 40, 20, outputModule, spkg)
		run.ProductionMode = true
		run.AllowTailTruncation = allowTailTruncation
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
		assert.Contains(t, err.Error(), `store "setup_test_store_add_i64" has initial block 1, but this endpoint truncated the chain's tail at block 20 and has no snapshot of the store at that block`)
	})

	t.Run("linear handoff at the first streamable block", func(t *testing.T) {
		for _, productionMode := range []bool{true, false} {
			for _, withSnapshot := range []bool{true, false} {
				tempDir := t.TempDir()
				if withSnapshot {
					buildSnapshots(t, tempDir)
				}

				run := truncatedRun(t, 20, true)
				run.LinearHandoffBlockNum = 20
				run.ExclusiveEndBlock = 25
				run.ProductionMode = productionMode
				err := run.RunWithTempDir(t, "handoff_at_first_streamable_block", tempDir)
				if withSnapshot {
					require.NoError(t, err, "production mode: %v", productionMode)
					continue
				}
				require.Error(t, err, "production mode: %v", productionMode)
				assert.Contains(t, err.Error(), `store "setup_test_store_add_i64" has initial block 1, but this endpoint truncated the chain's tail at block 20 and has no snapshot of the store at that block`)
			}
		}
	})

	t.Run("dev mode with a first streamable block between segment boundaries", func(t *testing.T) {
		tempDir := t.TempDir()
		buildSnapshots(t, tempDir)

		bstream.GetProtocolFirstStreamableBlock = 25
		for _, startBlock := range []uint64{25, 27, 33} {
			run := newTestRun(t, int64(startBlock), startBlock, startBlock+5, 25, outputModule, spkg)
			run.AllowTailTruncation = true
			err := run.RunWithTempDir(t, "first_streamable_block_between_boundaries", tempDir)
			if startBlock >= 30 {
				require.NoError(t, err, "start block %d resumes from the snapshot at block 30", startBlock)
				continue
			}
			require.Error(t, err, "start block %d needs the store at block 25, where it has no snapshot", startBlock)
			assert.Contains(t, err.Error(), `store "setup_test_store_add_i64" has initial block 1, but this endpoint truncated the chain's tail at block 25 and has no snapshot of the store at that block`)
		}
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

// A store without initial block is named after the first streamable block: once that block is
// raised, the snapshots written before are named after another block and must not be resumed from.
func TestFirstStreamableBlockRaised_StoreWithoutInitialBlock(t *testing.T) {
	manifest.TestUseSimpleHash = true
	defer func() { bstream.GetProtocolFirstStreamableBlock = 0 }()

	spkg := "./testdata/simple_substreams/substreams-test-v0.1.0.spkg"
	storeHash := hex.EncodeToString([]byte("setup_test_store_add_i64"))
	outputModule := "assert_test_store_add_i64"
	newRun := func(t *testing.T, startBlock int64, exclusiveEndBlock, firstStreamableBlock uint64) *testRun {
		bstream.GetProtocolFirstStreamableBlock = firstStreamableBlock
		run := newTestRun(t, startBlock, exclusiveEndBlock, exclusiveEndBlock, firstStreamableBlock, outputModule, spkg)
		run.ProductionMode = true
		for _, mod := range run.Package.Modules.Modules {
			mod.InitialBlock = 0
		}
		return run
	}

	tempDir := t.TempDir()
	require.NoError(t, newRun(t, 25, 30, 0).RunWithTempDir(t, "build_snapshots", tempDir))
	assertFileProduced(t, tempDir, storeHash+"/states/0000000030-0000000000.kv.zst")

	require.NoError(t, newRun(t, 35, 40, 20).RunWithTempDir(t, "first_streamable_block_raised", tempDir))
	assertFileProduced(t, tempDir, storeHash+"/states/0000000030-0000000020.kv.zst")
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

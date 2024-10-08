package integration

import (
	"context"
	"encoding/hex"
	"fmt"
	"math/big"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/streamingfast/bstream"
	"github.com/streamingfast/dmetering"
	"github.com/streamingfast/substreams/block"
	"github.com/streamingfast/substreams/manifest"
	"github.com/streamingfast/substreams/orchestrator/stage"
	"github.com/streamingfast/substreams/orchestrator/work"
	"github.com/streamingfast/substreams/reqctx"
	_ "github.com/streamingfast/substreams/wasm/wazero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestForkHandling(t *testing.T) {
	type response struct {
		id                string
		extraStoreOutputs []string
		output            string
		undo              bool
	}

	undoUpTo := func(id string) response {
		return response{
			id:   id,
			undo: true,
		}
	}

	tests := []struct {
		name                  string
		module                string
		start                 int64
		exclusiveEnd          uint64
		production            bool
		forkBlockRefs         []*ForkBlockRef
		inProcessValidation   func(ctx *execContext)
		expectedResponseNames []response
	}{
		{
			name:         "production",
			module:       "assert_test_store_add_bigint",
			start:        1,
			exclusiveEnd: 7,
			production:   true,
			forkBlockRefs: []*ForkBlockRef{
				{blockRef: bstream.NewBlockRef("1a", 1), previousID: "0a", libBlockRef: bstream.NewBlockRef("0a", 0)},
				{blockRef: bstream.NewBlockRef("2a", 2), previousID: "1a", libBlockRef: bstream.NewBlockRef("0a", 0)},
				{blockRef: bstream.NewBlockRef("3a", 3), previousID: "2a", libBlockRef: bstream.NewBlockRef("0a", 0)},
				{blockRef: bstream.NewBlockRef("2b", 2), previousID: "1a", libBlockRef: bstream.NewBlockRef("0a", 0)},
				{blockRef: bstream.NewBlockRef("4a", 4), previousID: "3a", libBlockRef: bstream.NewBlockRef("0a", 0)},
				{blockRef: bstream.NewBlockRef("3b", 3), previousID: "2b", libBlockRef: bstream.NewBlockRef("0a", 0)},
				{blockRef: bstream.NewBlockRef("4b", 4), previousID: "3b", libBlockRef: bstream.NewBlockRef("0a", 0)},
				{blockRef: bstream.NewBlockRef("5b", 5), previousID: "4b", libBlockRef: bstream.NewBlockRef("0a", 0)},
				{blockRef: bstream.NewBlockRef("5a", 5), previousID: "4a", libBlockRef: bstream.NewBlockRef("0a", 0)},
				{blockRef: bstream.NewBlockRef("6a", 6), previousID: "5a", libBlockRef: bstream.NewBlockRef("4a", 4)},
				{blockRef: bstream.NewBlockRef("7a", 7), previousID: "6a", libBlockRef: bstream.NewBlockRef("4a", 4)},
			},
			expectedResponseNames: []response{
				{id: "1a", output: "assert_test_store_add_bigint"},
				{id: "2a", output: "assert_test_store_add_bigint"},
				{id: "3a", output: "assert_test_store_add_bigint"},
				{id: "4a", output: "assert_test_store_add_bigint"},
				undoUpTo("1a"),
				{id: "2b", output: "assert_test_store_add_bigint"},
				{id: "3b", output: "assert_test_store_add_bigint"},
				{id: "4b", output: "assert_test_store_add_bigint"},
				{id: "5b", output: "assert_test_store_add_bigint"},
				undoUpTo("1a"),
				{id: "2a", output: "assert_test_store_add_bigint"},
				{id: "3a", output: "assert_test_store_add_bigint"},
				{id: "4a", output: "assert_test_store_add_bigint"},
				{id: "5a", output: "assert_test_store_add_bigint"},
				{id: "6a", output: "assert_test_store_add_bigint"},
			},
			inProcessValidation: func(ctx *execContext) {
				if ctx.block.Number == 6 {
					s, found := ctx.stores.Get("setup_test_store_add_bigint")
					require.True(t, found)
					bytes, found := s.GetLast("a.key.pos")
					require.True(t, found)
					bi := &big.Int{}
					_, success := bi.SetString(string(bytes), 10)
					require.True(t, success)
					require.Equal(t, "6", bi.String())
					bytes, found = s.GetLast("a.key.neg")
					require.True(t, found)
					_, success = bi.SetString(string(bytes), 10)
					require.True(t, success)
					require.Equal(t, "-6", bi.String())
				}
			},
		},
		{
			name:         "dev",
			module:       "assert_test_store_add_bigint",
			start:        1,
			exclusiveEnd: 4,
			production:   false,
			forkBlockRefs: []*ForkBlockRef{
				{blockRef: bstream.NewBlockRef("1a", 1), previousID: "0a", libBlockRef: bstream.NewBlockRef("0a", 0)},
				{blockRef: bstream.NewBlockRef("2b", 2), previousID: "1a", libBlockRef: bstream.NewBlockRef("0a", 0)},
				{blockRef: bstream.NewBlockRef("2a", 2), previousID: "1a", libBlockRef: bstream.NewBlockRef("0a", 0)},
				{blockRef: bstream.NewBlockRef("3a", 3), previousID: "2a", libBlockRef: bstream.NewBlockRef("0a", 0)},
				{blockRef: bstream.NewBlockRef("4a", 4), previousID: "3a", libBlockRef: bstream.NewBlockRef("0a", 0)},
			},
			expectedResponseNames: []response{
				{id: "1a", output: "assert_test_store_add_bigint", extraStoreOutputs: []string{"setup_test_store_add_bigint"}},
				{id: "2b", output: "assert_test_store_add_bigint", extraStoreOutputs: []string{"setup_test_store_add_bigint"}},
				undoUpTo("1a"),
				{id: "2a", output: "assert_test_store_add_bigint", extraStoreOutputs: []string{"setup_test_store_add_bigint"}},
				{id: "3a", output: "assert_test_store_add_bigint", extraStoreOutputs: []string{"setup_test_store_add_bigint"}},
				{id: "4a", output: "assert_test_store_add_bigint", extraStoreOutputs: []string{"setup_test_store_add_bigint"}},
			},
		},
	}

	for _, test := range tests {

		t.Run(test.name, func(t *testing.T) {
			run := newTestRun(t, 1, 1, 7, 0, test.module, "./testdata/simple_substreams/substreams-test-v0.1.0.spkg")
			run.NewBlockGenerator = func(startBlock uint64, inclusiveStopBlock uint64) TestBlockGenerator {
				return &ForkBlockGenerator{
					initialLIB:    bstream.NewBlockRef("0a", 0),
					forkBlockRefs: test.forkBlockRefs,
				}
			}

			run.ModuleName = test.module
			run.ProductionMode = test.production
			run.BlockProcessedCallback = test.inProcessValidation
			err := run.Run(t, test.name)

			require.NoError(t, err)
			i := 0
			for _, resp := range run.Responses {
				if resp.GetProgress() != nil {
					continue
				}
				if resp.GetSession() != nil {
					continue
				}
				if undo := resp.GetBlockUndoSignal(); undo != nil {
					assert.Truef(t, test.expectedResponseNames[i].undo, "received undo, expecting block %s", test.expectedResponseNames[i].id)
					assert.Equal(t, test.expectedResponseNames[i].id, undo.LastValidBlock.Id, "inside undo message, wrong ID")
					i++
					continue
				}
				require.Greater(t, len(test.expectedResponseNames), i, "too many response")

				require.NotNil(t, test.expectedResponseNames[i])
				require.False(t, test.expectedResponseNames[i].undo, "received undo where we shouldn't")

				data := resp.GetBlockScopedData()
				require.NotNil(t, data.Output)
				assert.Equal(t, test.expectedResponseNames[i].id, data.Clock.Id)

				var outputStoreNames []string
				for _, out := range data.DebugStoreOutputs {
					outputStoreNames = append(outputStoreNames, out.Name)
				}

				assert.Equal(t, test.expectedResponseNames[i].extraStoreOutputs, outputStoreNames)
				assert.Equal(t, test.expectedResponseNames[i].output, data.Output.Name)
				i++
			}
		})
	}
}

func TestOneStoreOneMap(t *testing.T) {
	testStoreAddI64Hash := hex.EncodeToString([]byte("setup_test_store_add_i64"))
	assertTestStoreAddI64Hash := hex.EncodeToString([]byte("assert_test_store_add_i64"))

	defaultSPKG := "./testdata/simple_substreams/substreams-test-v0.1.0.spkg"
	zeroInitialBlockSPKG := "./testdata/simple_substreams_init0/substreams-test-init0-v0.1.0.spkg"

	tests := []struct {
		name                  string
		startBlock            int64
		linearBlock           uint64
		stopBlock             uint64
		firstStreamableBlock  uint64
		production            bool
		preWork               testPreWork
		expectedResponseCount int
		spkg                  string
		expectFiles           []string
		expectError           string
		expectedMetering      []string
		expectTier1Events     bool
		expectTier2Events     bool
	}{
		{
			name:                  "dev_mode_backprocess",
			spkg:                  defaultSPKG,
			startBlock:            25,
			linearBlock:           25,
			stopBlock:             29,
			production:            false,
			expectedResponseCount: 4,
			expectFiles: []string{
				testStoreAddI64Hash + "/outputs/0000000001-0000000010.output", // store outputs
				testStoreAddI64Hash + "/outputs/0000000010-0000000020.output",
				testStoreAddI64Hash + "/states/0000000010-0000000001.kv", // store states
				testStoreAddI64Hash + "/states/0000000020-0000000001.kv",
			},
			expectTier1Events: true,
			expectTier2Events: true,
		},
		{
			name:                  "dev_mode_backprocess_then_save_state",
			spkg:                  defaultSPKG,
			startBlock:            25,
			linearBlock:           25,
			stopBlock:             32,
			production:            false,
			expectedResponseCount: 7,
			expectFiles: []string{
				testStoreAddI64Hash + "/outputs/0000000001-0000000010.output", // store outputs
				testStoreAddI64Hash + "/outputs/0000000010-0000000020.output",
				testStoreAddI64Hash + "/states/0000000010-0000000001.kv", // store states
				testStoreAddI64Hash + "/states/0000000020-0000000001.kv",
			},
			expectTier1Events: true,
			expectTier2Events: true,
		},
		{
			name:                  "prod_mode_back_forward_to_lib",
			spkg:                  defaultSPKG,
			startBlock:            25,
			linearBlock:           20,
			stopBlock:             29,
			production:            true,
			expectedResponseCount: 4,
			expectFiles: []string{
				testStoreAddI64Hash + "/outputs/0000000001-0000000010.output",
				testStoreAddI64Hash + "/outputs/0000000010-0000000020.output",
				testStoreAddI64Hash + "/states/0000000010-0000000001.kv",
				testStoreAddI64Hash + "/states/0000000020-0000000001.kv",
			},
			expectTier1Events: true,
			expectTier2Events: true,
		},
		{
			name:                  "prod_mode_back_forward_to_stop",
			spkg:                  defaultSPKG,
			startBlock:            25,
			linearBlock:           30,
			stopBlock:             30,
			production:            true,
			expectedResponseCount: 5,
			expectFiles: []string{
				testStoreAddI64Hash + "/outputs/0000000001-0000000010.output", //store
				testStoreAddI64Hash + "/outputs/0000000010-0000000020.output",
				testStoreAddI64Hash + "/outputs/0000000020-0000000030.output",
				testStoreAddI64Hash + "/states/0000000010-0000000001.kv",
				testStoreAddI64Hash + "/states/0000000020-0000000001.kv",
				testStoreAddI64Hash + "/states/0000000030-0000000001.kv",
				assertTestStoreAddI64Hash + "/outputs/0000000020-0000000030.output", // map
			},
			expectTier1Events: true,
			expectTier2Events: true,
		},
		{
			name:                  "prod_mode_back_forward_to_stop_nonzero_first_streamable",
			spkg:                  zeroInitialBlockSPKG,
			firstStreamableBlock:  16,
			startBlock:            0,
			linearBlock:           30,
			stopBlock:             30,
			production:            true,
			expectedResponseCount: 14,
			expectFiles: []string{
				assertTestStoreAddI64Hash + "/outputs/0000000016-0000000020.output", // map
				assertTestStoreAddI64Hash + "/outputs/0000000020-0000000030.output", // map
				testStoreAddI64Hash + "/outputs/0000000016-0000000020.output",
				testStoreAddI64Hash + "/outputs/0000000020-0000000030.output",
				testStoreAddI64Hash + "/states/0000000020-0000000016.kv",
				testStoreAddI64Hash + "/states/0000000030-0000000016.kv",
			},
			expectTier1Events: true,
			expectTier2Events: true,
		},
		{
			name:                 "nonzero_first_streamable on nonzero module",
			spkg:                 defaultSPKG,
			firstStreamableBlock: 16,
			startBlock:           0,
			linearBlock:          30,
			stopBlock:            30,
			production:           true,
			expectError:          "running test: module graph: module \"setup_test_store_add_i64\" has initial block 1 smaller than first streamable block 16",
		},

		{
			name:                  "prod_mode_back_forward_to_stop_passed_boundary",
			spkg:                  defaultSPKG,
			startBlock:            25,
			linearBlock:           40,
			stopBlock:             41,
			production:            true,
			expectedResponseCount: 16,
			expectFiles: []string{
				testStoreAddI64Hash + "/outputs/0000000001-0000000010.output", // store
				testStoreAddI64Hash + "/outputs/0000000010-0000000020.output",
				testStoreAddI64Hash + "/outputs/0000000020-0000000030.output",
				testStoreAddI64Hash + "/outputs/0000000030-0000000040.output",
				testStoreAddI64Hash + "/states/0000000010-0000000001.kv",
				testStoreAddI64Hash + "/states/0000000020-0000000001.kv",
				testStoreAddI64Hash + "/states/0000000030-0000000001.kv",
				testStoreAddI64Hash + "/states/0000000040-0000000001.kv",
				assertTestStoreAddI64Hash + "/outputs/0000000020-0000000030.output", // map
				assertTestStoreAddI64Hash + "/outputs/0000000030-0000000040.output",
			},
			expectTier1Events: true,
			expectTier2Events: true,
		},
		{
			name:        "prod_mode_partial_existing",
			spkg:        defaultSPKG,
			startBlock:  1,
			linearBlock: 30,
			stopBlock:   30,
			production:  true,
			preWork: func(t *testing.T, run *testRun, workerFactory work.WorkerFactory) {
				partialPreWork(t, 1, 0, run, workerFactory)
			},
			expectedResponseCount: 29,
			expectFiles: []string{
				testStoreAddI64Hash + "/outputs/0000000001-0000000010.output",
				testStoreAddI64Hash + "/outputs/0000000010-0000000020.output",
				testStoreAddI64Hash + "/outputs/0000000020-0000000030.output",
				testStoreAddI64Hash + "/states/0000000010-0000000001.kv",
				testStoreAddI64Hash + "/states/0000000020-0000000001.kv",
				testStoreAddI64Hash + "/states/0000000030-0000000001.kv",
				assertTestStoreAddI64Hash + "/outputs/0000000001-0000000010.output",
				assertTestStoreAddI64Hash + "/outputs/0000000010-0000000020.output",
				assertTestStoreAddI64Hash + "/outputs/0000000020-0000000030.output",
			},
			expectedMetering: []string{
				"tier1:egress_bytes:5349.00",
				"tier1:file_compressed_read_bytes:0.00",
				"tier1:file_compressed_read_forked_bytes:0.00",
				"tier1:file_compressed_write_bytes:0.00",
				"tier1:file_uncompressed_read_bytes:0.00",
				"tier1:file_uncompressed_read_forked_bytes:0.00",
				"tier1:file_uncompressed_write_bytes:0.00",
				"tier1:live_uncompressed_read_bytes:0.00",
				"tier1:live_uncompressed_read_forked_bytes:0.00",
				"tier1:message_count:31.00",
				"tier1:read_bytes:0.00",
				"tier1:wasm_input_bytes:0.00",
				"tier1:written_bytes:0.00",
				"tier2:egress_bytes:937.00",
				"tier2:file_compressed_read_bytes:125.00",
				"tier2:file_compressed_read_forked_bytes:0.00",
				"tier2:file_compressed_write_bytes:75.00",
				"tier2:file_uncompressed_read_bytes:60.00",
				"tier2:file_uncompressed_read_forked_bytes:0.00",
				"tier2:file_uncompressed_write_bytes:36.00",
				"tier2:live_uncompressed_read_bytes:0.00",
				"tier2:live_uncompressed_read_forked_bytes:0.00",
				"tier2:message_count:24.00",
				"tier2:read_bytes:125.00",
				"tier2:wasm_input_bytes:4697.00",
				"tier2:written_bytes:36.00",
			},
			expectTier1Events: true, expectTier2Events: true,
		},
	}

	manifest.TestUseSimpleHash = true

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			bstream.GetProtocolFirstStreamableBlock = test.firstStreamableBlock // set for tier1 request to grab
			run := newTestRun(t, test.startBlock, test.linearBlock, test.stopBlock, test.firstStreamableBlock, "assert_test_store_add_i64", test.spkg)

			run.ProductionMode = test.production
			run.ParallelSubrequests = 1
			run.PreWork = test.preWork
			err := run.Run(t, test.name)
			if test.expectError != "" {
				assert.Error(t, err)
				assert.Equal(t, err.Error(), test.expectError)
				return
			}
			require.NoError(t, err)

			mapOutput := run.MapOutputString("assert_test_store_add_i64")
			assert.Contains(t, mapOutput, `assert_test_store_add_i64: 0801`)

			if test.expectedMetering != nil {
				meteringEvents := computeMeteringEvents(run.Events)
				assert.Contains(t, meteringEvents, test.expectedMetering)
			}

			assert.Equal(t, test.expectedResponseCount, strings.Count(mapOutput, "\n"))

			withZST := func(s []string) []string {
				res := make([]string, len(s))
				for i, v := range s {
					res[i] = fmt.Sprintf("%s.zst", v)
				}
				return res
			}

			assertFiles(t, run.TempDir, true, withZST(test.expectFiles)...)
		})
	}
}

func computeMeteringEvents(events []dmetering.Event) (out []string) {
	meteringSums := make(map[string]float64)
	for _, ev := range events {
		for metricName, value := range ev.Metrics {
			key := fmt.Sprintf("%s:%s", ev.Endpoint, metricName)
			meteringSums[key] += value
		}
	}

	for k, v := range meteringSums {
		out = append(out, fmt.Sprintf("%s:%0.2f", k, v))
	}
	slices.Sort(out)
	return
}

func files(t *testing.T, tempDir string) []string {
	var storedFiles []string
	require.NoError(t, filepath.Walk(tempDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		storedFiles = append(storedFiles, filepath.Base(path))
		return nil
	}))
	return storedFiles
}

func TestMultipleStoresDifferentStartBlocks(t *testing.T) {
	manifest.TestUseSimpleHash = true
	run := newTestRun(t, 0, 999, 20, 0, "assert_set_sum_store_0", "./testdata/complex_substreams/complex-substreams-v0.1.0.spkg")

	setSumStoreInit0 := hex.EncodeToString([]byte("set_sum_store_init_0"))
	run.ProductionMode = true
	require.NoError(t, run.Run(t, "pass_one"))
	assert.Equal(t, []string{"0000000010-0000000000.kv.zst", "0000000020-0000000000.kv.zst"},
		files(t, path.Join(run.TempDir, "test.store", "tag", setSumStoreInit0, "states")))

	// run2 will be run on the the same TempDir as the first run
	run2 := newTestRun(t, 40, 999, 50, 0, "multi_store_different_40", "./testdata/complex_substreams/complex-substreams-v0.1.0.spkg")
	run2.ProductionMode = true
	run2.JobCallback = func(unit stage.Unit) {
		if unit.Segment < 2 {
			t.Errorf("unexpected job callback for segment %d", unit.Segment)
		}
	}

	require.NoError(t, run2.RunWithTempDir(t, "pass two", run.TempDir))
}

func TestMultipleStoresUnalignedStartBlocksDevMode(t *testing.T) {
	manifest.TestUseSimpleHash = true
	// dev mode
	run := newTestRun(t, 23, 999, 30, 0, "multi_store_different_23", "./testdata/complex_substreams/complex-substreams-v0.1.0.spkg")
	require.NoError(t, run.Run(t, "dev_mode"))
	outs := run.MapOutput("multi_store_different_23")
	// ensure that the 20->22 block range is processed by the "set_sum_store_init_0" store
	assert.Contains(t, string(outs[23]), "store2:sum:276") // 1+2+3+4+5+6+7+8+9+10+11+12+13+14+15+16+17+18+19+20+21+22+23 = 276
	assert.Contains(t, string(outs[24]), "store2:sum:300") // 276+24 ...
}
func TestMultipleStoresUnalignedStartBlocksProdMode(t *testing.T) {
	manifest.TestUseSimpleHash = true
	run := newTestRun(t, 23, 999, 30, 0, "multi_store_different_23", "./testdata/complex_substreams/complex-substreams-v0.1.0.spkg")
	run.ProductionMode = true
	require.NoError(t, run.Run(t, "prod_mode"))

	outs := run.MapOutput("multi_store_different_23")
	// ensure that the 20->22 block range is processed by the "set_sum_store_init_0" store
	assert.Contains(t, string(outs[23]), "store2:sum:276") // 1+2+3+4+5+6+7+8+9+10+11+12+13+14+15+16+17+18+19+20+21+22+23 = 276
	assert.Contains(t, string(outs[24]), "store2:sum:300") // 276+24 ...
}

func TestStoreDeletePrefix(t *testing.T) {
	run := newTestRun(t, 30, 40, 42, 0, "assert_test_store_delete_prefix", "./testdata/simple_substreams/substreams-test-v0.1.0.spkg")
	run.BlockProcessedCallback = func(ctx *execContext) {
		if ctx.block.Number == 40 {
			s, storeFound := ctx.stores.Get("test_store_delete_prefix")
			require.True(t, storeFound)
			require.Equal(t, uint64(1), s.Length())
		}
	}

	require.NoError(t, run.Run(t, "test_store_delete_prefix"))
}

func TestAllAssertions(t *testing.T) {
	// Relies on `assert_all_test` having modInit == 1, so
	run := newTestRun(t, 1, 31, 31, 0, "assert_all_test", "./testdata/simple_substreams/substreams-test-v0.1.0.spkg")

	require.NoError(t, run.Run(t, "assert_all_test"))

	//assert.Len(t, listFiles(t, run.TempDir), 90) // All these .kv files on disk
	// TODO: we don't produce those files when in linear mode..
	// because it produced inconsistent snapshots..
}

func Test_SimpleMapModule(t *testing.T) {
	run := newTestRun(t, 10000, 10001, 10001, 0, "test_map", "./testdata/simple_substreams/substreams-test-v0.1.0.spkg")
	run.Params = map[string]string{"test_map": "my test params"}
	run.NewBlockGenerator = func(startBlock uint64, inclusiveStopBlock uint64) TestBlockGenerator {
		return &LinearBlockGenerator{
			startBlock:         startBlock,
			inclusiveStopBlock: inclusiveStopBlock + 10,
		}
	}
	run.ParallelSubrequests = 5
	run.Context = cancelledContext(100 * time.Millisecond)

	require.NoError(t, run.Run(t, "test_map"))
}

func Test_WASMBindgenShims(t *testing.T) {
	run := newTestRun(t, 12, 14, 14, 0, "map_block", "./testdata/wasmbindgen_substreams/wasmbindgen-substreams-v0.1.0.spkg")
	run.NewBlockGenerator = func(startBlock uint64, inclusiveStopBlock uint64) TestBlockGenerator {
		return &LinearBlockGenerator{
			startBlock:         startBlock,
			inclusiveStopBlock: inclusiveStopBlock + 10,
		}
	}
	run.ParallelSubrequests = 1

	require.NoError(t, run.Run(t, "test_wasmbindgenshims"))

	mapOutput := run.MapOutputString("map_block")
	fmt.Println(mapOutput)

}

func Test_Early(t *testing.T) {
	run := newTestRun(t, 12, 14, 14, 0, "test_map", "./testdata/simple_substreams/substreams-test-v0.1.0.spkg")
	run.Params = map[string]string{"test_map": "my test params"}
	run.ProductionMode = true
	run.NewBlockGenerator = func(startBlock uint64, inclusiveStopBlock uint64) TestBlockGenerator {
		return &LinearBlockGenerator{
			startBlock:         startBlock,
			inclusiveStopBlock: inclusiveStopBlock + 10,
		}
	}
	run.ParallelSubrequests = 1

	require.NoError(t, run.Run(t, "test_map"))
}

func TestEarlyWithEmptyStore(t *testing.T) {
	run := newTestRun(t, 2, 4, 4, 0, "assert_test_store_delete_prefix", "./testdata/simple_substreams/substreams-test-v0.1.0.spkg")
	run.ProductionMode = true

	var foundBlock3 bool
	run.BlockProcessedCallback = func(ctx *execContext) {
		if ctx.block.Number == 3 {
			foundBlock3 = true
		}
	}
	run.Context = cancelledContext(2000 * time.Millisecond)

	require.NoError(t, run.Run(t, "assert_test_store_delete_prefix"))
	require.True(t, foundBlock3)
}

func Test_SingleMapModule_FileWalker(t *testing.T) {
	run := newTestRun(t, 200, 250, 300, 0, "test_map", "./testdata/simple_substreams/substreams-test-v0.1.0.spkg")
	run.Params = map[string]string{"test_map": "my test params"}
	run.ProductionMode = true
	run.NewBlockGenerator = func(startBlock uint64, inclusiveStopBlock uint64) TestBlockGenerator {
		return &LinearBlockGenerator{
			startBlock:         startBlock,
			inclusiveStopBlock: inclusiveStopBlock + 10,
		}
	}
	run.ParallelSubrequests = 5
	run.Context = cancelledContext(2000 * time.Millisecond)

	// TODO: make sure we're exercising the FileWalker and going through the Scheduler with _no Stores_ to process.
	// make sure we have those NoOp fields on the stores we don't need to process.

	require.NoError(t, run.Run(t, "test_map"))
}

func cancelledContext(delay time.Duration) context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(delay)
		cancel()
	}()
	return ctx
}

func listFiles(t *testing.T, tempDir string) []string {
	var storedFiles []string
	require.NoError(t, filepath.Walk(tempDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		storedFiles = append(storedFiles, strings.TrimPrefix(path, tempDir))
		return nil
	}))
	return storedFiles
}

func assertFiles(t *testing.T, tempDir string, expectPartialSpkg bool, wantedFiles ...string) {
	producedFiles := listFiles(t, tempDir)

	actualFiles := make([]string, 0, len(producedFiles))
	var seenPartialSpkg bool
	for _, f := range producedFiles {
		parts := strings.Split(f, string(os.PathSeparator))
		if parts[len(parts)-1] == "substreams.partial.spkg.zst" {
			seenPartialSpkg = true
			continue
		}
		actualFiles = append(actualFiles, filepath.Join(parts[3:]...))
	}

	if expectPartialSpkg {
		assert.True(t, seenPartialSpkg, "substreams.partial.spkg should be produced")
	}

	assert.ElementsMatch(t, wantedFiles, actualFiles)
}

func partialPreWork(t *testing.T, start uint64, stageIdx int, run *testRun, workerFactory work.WorkerFactory) {
	worker := workerFactory(zlog)

	// FIXME: use the new `Work` interface here, and validate that the
	// caller to `partialPreWork` doesn't need to be changed too much? :)
	segmenter := block.NewSegmenter(10, 0, 0)
	unit := stage.Unit{Segment: segmenter.IndexForStartBlock(start), Stage: stageIdx}
	ctx := reqctx.WithRequest(run.Context, &reqctx.RequestDetails{Modules: run.Package.Modules, OutputModule: run.ModuleName})
	cmd := worker.Work(ctx, unit, start, []string{run.ModuleName}, nil)
	result := cmd()
	msg, ok := result.(work.MsgJobSucceeded)
	require.True(t, ok)
	assert.Equal(t, msg.Unit, unit)
	//require.Equal(t, store.PartialFiles(fmt.Sprintf("%d-%d", start, end), store.TraceIDParam(traceID)), result.PartialFilesWritten)
}

package integration

import (
	"context"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/streamingfast/bstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/streamingfast/substreams/block"
	"github.com/streamingfast/substreams/orchestrator/stage"
	"github.com/streamingfast/substreams/orchestrator/work"
	"github.com/streamingfast/substreams/reqctx"

	//_ "github.com/streamingfast/substreams/wasm/wasmtime"
	_ "github.com/streamingfast/substreams/wasm/wazero"
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
				{blockRef: bstream.NewBlockRef("7a", 6), previousID: "6a", libBlockRef: bstream.NewBlockRef("4a", 4)},
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
			run := newTestRun(t, 1, 1, 7, test.module)
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
	tests := []struct {
		name                  string
		startBlock            int64
		linearBlock           uint64
		stopBlock             uint64
		production            bool
		preWork               testPreWork
		expectedResponseCount int
		expectFiles           []string
	}{
		{
			name:                  "dev_mode_backprocess",
			startBlock:            25,
			linearBlock:           25,
			stopBlock:             29,
			production:            false,
			expectedResponseCount: 4,
			expectFiles: []string{

				"ebd5bb65aaf4471e468efea126f27dbddb37b59e/outputs/0000000001-0000000010.output", // store outputs
				"ebd5bb65aaf4471e468efea126f27dbddb37b59e/outputs/0000000010-0000000020.output",
				"ebd5bb65aaf4471e468efea126f27dbddb37b59e/states/0000000010-0000000001.kv", // store states
				"ebd5bb65aaf4471e468efea126f27dbddb37b59e/states/0000000020-0000000001.kv",
				//				"states/0000000025-0000000020.partial", // produced, then deleted
			},
		},
		{
			name:                  "dev_mode_backprocess_then_save_state",
			startBlock:            25,
			linearBlock:           25,
			stopBlock:             32,
			production:            false,
			expectedResponseCount: 7,
			expectFiles: []string{
				"ebd5bb65aaf4471e468efea126f27dbddb37b59e/outputs/0000000001-0000000010.output", // store outputs
				"ebd5bb65aaf4471e468efea126f27dbddb37b59e/outputs/0000000010-0000000020.output",
				"ebd5bb65aaf4471e468efea126f27dbddb37b59e/states/0000000010-0000000001.kv", // store states
				"ebd5bb65aaf4471e468efea126f27dbddb37b59e/states/0000000020-0000000001.kv",
				// "states/0000000025-0000000020.partial", // produced, then deleted
				//"states/0000000030-0000000001.kv", // Again, backprocess wouldn't save this one, nor does it need to.
			},
		},
		{
			name:                  "prod_mode_back_forward_to_lib",
			startBlock:            25,
			linearBlock:           27,
			stopBlock:             29,
			production:            true,
			expectedResponseCount: 4,
			expectFiles: []string{
				"3574de26d590713344b911bbc1c3bf3305ccb906/outputs/0000000020-0000000027.output",
				"ebd5bb65aaf4471e468efea126f27dbddb37b59e/outputs/0000000001-0000000010.output",
				"ebd5bb65aaf4471e468efea126f27dbddb37b59e/outputs/0000000010-0000000020.output",
				"ebd5bb65aaf4471e468efea126f27dbddb37b59e/states/0000000010-0000000001.kv",
				"ebd5bb65aaf4471e468efea126f27dbddb37b59e/states/0000000020-0000000001.kv",
			},
		},
		{
			name:                  "prod_mode_back_forward_to_stop",
			startBlock:            25,
			linearBlock:           29,
			stopBlock:             29,
			production:            true,
			expectedResponseCount: 4,
			expectFiles: []string{
				"ebd5bb65aaf4471e468efea126f27dbddb37b59e/outputs/0000000001-0000000010.output", //store
				"ebd5bb65aaf4471e468efea126f27dbddb37b59e/outputs/0000000010-0000000020.output",
				"ebd5bb65aaf4471e468efea126f27dbddb37b59e/states/0000000010-0000000001.kv",
				"ebd5bb65aaf4471e468efea126f27dbddb37b59e/states/0000000020-0000000001.kv",
				"3574de26d590713344b911bbc1c3bf3305ccb906/outputs/0000000020-0000000029.output", // map
			},
		},
		{
			name:                  "prod_mode_back_forward_to_stop_passed_boundary",
			startBlock:            25,
			linearBlock:           38,
			stopBlock:             38,
			production:            true,
			expectedResponseCount: 13,
			expectFiles: []string{
				"ebd5bb65aaf4471e468efea126f27dbddb37b59e/outputs/0000000001-0000000010.output", // store
				"ebd5bb65aaf4471e468efea126f27dbddb37b59e/outputs/0000000010-0000000020.output",
				"ebd5bb65aaf4471e468efea126f27dbddb37b59e/outputs/0000000020-0000000030.output",
				"ebd5bb65aaf4471e468efea126f27dbddb37b59e/states/0000000010-0000000001.kv",
				"ebd5bb65aaf4471e468efea126f27dbddb37b59e/states/0000000020-0000000001.kv",
				"ebd5bb65aaf4471e468efea126f27dbddb37b59e/states/0000000030-0000000001.kv",
				"3574de26d590713344b911bbc1c3bf3305ccb906/outputs/0000000020-0000000030.output", // map
				"3574de26d590713344b911bbc1c3bf3305ccb906/outputs/0000000030-0000000038.output",
			},
		},
		{
			name:                  "prod_mode_start_before_linear_and_firstboundary",
			startBlock:            7,
			linearBlock:           8,
			stopBlock:             9,
			production:            true,
			expectedResponseCount: 2,
			expectFiles: []string{
				"3574de26d590713344b911bbc1c3bf3305ccb906/outputs/0000000001-0000000008.output",
			},
		},
		{
			name:                  "prod_mode_start_before_linear_then_pass_firstboundary",
			startBlock:            7,
			linearBlock:           8,
			stopBlock:             15,
			production:            true,
			expectedResponseCount: 8,
			expectFiles: []string{
				//"states/0000000010-0000000001.kv", // TODO: not sure why this would have been produced with the prior code..
				"3574de26d590713344b911bbc1c3bf3305ccb906/outputs/0000000001-0000000008.output",
			},
		},
		{
			name:        "prod_mode_partial_existing",
			startBlock:  1,
			linearBlock: 29,
			stopBlock:   29,
			production:  true,
			preWork: func(t *testing.T, run *testRun, workerFactory work.WorkerFactory) {
				partialPreWork(t, 1, 10, 0, run, workerFactory)
			},
			expectedResponseCount: 28,
			expectFiles: []string{
				"ebd5bb65aaf4471e468efea126f27dbddb37b59e/outputs/0000000001-0000000010.output",
				"ebd5bb65aaf4471e468efea126f27dbddb37b59e/outputs/0000000010-0000000020.output",
				"ebd5bb65aaf4471e468efea126f27dbddb37b59e/states/0000000010-0000000001.kv",
				"ebd5bb65aaf4471e468efea126f27dbddb37b59e/states/0000000020-0000000001.kv",
				"3574de26d590713344b911bbc1c3bf3305ccb906/outputs/0000000001-0000000010.output",
				"3574de26d590713344b911bbc1c3bf3305ccb906/outputs/0000000010-0000000020.output",
				"3574de26d590713344b911bbc1c3bf3305ccb906/outputs/0000000020-0000000029.output",
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			run := newTestRun(t, test.startBlock, test.linearBlock, test.stopBlock, "assert_test_store_add_i64")
			run.ProductionMode = test.production
			run.ParallelSubrequests = 1
			run.PreWork = test.preWork
			require.NoError(t, run.Run(t, test.name))

			mapOutput := run.MapOutput("assert_test_store_add_i64")
			assert.Contains(t, mapOutput, `assert_test_store_add_i64: 0801`)

			fmt.Println(mapOutput)
			assert.Equal(t, test.expectedResponseCount, strings.Count(mapOutput, "\n"))

			withZST := func(s []string) []string {
				res := make([]string, len(s), len(s))
				for i, v := range s {
					res[i] = fmt.Sprintf("%s.zst", v)
				}
				return res
			}

			assertFiles(t, run.TempDir, withZST(test.expectFiles)...)
		})
	}
}

func TestStoreDeletePrefix(t *testing.T) {
	run := newTestRun(t, 30, 41, 41, "assert_test_store_delete_prefix")
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
	run := newTestRun(t, 1, 31, 31, "assert_all_test")

	require.NoError(t, run.Run(t, "assert_all_test"))

	//assert.Len(t, listFiles(t, run.TempDir), 90) // All these .kv files on disk
	// TODO: we don't produce those files when in linear mode..
	// because it produced inconsistent snapshots..
}

func Test_SimpleMapModule(t *testing.T) {
	run := newTestRun(t, 10000, 10001, 10001, "test_map")
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

func Test_Early(t *testing.T) {
	run := newTestRun(t, 12, 14, 14, "test_map")
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
	run := newTestRun(t, 2, 4, 4, "assert_test_store_delete_prefix")
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
	run := newTestRun(t, 200, 250, 300, "test_map")
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

func assertFiles(t *testing.T, tempDir string, wantedFiles ...string) {
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

	assert.True(t, seenPartialSpkg, "substreams.partial.spkg should be produced")
	assert.ElementsMatch(t, wantedFiles, actualFiles)
}

func partialPreWork(t *testing.T, start, end uint64, stageIdx int, run *testRun, workerFactory work.WorkerFactory) {
	worker := workerFactory(zlog)

	// FIXME: use the new `Work` interface here, and validate that the
	// caller to `partialPreWork` doesn't need to be changed too much? :)
	segmenter := block.NewSegmenter(10, 0, 0)
	unit := stage.Unit{Segment: segmenter.IndexForStartBlock(start), Stage: stageIdx}
	ctx := reqctx.WithRequest(run.Context, &reqctx.RequestDetails{Modules: run.Package.Modules, OutputModule: run.ModuleName, CacheTag: "tag"})
	cmd := worker.Work(ctx, unit, block.NewRange(start, end), []string{run.ModuleName}, nil)
	result := cmd()
	msg, ok := result.(work.MsgJobSucceeded)
	require.True(t, ok)
	assert.Equal(t, msg.Unit, unit)
	//require.Equal(t, store.PartialFiles(fmt.Sprintf("%d-%d", start, end), store.TraceIDParam(traceID)), result.PartialFilesWritten)
}

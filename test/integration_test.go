package integration

import (
	"context"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/streamingfast/bstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestForkHandling(t *testing.T) {
	type response struct {
		id      string
		outputs []string
		undo    bool
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
				{id: "1a", outputs: []string{"assert_test_store_add_bigint"}},
				{id: "2a", outputs: []string{"assert_test_store_add_bigint"}},
				{id: "3a", outputs: []string{"assert_test_store_add_bigint"}},
				{id: "4a", outputs: []string{"assert_test_store_add_bigint"}},
				{id: "4a", undo: true, outputs: []string{"assert_test_store_add_bigint"}},
				{id: "3a", undo: true, outputs: []string{"assert_test_store_add_bigint"}},
				{id: "2a", undo: true, outputs: []string{"assert_test_store_add_bigint"}},
				{id: "2b", outputs: []string{"assert_test_store_add_bigint"}},
				{id: "3b", outputs: []string{"assert_test_store_add_bigint"}},
				{id: "4b", outputs: []string{"assert_test_store_add_bigint"}},
				{id: "5b", outputs: []string{"assert_test_store_add_bigint"}},
				{id: "5b", undo: true, outputs: []string{"assert_test_store_add_bigint"}},
				{id: "4b", undo: true, outputs: []string{"assert_test_store_add_bigint"}},
				{id: "3b", undo: true, outputs: []string{"assert_test_store_add_bigint"}},
				{id: "2b", undo: true, outputs: []string{"assert_test_store_add_bigint"}},
				{id: "2a", outputs: []string{"assert_test_store_add_bigint"}},
				{id: "3a", outputs: []string{"assert_test_store_add_bigint"}},
				{id: "4a", outputs: []string{"assert_test_store_add_bigint"}},
				{id: "5a", outputs: []string{"assert_test_store_add_bigint"}},
				{id: "6a", outputs: []string{"assert_test_store_add_bigint"}},
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
				{id: "1a", outputs: []string{"setup_test_store_add_bigint", "assert_test_store_add_bigint"}},
				{id: "2b", outputs: []string{"setup_test_store_add_bigint", "assert_test_store_add_bigint"}},
				{id: "2b", undo: true, outputs: []string{"setup_test_store_add_bigint", "assert_test_store_add_bigint"}},
				{id: "2a", outputs: []string{"setup_test_store_add_bigint", "assert_test_store_add_bigint"}},
				{id: "3a", outputs: []string{"setup_test_store_add_bigint", "assert_test_store_add_bigint"}},
				{id: "4a", outputs: []string{"setup_test_store_add_bigint", "assert_test_store_add_bigint"}},
			},
		},
	}

	for _, test := range tests {

		t.Run(test.name, func(t *testing.T) {
			run := newTestRun(1, 1, 7, test.module)
			run.NewBlockGenerator = func(startBlock uint64, inclusiveStopBlock uint64) TestBlockGenerator {
				return &ForkBlockGenerator{
					initialLIB:    bstream.NewBlockRef("0a", 0),
					forkBlockRefs: test.forkBlockRefs,
				}
			}

			run.ModuleName = test.module
			run.ProductionMode = test.production
			run.BlockProcessedCallback = test.inProcessValidation
			err := run.Run(t)

			require.NoError(t, err)
			i := 0
			for _, resp := range run.Responses {
				if resp.GetProgress() != nil {
					continue
				}
				data := resp.GetData()
				require.Greater(t, len(test.expectedResponseNames), i, "too many responses")
				assert.Equal(t, test.expectedResponseNames[i].id, data.Clock.Id)

				var seenOutputModules []string
				for _, out := range data.Outputs {
					seenOutputModules = append(seenOutputModules, out.Name)
				}
				assert.Equal(t, test.expectedResponseNames[i].outputs, seenOutputModules)
				i++
			}
		})
	}
}

func TestOneStoreOneMap(t *testing.T) {
	tests := []struct {
		name        string
		startBlock  int64
		linearBlock uint64
		stopBlock   uint64
		production  bool
		expectCount int
		expectFiles []string
	}{
		{
			name:        "dev_mode_backprocess",
			startBlock:  25,
			linearBlock: 25,
			stopBlock:   29,
			production:  false,
			expectCount: 4,
			expectFiles: []string{
				"states/0000000010-0000000001.kv",
				"states/0000000020-0000000001.kv",
			},
		},
		{
			name:        "dev_mode_backprocess_then_save_state",
			startBlock:  25,
			linearBlock: 25,
			stopBlock:   32,
			production:  false,
			expectCount: 7,
			expectFiles: []string{
				"states/0000000010-0000000001.kv",
				"states/0000000020-0000000001.kv",
				"states/0000000030-0000000001.kv",
			},
		},
		{
			name:        "prod_mode_back_forward_to_lib",
			startBlock:  25,
			linearBlock: 27,
			stopBlock:   29,
			production:  true,
			expectCount: 4,
			expectFiles: []string{
				"states/0000000010-0000000001.kv",
				"states/0000000020-0000000001.kv",
				"outputs/0000000020-0000000027.output",
			},
		},
		{
			name:        "prod_mode_back_forward_to_stop",
			startBlock:  25,
			linearBlock: 29,
			stopBlock:   29,
			production:  true,
			expectCount: 4,
			expectFiles: []string{
				"states/0000000010-0000000001.kv",
				"states/0000000020-0000000001.kv",
				"outputs/0000000020-0000000029.output",
			},
		},
		{
			name:        "prod_mode_back_forward_to_stop_passed_boundary",
			startBlock:  25,
			linearBlock: 38,
			stopBlock:   38,
			production:  true,
			expectCount: 13,
			expectFiles: []string{
				"states/0000000010-0000000001.kv",
				"states/0000000020-0000000001.kv",
				"states/0000000030-0000000001.kv",
				"outputs/0000000020-0000000030.output",
				"outputs/0000000030-0000000038.output",
			},
		},
		{
			name:        "prod_mode_start_before_linear_and_firstboundary",
			startBlock:  7,
			linearBlock: 8,
			stopBlock:   9,
			production:  true,
			expectCount: 2,
			expectFiles: []string{
				"outputs/0000000001-0000000008.output",
			},
		},
		{
			name:        "prod_mode_start_before_linear_then_pass_firstboundary",
			startBlock:  7,
			linearBlock: 8,
			stopBlock:   15,
			production:  true,
			expectCount: 8,
			expectFiles: []string{
				"states/0000000010-0000000001.kv",
				"outputs/0000000001-0000000008.output",
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {

			run := newTestRun(test.startBlock, test.linearBlock, test.stopBlock, "assert_test_store_add_i64")
			run.ProductionMode = test.production
			run.ParallelSubrequests = 5
			require.NoError(t, run.Run(t))

			mapOutput := run.MapOutput("assert_test_store_add_i64")
			assert.Contains(t, mapOutput, `assert_test_store_add_i64: 0801`)

			assert.Equal(t, test.expectCount, strings.Count(mapOutput, "\n"))
			assertFiles(t, run.TempDir, test.expectFiles...)
		})
	}
}

func TestStoreDeletePrefix(t *testing.T) {
	run := newTestRun(30, 41, 41, "assert_test_store_delete_prefix")
	run.BlockProcessedCallback = func(ctx *execContext) {
		if ctx.block.Number == 40 {
			s, storeFound := ctx.stores.Get("test_store_delete_prefix")
			require.True(t, storeFound)
			require.Equal(t, uint64(1), s.Length())
		}
	}

	require.NoError(t, run.Run(t))
}

func TestAllAssertions(t *testing.T) {
	// Relies on `assert_all_test` having modInit == 1, so
	run := newTestRun(1, 31, 31, "assert_all_test")

	require.NoError(t, run.Run(t))

	assert.Len(t, listFiles(t, run.TempDir), 90) // All these .kv files on disk
}

func Test_SimpleMapModule(t *testing.T) {
	run := newTestRun(10000, 10001, 10001, "test_map")
	run.Params = map[string]string{"test_map": "my test params"}
	run.NewBlockGenerator = func(startBlock uint64, inclusiveStopBlock uint64) TestBlockGenerator {
		return &LinearBlockGenerator{
			startBlock:         startBlock,
			inclusiveStopBlock: inclusiveStopBlock + 10,
		}
	}
	run.ParallelSubrequests = 5
	run.Context = cancelledContext(100 * time.Millisecond)

	require.NoError(t, run.Run(t))
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
	//fmt.Println("STORED FILES", storedFiles)
	return storedFiles
}

func assertFiles(t *testing.T, tempDir string, wantedFiles ...string) {
	storedFiles := listFiles(t, tempDir)
	assert.Len(t, storedFiles, len(wantedFiles))
	filenames := strings.Join(storedFiles, "\n")
	for _, re := range wantedFiles {
		assert.Regexp(t, re, filenames)
	}
}

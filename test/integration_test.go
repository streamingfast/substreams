package integration

import (
	"context"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/streamingfast/bstream"
	"github.com/stretchr/testify/require"
)

//todo:
// 1. add test and new block generator (string and parse easier) that generates
//		different sequence of blocks and test with a store add the value in the store
// 	- 10a, 11a, 12b, 12a, 13a
//   new10a, new11a, new12b, undo12b, new12a, new13a (with some irreversible steps maybe...)
// 2. also expected field validation for the cursor and the step type

func TestForkSituation(t *testing.T) { // todo: change test name
	run := newTestRun(1, 1, 7, "assert_test_store_add_bigint")
	run.NewBlockGenerator = func(startBlock uint64, inclusiveStopBlock uint64) TestBlockGenerator {
		return &ForkBlockGenerator{
			initialLIB: bstream.NewBlockRef("0a", 0),
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
		}
	}
	run.BlockProcessedCallback = func(ctx *execContext) {
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
	}

	require.NoError(t, run.Run(t))
}

func TestProductionMode(t *testing.T) {
	run := newTestRun(20, 28, 31, "assert_test_store_add_i64")
	run.ProductionMode = true
	run.ParallelSubrequests = 5
	require.NoError(t, run.Run(t))

	require.Equal(t, "bob", run.MapOutput("assert_test_store_add_i64"))
}

func Test_MultipleModule_Batch_Output_Written(t *testing.T) {
	run := newTestRun(110, 112, 112, "test_map", "test_store_proto")
	outputFilesLen := 0
	run.BlockProcessedCallback = func(ctx *execContext) {
		err := ctx.baseStore.Walk(context.Background(), "", func(filename string) (err error) {
			if strings.Contains(filename, "output") {
				outputFilesLen++
			}
			return nil
		})
		require.NoError(t, err)
	}

	require.NoError(t, run.Run(t))

	require.NotZero(t, run.MapOutput("test_map"))
	require.NotZero(t, outputFilesLen)
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
	run := newTestRun(20, 31, 31, "assert_all_test")
	require.NoError(t, run.Run(t))
}

func Test_SimpleMapModule(t *testing.T) {
	t.Skip("Skipping until we can figure out why this is failing")

	run := newTestRun(10000, 10001, 10001, "test_store_proto")
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

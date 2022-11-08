package integration

import (
	"context"
	"github.com/streamingfast/bstream"
	"github.com/streamingfast/dstore"
	"github.com/streamingfast/substreams/pipeline"
	"github.com/streamingfast/substreams/store"
	"github.com/stretchr/testify/require"
	"math/big"
	"strings"
	"testing"
)

//todo:
// 1. add test and new block generator (string and parse easier) that generates
//		different sequence of blocks and test with a store add the value in the store
// 	- 10a, 11a, 12b, 12a, 13a
//   new10a, new11a, new12b, undo12b, new12a, new13a (with some irreversible steps maybe...)
// 2. also expected field validation for the cursor and the step type

func TestForkSituation(t *testing.T) { // todo: change test name
	run := newTestRun(1, 7, "assert_test_store_add_bigint")
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
			},
		}
	}
	run.BlockProcessedCallback = func(p *pipeline.Pipeline, b *bstream.Block, stores store.Map, baseStore dstore.Store) {
		if b.Number == 6 {
			s, found := stores.Get("setup_test_store_add_bigint")
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

func Test_MultipleModule_Batch_Output_Written(t *testing.T) {
	run := newTestRun(110, 112, "test_map", "test_store_proto")
	outputFilesLen := 0
	run.BlockProcessedCallback = func(p *pipeline.Pipeline, b *bstream.Block, stores store.Map, baseStore dstore.Store) {
		err := baseStore.Walk(context.Background(), "", func(filename string) (err error) {
			if strings.Contains(filename, "output") {
				outputFilesLen++
			}
			return nil
		})
		require.NoError(t, err)
	}

	require.NoError(t, run.Run(t))

	require.NotZero(t, run.ModuleOutputs(t))
	require.NotZero(t, outputFilesLen)
}

func TestStoreDeletePrefix(t *testing.T) {
	run := newTestRun(30, 41, "assert_test_store_delete_prefix")
	run.BlockProcessedCallback = func(p *pipeline.Pipeline, b *bstream.Block, stores store.Map, baseStore dstore.Store) {
		if b.Number == 40 {
			s, storeFound := stores.Get("test_store_delete_prefix")
			require.True(t, storeFound)
			require.Equal(t, uint64(1), s.Length())
		}
	}
	require.NoError(t, run.Run(t))
}

func TestAllAssertions(t *testing.T) {
	// Relies on `assert_all_test` having modInit == 1, so
	run := newTestRun(20, 31, "assert_all_test")
	require.NoError(t, run.Run(t))
}

func TestAllAssertionsParallel(t *testing.T) {
	run := newTestRun(20, 31, "assert_all_test")
	run.ParallelSubrequests = 5
	require.NoError(t, run.Run(t))
}

package integration

import (
	"context"
	"github.com/streamingfast/bstream"
	"github.com/streamingfast/dstore"
	"github.com/streamingfast/substreams/pipeline"
	"github.com/streamingfast/substreams/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"math/big"
	"strings"
	"testing"
)

func Test_SimpleMapModule(t *testing.T) {
	test := newTestRun(10, 12, "test_map")

	require.NoError(t, test.Run(t))

	require.Equal(t, `
10: test_map: {"name":"test_map","result":{"block_number":10,"block_hash":"block-10"}}
11: test_map: {"name":"test_map","result":{"block_number":11,"block_hash":"block-11"}}`,
		test.MapOutput("test_map"))
}

//todo:
// 1. add test and new block generator (string and parse easier) that generates
//		different sequence of blocks and test with a store add the value in the store
// 	- 10a, 11a, 12b, 12a, 13a
//   new10a, new11a, new12b, undo12b, new12a, new13a (with some irreversible steps maybe...)
// 2. also expected field validation for the cursor and the step type

func Test_AddBigIntWithCursorGeneratorStepNew(t *testing.T) { // todo: change test name
	run := newTestRun(1, 7, "test_store_add_bigint", "assert_test_store_add_bigint")
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
			s, found := stores.Get("test_store_add_bigint")
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

func Test_test_store_proto(t *testing.T) {
	run := newTestRun(10, 12, "test_store_proto")

	require.NoError(t, run.Run(t))

	require.Equal(t, []string{
		`{"name":"test_store_proto","deltas":[{"op":"CREATE","old":{},"new":{"block_number":10,"block_hash":"block-10"}}]}`,
		`{"name":"test_store_proto","deltas":[{"op":"CREATE","old":{},"new":{"block_number":11,"block_hash":"block-11"}}]}`,
	}, run.ModuleOutputs(t))
}

func Test_MultipleModule(t *testing.T) {
	run := newTestRun(10, 12, "test_map", "test_store_add_int64", "test_store_proto")

	require.NoError(t, run.Run(t))

	require.Equal(t, []string{
		`{"name":"test_map","result":{"block_number":10,"block_hash":"block-10"}}`,
		`{"name":"test_store_add_int64","deltas":[{"op":"UPDATE","old":"9","new":"10"}]}`,
		`{"name":"test_store_proto","deltas":[{"op":"CREATE","old":{},"new":{"block_number":10,"block_hash":"block-10"}}]}`,
		`{"name":"test_map","result":{"block_number":11,"block_hash":"block-11"}}`,
		`{"name":"test_store_add_int64","deltas":[{"op":"UPDATE","old":"10","new":"11"}]}`,
		`{"name":"test_store_proto","deltas":[{"op":"CREATE","old":{},"new":{"block_number":11,"block_hash":"block-11"}}]}`,
	}, run.ModuleOutputs(t))
}

func Test_MultipleModule_Batch(t *testing.T) {
	run := newTestRun(1000, 1021, "test_store_add_bigint", "assert_test_store_add_bigint")

	require.NoError(t, run.Run(t))
	//todo: Need to validate the storage file
}

func Test_MultipleModule_Batch_2(t *testing.T) {
	newBlockGenerator := func(startBlock uint64, inclusiveStopBlock uint64) TestBlockGenerator {
		return &LinearBlockGenerator{
			startBlock:         startBlock,
			inclusiveStopBlock: inclusiveStopBlock,
		}
	}

	moduleOutputs, err := runTest(t, nil, 110, 112, []string{"test_map", "test_store_proto"}, 10, 1, newBlockGenerator, nil)
	require.NoError(t, err)

	require.Equal(t, []string{
		`{"name":"test_map","result":{"block_number":110,"block_hash":"block-110"}}`,
		`{"name":"test_store_proto","deltas":[{"op":"CREATE","old":{},"new":{"block_number":110,"block_hash":"block-110"}}]}`,
		`{"name":"test_map","result":{"block_number":111,"block_hash":"block-111"}}`,
		`{"name":"test_store_proto","deltas":[{"op":"CREATE","old":{},"new":{"block_number":111,"block_hash":"block-111"}}]}`,
	}, moduleOutputs)
}

func Test_MultipleModule_Batch_Output_Written(t *testing.T) {
	newBlockGenerator := func(startBlock uint64, inclusiveStopBlock uint64) TestBlockGenerator {
		return &LinearBlockGenerator{
			startBlock:         startBlock,
			inclusiveStopBlock: inclusiveStopBlock,
		}
	}

	outputFilesLen := 0
	moduleOutputs, err := runTest(t, nil, 110, 112, []string{"test_map", "test_store_proto"}, 10, 1,
		newBlockGenerator,
		func(p *pipeline.Pipeline, b *bstream.Block, stores store.Map, baseStore dstore.Store) {
			err := baseStore.Walk(context.Background(), "", func(filename string) (err error) {
				if strings.Contains(filename, "output") {
					outputFilesLen++
				}
				return nil
			})
			require.NoError(t, err)
		},
	)
	require.NoError(t, err)

	require.NotZero(t, moduleOutputs)
	require.NotZero(t, outputFilesLen)
}

//	func Test_test_store_add_bigint(t *testing.T) {
//		newBlockGenerator := func(startBlock uint64, inclusiveStopBlock uint64) TestBlockGenerator {
//			return &LinearBlockGenerator{
//				startBlock:         startBlock,
//				inclusiveStopBlock: inclusiveStopBlock,
//			}
//		}
//
//		_, err := runTest(t, 1, 1001, []string{"test_store_add_bigint", "assert_test_store_add_bigint"}, newBlockGenerator, nil)
//		require.NoError(t, err)
//
// }
func Test_test_store_delete_prefix(t *testing.T) {
	newBlockGenerator := func(startBlock uint64, inclusiveStopBlock uint64) TestBlockGenerator {
		return &LinearBlockGenerator{
			startBlock:         startBlock,
			inclusiveStopBlock: inclusiveStopBlock,
		}
	}
	_, err := runTest(
		t,
		nil,
		30,
		41,
		[]string{"test_store_delete_prefix", "assert_test_store_delete_prefix"},
		10,
		1, newBlockGenerator, func(p *pipeline.Pipeline, b *bstream.Block, stores store.Map, baseStore dstore.Store) {
			if b.Number == 40 {
				s, storeFound := stores.Get("test_store_delete_prefix")
				require.True(t, storeFound)
				require.Equal(t, uint64(1), s.Length())
			}
		},
	)
	require.NoError(t, err)
}

// -------------------- StoreAddI64 -------------------- //
func Test_test_store_add_i64(t *testing.T) {
	newBlockGenerator := func(startBlock uint64, inclusiveStopBlock uint64) TestBlockGenerator {
		return &LinearBlockGenerator{
			startBlock:         startBlock,
			inclusiveStopBlock: inclusiveStopBlock,
		}
	}
	_, err := runTest(t, nil, 1, 2, []string{"setup_test_store_add_i64", "assert_test_store_add_i64"}, 10, 1, newBlockGenerator, nil)
	require.NoError(t, err)
}

func Test_test_store_add_i64_deltas(t *testing.T) {
	newBlockGenerator := func(startBlock uint64, inclusiveStopBlock uint64) TestBlockGenerator {
		return &LinearBlockGenerator{
			startBlock:         startBlock,
			inclusiveStopBlock: inclusiveStopBlock,
		}
	}
	_, err := runTest(t, nil, 1, 2, []string{"setup_test_store_add_i64", "assert_test_store_add_i64_deltas"}, 10, 1, newBlockGenerator, nil)
	require.NoError(t, err)
}

// -------------------- StoreSetI64/StoreGetI64 -------------------- //
func Test_test_store_set_i64(t *testing.T) {
	newBlockGenerator := func(startBlock uint64, inclusiveStopBlock uint64) TestBlockGenerator {
		return &LinearBlockGenerator{
			startBlock:         startBlock,
			inclusiveStopBlock: inclusiveStopBlock,
		}
	}
	_, err := runTest(t, nil, 20, 31, []string{"setup_test_store_set_i64", "assert_test_store_set_i64"}, 10, 1, newBlockGenerator, nil)
	require.NoError(t, err)
}

func Test_test_store_root_depend(t *testing.T) {
	newBlockGenerator := func(startBlock uint64, inclusiveStopBlock uint64) TestBlockGenerator {
		return &LinearBlockGenerator{
			startBlock:         startBlock,
			inclusiveStopBlock: inclusiveStopBlock,
		}
	}

	_, err := runTest(t, nil, 10, 11, []string{"store_depends_on_depend"}, 10, 10, newBlockGenerator, nil)
	require.NoError(t, err)
}

func Test_test_store_string_get(t *testing.T) {
	newBlockGenerator := func(startBlock uint64, inclusiveStopBlock uint64) TestBlockGenerator {
		return &LinearBlockGenerator{
			startBlock:         startBlock,
			inclusiveStopBlock: inclusiveStopBlock,
		}
	}

	_, err := runTest(t, nil, 10, 11, []string{"setup_test_store_get_set_string", "assert_test_store_get_set_string"}, 10, 1, newBlockGenerator, nil)
	require.NoError(t, err)
}

func Test_test_store_get_array_string(t *testing.T) {
	newBlockGenerator := func(startBlock uint64, inclusiveStopBlock uint64) TestBlockGenerator {
		return &LinearBlockGenerator{
			startBlock:         startBlock,
			inclusiveStopBlock: inclusiveStopBlock,
		}
	}

	_, err := runTest(t, nil, 1, 2, []string{"setup_test_store_get_array_string", "assert_test_store_get_array_string"}, 10, 1, newBlockGenerator, nil)
	require.NoError(t, err)
}

func Test_test_store_get_array_proto(t *testing.T) {
	newBlockGenerator := func(startBlock uint64, inclusiveStopBlock uint64) TestBlockGenerator {
		return &LinearBlockGenerator{
			startBlock:         startBlock,
			inclusiveStopBlock: inclusiveStopBlock,
		}
	}

	_, err := runTest(t, nil, 1, 5, []string{"setup_test_store_get_array_proto", "assert_test_store_get_array_proto"}, 10, 1, newBlockGenerator, nil)
	require.NoError(t, err)
}

func Test_assert_all_test(t *testing.T) {
	newBlockGenerator := func(startBlock uint64, inclusiveStopBlock uint64) TestBlockGenerator {
		return &LinearBlockGenerator{
			startBlock:         startBlock,
			inclusiveStopBlock: inclusiveStopBlock,
		}
	}
	_, err := runTest(t, nil, 20, 31, []string{"assert_all_test"}, 10, 1, newBlockGenerator, nil)
	require.NoError(t, err)
}

func Test_assert_all_string(t *testing.T) {
	test := newTestRun(20, 31, "assert_all_test_string")

	require.NoError(t, test.Run(t))

	assert.Equal(t, []string{`hello`}, test.Logs())
}

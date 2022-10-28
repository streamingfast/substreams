package reqctx

import (
	"testing"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

func Test_ResolveStartBlockNumStepNew(t *testing.T) {
	testCases := struct {
		name             string
		req              *pbsubstreams.Request
		expectedBlockNum uint64
	}{}
	_ = testCases
}

// todo:
//func Test_SimpleMapModuleWithCursor(t *testing.T) {
//	newBlockGenerator := func(startBlock uint64, inclusiveStopBlock uint64) TestBlockGenerator {
//		return &LinearBlockGenerator{
//			startBlock:         startBlock,
//			inclusiveStopBlock: inclusiveStopBlock,
//		}
//	}
//	cursor := &bstream.Cursor{
//		Step:      bstream.StepNew,
//		Block:     bstream.NewBlockRef("10a", 10),
//		LIB:       bstream.NewBlockRef("9a", 9),
//		HeadBlock: bstream.NewBlockRef("10a", 10),
//	}
//	moduleOutputs, err := runTest(t, cursor, 10, 12, []string{"test_map"}, 10, newBlockGenerator, nil)
//	require.NoError(t, err)
//	require.Equal(t, []string{
//		//		`{"name":"test_map","result":{"block_number":10,"block_hash":"block-10"}}`, // we already saw block 10, it is the cursor
//		`{"name":"test_map","result":{"block_number":11,"block_hash":"block-11"}}`,
//	}, moduleOutputs)
//}
//
//func Test_SimpleMapModuleWithCursorUndo(t *testing.T) {
//	newBlockGenerator := func(startBlock uint64, inclusiveStopBlock uint64) TestBlockGenerator {
//		return &LinearBlockGenerator{
//			startBlock:         startBlock,
//			inclusiveStopBlock: inclusiveStopBlock,
//		}
//	}
//	cursor := &bstream.Cursor{
//		Step:      bstream.StepUndo,
//		Block:     bstream.NewBlockRef("10a", 10),
//		LIB:       bstream.NewBlockRef("8a", 8),
//		HeadBlock: bstream.NewBlockRef("10a", 10),
//	}
//	moduleOutputs, err := runTest(t, cursor, 10, 12, []string{"test_map"}, 10, newBlockGenerator, nil)
//	require.NoError(t, err)
//	require.Equal(t, []string{
//		`{"name":"test_map","result":{"block_number":10,"block_hash":"block-10"}}`, // customer has "undone" block 10, need to send it again
//		`{"name":"test_map","result":{"block_number":11,"block_hash":"block-11"}}`,
//	}, moduleOutputs)
//}

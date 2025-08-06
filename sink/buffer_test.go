package sink

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/bobg/go-generics/v2/slices"
	pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/stretchr/testify/require"
)

func TestBlockBuffer(t *testing.T) {
	type messageToResult map[*substreamsMessage]any
	type finalBlocks []*pbsubstreamsrpc.BlockScopedData

	tests := []struct {
		name     string
		undoSize int
		// gives you the chance to populate the state and assert for each message the return value
		// of either `blockBuffer.HandleBlockScopedData(...) (finalBlock BlockScopedData, error)`
		// or `blockBuffer.HandleBlockUndoSignal(...) error`. The expected value being `any`,
		// you can use either `BlockScopedData` or an error.
		messages []messageToResult
	}{
		{
			name:     "one_new_and_not_full",
			undoSize: 3,
			messages: []messageToResult{
				{msgBlockScopedData("1a", 0): nil},
			},
		},
		{
			name:     "all_new_until_exactly_full",
			undoSize: 3,
			messages: []messageToResult{
				{msgBlockScopedData("1a", 0): nil},
				{msgBlockScopedData("2a", 0): nil},
				{msgBlockScopedData("3a", 0): nil},
			},
		},
		{
			name:     "all_new_until_we_final_block_need_to_be_pushed_out",
			undoSize: 3,
			messages: []messageToResult{
				{msgBlockScopedData("1a", 0): nil},
				{msgBlockScopedData("2a", 0): nil},
				{msgBlockScopedData("3a", 0): nil},
				{msgBlockScopedData("4a", 0): blockScopedData("1a", 0)},
			},
		},
		{
			name:     "new_block_is_lower_than_our_highest_block_invalid_invariant",
			undoSize: 2,
			messages: []messageToResult{
				{msgBlockScopedData("2a", 0): nil},
				{msgBlockScopedData("1a", 0): errors.New("received new block scoped data (Block #1 (a)) whose height is lower or equal than our most recent block (Block #2 (a))")},
			},
		},
		{
			name:     "new_block_is_higher_than_our_highest_block_but_skip_block_height",
			undoSize: 2,
			messages: []messageToResult{
				{msgBlockScopedData("2a", 0): nil},
				{msgBlockScopedData("5a", 0): nil},
			},
		},
		{
			name:     "undo_while_not_full_and_no_new_after",
			undoSize: 3,
			messages: []messageToResult{
				{msgBlockScopedData("1a", 0): nil},
				{msgBlockScopedData("2a", 0): nil},
				{msgBlockUndoSignal("1a"): nil},
			},
		},
		{
			name:     "undo_while_not_full_and_push_new_until_pop",
			undoSize: 2,
			messages: []messageToResult{
				{msgBlockScopedData("1a", 0): nil},
				{msgBlockScopedData("2a", 0): nil},
				{msgBlockUndoSignal("1a"): nil},
				{msgBlockScopedData("2b", 0): nil},
				{msgBlockScopedData("3b", 0): blockScopedData("1a", 0)},
				{msgBlockScopedData("4b", 0): blockScopedData("2b", 0)},
				{msgBlockScopedData("5b", 0): blockScopedData("3b", 0)},
			},
		},
		{
			name:     "start_with_undo_signal_while_empty",
			undoSize: 2,
			messages: []messageToResult{
				{msgBlockUndoSignal("3b"): nil},
			},
		},
		{
			name:     "undo_signal_last_valid_block_is_exactly_the_one_we_sent_last_to_user",
			undoSize: 2,
			messages: []messageToResult{
				{msgBlockScopedData("1a", 0): nil},
				{msgBlockScopedData("2a", 0): nil},
				{msgBlockScopedData("3a", 0): blockScopedData("1a", 0)},
				{msgBlockUndoSignal("1a"): nil},
				{msgBlockScopedData("2b", 0): nil},
				{msgBlockScopedData("3b", 0): nil},
				{msgBlockScopedData("4b", 0): blockScopedData("2b", 0)},
				{msgBlockScopedData("5b", 0): blockScopedData("3b", 0)},
			},
		},
		{
			name:     "undo_signal_last_valid_block_is_below_height_of_the_one_we_sent_last_to_user",
			undoSize: 2,
			messages: []messageToResult{
				{msgBlockScopedData("2a", 0): nil},
				{msgBlockScopedData("3a", 0): nil},
				{msgBlockScopedData("4a", 0): blockScopedData("2a", 0)},
				{msgBlockUndoSignal("1a"): errors.New("cannot undo down to last valid Block #1 (a) because we already sent you Block #2 (a) which is after last valid block")},
			},
		},
		{
			// I don't think this case is actually possible in reality, but let's try to cover everything here
			name:     "undo_signal_last_valid_block_is_exactly_at_height_of_the_one_we_sent_last_to_user_but_wrong_id",
			undoSize: 2,
			messages: []messageToResult{
				{msgBlockScopedData("1a", 0): nil},
				{msgBlockScopedData("2a", 0): nil},
				{msgBlockScopedData("3a", 0): blockScopedData("1a", 0)},
				{msgBlockUndoSignal("1b"): errors.New("cannot undo down to last valid Block #1 (b) because we already sent you Block #1 (a) which is after last valid block")},
			},
		},
		{
			name:     "undo_signal_new_block_then_undo_lower",
			undoSize: 5,
			messages: []messageToResult{
				{msgBlockScopedData("1a", 0): nil},
				{msgBlockScopedData("2a", 0): nil},
				{msgBlockScopedData("3a", 0): nil},
				{msgBlockUndoSignal("2a"): nil},
				{msgBlockScopedData("3b", 0): nil},
				{msgBlockScopedData("4b", 0): nil},
				{msgBlockUndoSignal("1a"): nil},
				{msgBlockScopedData("2c", 0): nil},
				{msgBlockScopedData("3c", 0): nil},
				{msgBlockScopedData("4c", 0): nil},
				{msgBlockScopedData("5c", 0): nil},
				{msgBlockScopedData("6c", 0): blockScopedData("1a", 0)},
				{msgBlockScopedData("7c", 0): blockScopedData("2c", 0)},
			},
		},
		{
			name:     "undo_before_seen_lowest_block",
			undoSize: 3,
			messages: []messageToResult{
				{msgBlockScopedData("2a", 0): nil},
				{msgBlockScopedData("3a", 0): nil},
				{msgBlockScopedData("4a", 0): nil},
				{msgBlockUndoSignal("1a"): nil},
				{msgBlockScopedData("2b", 0): nil},
				{msgBlockScopedData("3b", 0): nil},
				{msgBlockScopedData("4b", 0): nil},
				{msgBlockScopedData("5b", 0): blockScopedData("2b", 0)},
			},
		},
		{
			name:     "new_block_is_final_right_away",
			undoSize: 3,
			messages: []messageToResult{
				{msgBlockScopedData("2a", 2): blockScopedData("2a", 2)},
			},
		},
		{
			name:     "new_until_full_then_next_finalizes_everything_including_itself",
			undoSize: 3,
			messages: []messageToResult{
				{msgBlockScopedData("2a", 1): nil},
				{msgBlockScopedData("3a", 1): nil},
				{msgBlockScopedData("4a", 1): nil},
				{msgBlockScopedData("5a", 5): finalBlocks{
					blockScopedData("2a", 1),
					blockScopedData("3a", 1),
					blockScopedData("4a", 1),
					blockScopedData("5a", 5),
				}},
			},
		},
		{
			name:     "new_until_full_then_next_finalizes_everything_including_itself_then_new_to_pop_again",
			undoSize: 2,
			messages: []messageToResult{
				{msgBlockScopedData("2a", 1): nil},
				{msgBlockScopedData("3a", 1): nil},
				{msgBlockScopedData("4a", 4): finalBlocks{
					blockScopedData("2a", 1),
					blockScopedData("3a", 1),
					blockScopedData("4a", 4),
				}},
				{msgBlockScopedData("5a", 4): nil},
				{msgBlockScopedData("6a", 4): nil},
				{msgBlockScopedData("7a", 4): blockScopedData("5a", 4)},
			},
		},
		{
			name:     "new_then_finalize_everything_without_full_capacity",
			undoSize: 3,
			messages: []messageToResult{
				{msgBlockScopedData("2a", 1): nil},
				{msgBlockScopedData("3a", 3): finalBlocks{
					blockScopedData("2a", 1),
					blockScopedData("3a", 3),
				}},
				{msgBlockScopedData("5a", 4): nil},
				{msgBlockScopedData("6a", 4): nil},
				{msgBlockScopedData("7a", 4): nil},
				{msgBlockScopedData("8a", 4): blockScopedData("5a", 4)},
			},
		},
		{
			name:     "new_then_finalize_everything_much_before_oldest_blocl",
			undoSize: 3,
			messages: []messageToResult{
				{msgBlockScopedData("2a", 1): nil},
				{msgBlockScopedData("3a", 1): nil},
				{msgBlockScopedData("4a", 1): nil},
				{msgBlockScopedData("5a", 10): finalBlocks{
					blockScopedData("2a", 1),
					blockScopedData("3a", 1),
					blockScopedData("4a", 1),
					blockScopedData("5a", 10),
				}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newBlockDataBuffer(tt.undoSize)

			handleBlockScopedData := func(blockScopedData *pbsubstreamsrpc.BlockScopedData, expectedResult any) {
				results, err := b.HandleBlockScopedData(blockScopedData)

				switch v := expectedResult.(type) {
				case error:
					require.EqualError(t, err, v.Error())

				case *pbsubstreamsrpc.BlockScopedData:
					require.NoError(t, err)
					require.Equal(t, []*pbsubstreamsrpc.BlockScopedData{v}, results)

				case finalBlocks:
					require.NoError(t, err)

					received := slices.Map(results, func(block *pbsubstreamsrpc.BlockScopedData) string { return "Block " + blockToRef(block).String() })
					require.Equal(t, ([]*pbsubstreamsrpc.BlockScopedData)(v), results, "Received %s", strings.Join(received, ", "))

				default:
					if v == nil {
						require.NoError(t, err)
					} else {
						require.FailNow(t, fmt.Sprintf(`invalid expected result value of type "%T"`, v))
					}
				}
			}

			handleBlockUndoSignal := func(blockUndoSignal *pbsubstreamsrpc.BlockUndoSignal, expectedResult any) {
				err := b.HandleBlockUndoSignal(blockUndoSignal)

				switch v := expectedResult.(type) {
				case error:
					require.EqualError(t, err, v.Error())

				default:
					if v == nil {
						require.NoError(t, err)
					} else {
						require.FailNow(t, fmt.Sprintf(`invalid expected result value of type "%T"`, v))
					}
				}
			}

			for _, msgToExpected := range tt.messages {
				// We actually have a single key/value in each map
				require.Len(t, msgToExpected, 1)

				for msg, expectedResult := range msgToExpected {
					switch {
					case msg.blockScopedData != nil:
						handleBlockScopedData(msg.blockScopedData, expectedResult)

					case msg.blockUndoSignal != nil:
						handleBlockUndoSignal(msg.blockUndoSignal, expectedResult)

					default:
						require.FailNow(t, "substreams message is neither BlockScopedData nor BlockUndoSignal")
					}
				}
			}
		})
	}
}

var blockIDRegex = regexp.MustCompile("([0-9]{1,2})([a-z])")

func blockScopedData(id string, finalBLockHeight uint64) *pbsubstreamsrpc.BlockScopedData {
	number, id := extractNumberAndIDFromBlockID(id)

	return &pbsubstreamsrpc.BlockScopedData{
		Clock: &pbsubstreams.Clock{
			Number: number,
			Id:     id,
		},
		FinalBlockHeight: finalBLockHeight,
		Cursor:           "",
	}
}

func msgBlockScopedData(id string, finalBLockHeight uint64) *substreamsMessage {
	return &substreamsMessage{
		blockScopedData: blockScopedData(id, finalBLockHeight),
	}
}

func msgBlockUndoSignal(id string) *substreamsMessage {
	number, id := extractNumberAndIDFromBlockID(id)

	return &substreamsMessage{
		blockUndoSignal: &pbsubstreamsrpc.BlockUndoSignal{
			LastValidBlock: &pbsubstreams.BlockRef{
				Id:     id,
				Number: number,
			},
			LastValidCursor: "",
		},
	}
}

func extractNumberAndIDFromBlockID(in string) (number uint64, id string) {
	matches := blockIDRegex.FindAllStringSubmatch(in, 1)
	if len(matches) == 0 {
		panic(fmt.Errorf("expected block id to match %q but it did not", blockIDRegex))
	}

	groups := matches[0]

	number, _ = strconv.ParseUint(groups[1], 10, 16)
	id = groups[2]
	return
}

type substreamsMessage struct {
	blockScopedData *pbsubstreamsrpc.BlockScopedData
	blockUndoSignal *pbsubstreamsrpc.BlockUndoSignal
}

func Test_BlockDataBuffer_Capacity(t *testing.T) {
	require.Equal(t, 1, newBlockDataBuffer(1).Capacity())
	require.Equal(t, 12, newBlockDataBuffer(12).Capacity())
}

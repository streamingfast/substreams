package pipeline

import (
	"context"
	"fmt"
	"testing"

	"github.com/streamingfast/bstream"
	pbbstream "github.com/streamingfast/bstream/pb/sf/bstream/v1"
	pbacme "github.com/streamingfast/dummy-blockchain/pb/sf/acme/type/v1"
	"github.com/streamingfast/substreams"
	"github.com/streamingfast/substreams/metrics"
	pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/pipeline/exec"
	"github.com/streamingfast/substreams/reqctx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/anypb"
)

func TestHandleStepPartial(t *testing.T) {
	tests := []struct {
		name               string
		blocks             []blockWithStep
		pendingUndoMessage *pbsubstreamsrpc.Response
		expectedResponses  []substreamsResp
	}{
		{
			name:               "pending undo message is flushed by the first partial block, not muting the stream",
			pendingUndoMessage: pendingUndoMsg(3, "3a"),
			blocks: []blockWithStep{
				withStepNew(testPartialBlock(4, "4b", 1, false)),
				withStepNew(testPartialBlock(4, "4a", 2, true)),
				withStepNew(testBlock(5, "5a")),
			},
			expectedResponses: []substreamsResp{
				undoResp(3, "3a"),
				partialDataResp(4, "4b", 1, false),
				partialDataResp(4, "4a", 2, true),
				dataResp(5, "5a"),
			},
		},
		{
			name:               "pending undo message flushed when resuming on settled partial blocks near head",
			pendingUndoMessage: pendingUndoMsg(3, "3a"),
			blocks: []blockWithStep{
				withStepNew(testPartialBlock(4, "4a", 2, true)),
				withStepNew(testPartialBlock(5, "5a", 2, true)),
				withStepNew(testPartialBlock(6, "6b", 1, false)),
			},
			expectedResponses: []substreamsResp{
				undoResp(3, "3a"),
				partialDataResp(4, "4a", 2, true),
				partialDataResp(5, "5a", 2, true),
				partialDataResp(6, "6b", 1, false),
			},
		},
		{
			name: "bstream test output: happy path with partial block",
			blocks: []blockWithStep{
				withStepNew(testBlock(3, "3a")),
				withStepNew(testPartialBlock(4, "4b", 1, false)),
				withStepNew(testPartialBlock(4, "4a", 3, true)),
				withStepNew(testBlock(5, "5a")),
			},
			expectedResponses: []substreamsResp{
				dataResp(3, "3a"),
				partialDataResp(4, "4b", 1, false),
				partialDataResp(4, "4a", 3, true),
				dataResp(5, "5a"),
			},
		},
		{
			name: "bstream test output: undo wrong lastPartialBlock",
			blocks: []blockWithStep{
				withStepNew(testBlock(3, "3a")),
				withStepNew(testPartialBlock(4, "4b", 1, false)),
				withStepNew(testPartialBlock(4, "4c", 3, true)),
				withStepUndo(testPartialBlock(4, "4c", 3, true), 3, "3a"),
				withStepNew(testBlock(4, "4a")),
				withStepNew(testBlock(5, "5a")),
			},
			expectedResponses: []substreamsResp{
				dataResp(3, "3a"),
				partialDataResp(4, "4b", 1, false),
				partialDataResp(4, "4c", 3, true),
				undoResp(3, "3a"),
				dataResp(4, "4a"),
				dataResp(5, "5a"),
			},
		},

		{
			name: "bstream test output: undo partial scenario",
			blocks: []blockWithStep{
				withStepNew(testBlock(3, "3a")),
				withStepNew(testPartialBlock(4, "4a", 3, true)),
				withStepNew(testPartialBlock(5, "5b", 1, false)),
				withStepUndo(testPartialBlock(5, "5b", 3, true), 4, "4a"),
				withStepNew(testBlock(5, "5a")),
			},
			expectedResponses: []substreamsResp{
				dataResp(3, "3a"),
				partialDataResp(4, "4a", 3, true),
				partialDataResp(5, "5b", 1, false),
				undoResp(4, "4a"),
				dataResp(5, "5a"),
			},
		},
		{
			name: "bstream test output: undo partial scenario 2",
			blocks: []blockWithStep{
				withStepNew(testBlock(3, "3a")),
				withStepNew(testPartialBlock(4, "4b", 2, false)),
				withStepNew(testPartialBlock(4, "4a", 3, false)),
				withStepNew(testBlock(4, "4a")),
				withStepNew(testPartialBlock(5, "5b", 1, false)),
				withStepNew(testPartialBlock(5, "5a", 3, true)),
			},
			expectedResponses: []substreamsResp{
				dataResp(3, "3a"),
				partialDataResp(4, "4b", 2, false),
				partialDataResp(4, "4a", 3, false),
				partialDataResp(4, "4a", 4, true), // triggered by the real testBlock(4a), bumped the index
				partialDataResp(5, "5b", 1, false),
				partialDataResp(5, "5a", 3, true),
			},
		},
		{
			name: "bstream test output: undo partial scenario 3",
			blocks: []blockWithStep{
				withStepNew(testBlock(3, "3a")),
				withStepNew(testPartialBlock(4, "4b", 2, false)),
				withStepNew(testBlock(4, "4b")),
				withStepUndo(testBlock(4, "4b"), 3, "3a"),
				withStepNew(testBlock(4, "4a")),
				withStepNew(testPartialBlock(5, "5b", 1, false)),
			},
			expectedResponses: []substreamsResp{
				dataResp(3, "3a"),
				partialDataResp(4, "4b", 2, false),
				partialDataResp(4, "4b", 3, true),
				undoResp(3, "3a"),
				dataResp(4, "4a"),
				partialDataResp(5, "5b", 1, false),
			},
		},
		{
			name: "bstream test output: undo incomplete partialblock sequence",
			blocks: []blockWithStep{
				withStepNew(testBlock(3, "3a")),
				withStepNew(testPartialBlock(4, "4b", 2, false)),
				withStepUndo(testPartialBlock(4, "4b", 3, false), 3, "3a"),
				withStepNew(testBlock(4, "4a")),
				withStepNew(testBlock(5, "5a")),
			},
			expectedResponses: []substreamsResp{
				dataResp(3, "3a"),
				partialDataResp(4, "4b", 2, false),
				undoResp(3, "3a"),
				dataResp(4, "4a"),
				dataResp(5, "5a"),
			},
		},
		{
			name: "partials with lastPartial, a few full blocks",
			blocks: []blockWithStep{
				withStepNew(testPartialBlock(1, "1x", 3, false)),
				withStepNew(testPartialBlock(1, "1a", 4, true)),
				withStepNew(testBlock(1, "1a")), // should not be sent: same as last partial(4)
				withStepNew(testBlock(2, "2b")),
				withStepUndo(testBlock(2, "2b"), 1, "1a"),
				withStepNew(testBlock(2, "2a")),
			},
			expectedResponses: []substreamsResp{
				partialDataResp(1, "1x", 3, false),
				partialDataResp(1, "1a", 4, true),
				// should not be sent again
				dataResp(2, "2b"),
				undoResp(1, "1a"),
				dataResp(2, "2a"),
			},
		},
		{
			name: "partials with real block equal to previous partial that is was not yet LastPartial",
			blocks: []blockWithStep{
				withStepNew(testBlock(1, "1a")),                  // 1a
				withStepNew(testPartialBlock(2, "2x", 3, false)), // p:2x
				withStepNew(testPartialBlock(2, "2a", 4, false)), // p:2a
				withStepNew(testBlock(2, "2a")),                  // p:2a(last)
				withStepNew(testPartialBlock(2, "2a", 4, true)),  // noop (redundant)
			},
			expectedResponses: []substreamsResp{
				dataResp(1, "1a"),
				partialDataResp(2, "2x", 3, false),
				partialDataResp(2, "2a", 4, false),
				partialDataResp(2, "2a", 5, true),
			},
		},
		{
			name: "partials missing last, then same full block",
			blocks: []blockWithStep{
				withStepNew(testPartialBlock(1, "1x", 2, false)),
				withStepNew(testPartialBlock(1, "1y", 3, false)),
				withStepUndo(testBlock(1, "1y"), 0, "0a"),
				withStepNew(testBlock(1, "1a")),
				withStepNew(testBlock(2, "2a")),
			},
			expectedResponses: []substreamsResp{
				partialDataResp(1, "1x", 2, false),
				partialDataResp(1, "1y", 3, false),
				undoResp(0, "0a"),
				dataResp(1, "1a"),
				dataResp(2, "2a"),
			},
		},
		{
			name: "partials with lastPartial, then older StepNew blocks should be ignored",
			blocks: []blockWithStep{
				withStepNew(testBlock(8, "8a")),
				withStepNew(testPartialBlock(9, "9a", 4, true)),
				withStepNew(testPartialBlock(10, "10x", 3, false)),
				withStepNew(testPartialBlock(10, "10a", 4, true)),
				withStepNew(testBlock(9, "9a")),   // should not be sent: older than last partial
				withStepNew(testBlock(10, "10a")), // should not be sent: same number as last partial
				withStepNew(testBlock(11, "11a")), // should be sent: newer than last partial
			},
			expectedResponses: []substreamsResp{
				dataResp(8, "8a"),
				partialDataResp(9, "9a", 4, true),
				partialDataResp(10, "10x", 3, false),
				partialDataResp(10, "10a", 4, true),
				// blocks 9, 10 should not be sent
				dataResp(11, "11a"),
			},
		},
		{
			name: "partial with mismatching transactions triggers undo then waits for the last partial, not an error",
			blocks: []blockWithStep{
				withStepNew(testBlock(3, "3a")),
				withTxs(withStepNew(testPartialBlock(4, "4x", 1, false)), "t1", "t2"),
				withTxs(withStepNew(testPartialBlock(4, "4y", 2, false)), "t1", "t2", "t3"), // extends 4x: no undo
				withTxs(withStepNew(testPartialBlock(4, "4z", 3, false)), "o1", "o2"),       // different transactions: new version of block 4
				withTxs(withStepNew(testPartialBlock(4, "4a", 4, true)), "o1", "o2", "o3"),  // last partial settles the block
				withStepNew(testBlock(5, "5a")),
			},
			expectedResponses: []substreamsResp{
				dataResp(3, "3a"),
				partialDataResp(4, "4x", 1, false),
				partialDataResp(4, "4y", 2, false),
				undoResp(3, "3a"),
				// 4z is not sent: after the undo, we wait for the last partial of block 4
				partialDataResp(4, "4a", 4, true),
				dataResp(5, "5a"),
			},
		},
		{
			name: "partial with fewer transactions than already sent triggers undo",
			blocks: []blockWithStep{
				withStepNew(testBlock(3, "3a")),
				withTxs(withStepNew(testPartialBlock(4, "4x", 1, false)), "t1", "t2", "t3"),
				withTxs(withStepNew(testPartialBlock(4, "4y", 2, false)), "t1"), // shrunk: new version of block 4
				withTxs(withStepNew(testPartialBlock(4, "4a", 3, true)), "t1", "t2"),
			},
			expectedResponses: []substreamsResp{
				dataResp(3, "3a"),
				partialDataResp(4, "4x", 1, false),
				undoResp(3, "3a"),
				partialDataResp(4, "4a", 3, true),
			},
		},
		{
			name: "churning partial versions trigger a single undo then silence until the last partial",
			blocks: []blockWithStep{
				withStepNew(testBlock(3, "3a")),
				withTxs(withStepNew(testPartialBlock(4, "4w", 1, false)), "t1"),
				withTxs(withStepNew(testPartialBlock(4, "4x", 2, false)), "t1", "t2"),
				withTxs(withStepNew(testPartialBlock(4, "4y", 3, false)), "u1", "u2"),       // mismatch: undo, then mute
				withTxs(withStepNew(testPartialBlock(4, "4z", 4, false)), "u1", "u2", "u3"), // muted, even though it extends 4y
				withTxs(withStepNew(testPartialBlock(4, "4q", 5, false)), "v1"),             // muted
				withTxs(withStepNew(testPartialBlock(4, "4a", 6, true)), "v1", "v2"),        // last partial settles the block
				withStepNew(testBlock(5, "5a")),
			},
			expectedResponses: []substreamsResp{
				dataResp(3, "3a"),
				partialDataResp(4, "4w", 1, false),
				partialDataResp(4, "4x", 2, false),
				undoResp(3, "3a"),
				partialDataResp(4, "4a", 6, true),
				dataResp(5, "5a"),
			},
		},
		{
			name: "undo to the same junction after a mismatch-undo is not suppressed",
			blocks: []blockWithStep{
				withStepNew(testBlock(3, "3a")),
				withTxs(withStepNew(testPartialBlock(4, "4x", 1, false)), "t1", "t2"),
				withTxs(withStepNew(testPartialBlock(4, "4y", 2, false)), "u1", "u2"), // mismatch: undo to 3a, then mute
				withTxs(withStepNew(testPartialBlock(4, "4b", 3, true)), "u1", "u2", "u3"),
				withStepUndo(testBlock(4, "4b"), 3, "3a"), // chain reorgs block 4 away: undo to 3a again
				withStepNew(testBlock(4, "4a")),
			},
			expectedResponses: []substreamsResp{
				dataResp(3, "3a"),
				partialDataResp(4, "4x", 1, false),
				undoResp(3, "3a"),
				partialDataResp(4, "4b", 3, true),
				undoResp(3, "3a"), // must not be suppressed by the undo dedup
				dataResp(4, "4a"),
			},
		},
		{
			name: "full block with mismatching transactions triggers undo and is sent as last partial",
			blocks: []blockWithStep{
				withStepNew(testBlock(3, "3a")),
				withTxs(withStepNew(testPartialBlock(4, "4y", 1, false)), "t1", "t2"),
				withTxs(withStepNew(testBlock(4, "4y")), "o1", "o2"), // same ID as the partial but different content
				withStepNew(testBlock(5, "5a")),
			},
			expectedResponses: []substreamsResp{
				dataResp(3, "3a"),
				partialDataResp(4, "4y", 1, false),
				undoResp(3, "3a"),
				partialDataResp(4, "4y", 2, true),
				dataResp(5, "5a"),
			},
		},
		{
			name: "partials on a chain with skipped block numbers are accepted",
			blocks: []blockWithStep{
				withStepNew(testBlock(3, "3a")),
				withTxs(withStepNew(testPartialBlock(5, "5x", 1, false)), "t1"), // block 4 skipped
				withTxs(withStepNew(testPartialBlock(5, "5a", 2, true)), "t1", "t2"),
			},
			expectedResponses: []substreamsResp{
				dataResp(3, "3a"),
				partialDataResp(5, "5x", 1, false),
				partialDataResp(5, "5a", 2, true),
			},
		},
		{
			name: "partial undo does not reset 'lastPartial'",
			blocks: []blockWithStep{
				withStepNew(testBlock(8, "8a")),
				withStepNew(testPartialBlock(9, "9a", 4, true)),
				withStepNew(testPartialBlock(10, "10x", 1, false)),
				withStepUndo(testPartialBlock(10, "10x", 1, false), 9, "9a"), // resulst in an UNDO to 9
				withStepNew(testBlock(9, "9a")),                              // should not be sent: older than last partial
				withStepNew(testBlock(10, "10a")),
			},
			expectedResponses: []substreamsResp{
				dataResp(8, "8a"),
				partialDataResp(9, "9a", 4, true),
				partialDataResp(10, "10x", 1, false),
				undoResp(9, "9a"),
				dataResp(10, "10a"),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := setupTestContext(t, "test_module", 200)
			ctx = reqctx.WithPartialBlocks(ctx, true)

			responses := []*pbsubstreamsrpc.Response{}
			respFunc := func(resp substreams.ResponseFromAnyTier) error {
				rpcResp := resp.(*pbsubstreamsrpc.Response)
				if rpcResp.GetBlockScopedData() != nil || rpcResp.GetBlockUndoSignal() != nil {
					responses = append(responses, rpcResp)
				}
				return nil

			}

			blockType := "sf.acme.type.v1.Block"
			outputModule := "outmod"
			pipe := setupTestPipeline(t, [][]exec.ModuleExecutor{{exec.NewTestingExecutor(outputModule, blockType)}})
			pipe.execGraph = newTestModuleGraph(t, outputModule, blockType)
			pipe.respFunc = respFunc
			pipe.ctx = ctx
			pipe.gate = newGate(ctx)
			pipe.gate.passed = true
			pipe.blockType = blockType
			pipe.stateBundleSize = 100
			pipe.pendingUndoMessage = tt.pendingUndoMessage

			for _, blk := range tt.blocks {

				err := fillTestAcmeBlock(blk.blk, blk.txs)
				require.NoError(t, err)

				clock := BlockToClock(blk.blk)
				execOutput := NewExecOutputTesting(t, blk.blk, clock, blockType)
				switch blk.step {
				case bstream.StepNew:
					err = pipe.handleStepNew(ctx, clock, testCursor(blk.blk, blk.step), execOutput, false)
				case bstream.StepPartial, bstream.StepNewPartial: // we always have partial blocks active here
					err = pipe.handleStepPartial(ctx, clock, testCursor(blk.blk, blk.step), execOutput, blk.blk.PreviousRef(), blk.blk.PartialIndex, blk.blk.LastPartial)
				case bstream.StepUndo, bstream.StepUndoPartial:
					err = pipe.handleStepUndo(clock, testCursor(blk.blk, blk.step), blk.junction)
				}
				require.NoError(t, err)
			}

			//	We could verify that executors were called
			//	for _, layer := range pipe.StagedModuleExecutors {
			//		for _, executor := range layer {
			//			te := executor.(*exec.TestingExecutor)
			//			calls := te.GetRunCalls()
			//          ...
			//		}
			//	}
			//}

			require.Equal(t, len(tt.expectedResponses), len(responses), "response count mismatch")

			for i, expected := range tt.expectedResponses {
				resp := responses[i]
				if expected.isUndo {
					// Validate undo signal
					undo := resp.GetBlockUndoSignal()
					require.NotNil(t, undo, "expected undo signal at index %d, got %v", i, resp)
					assert.Equal(t, expected.lastValidBlockNum, undo.LastValidBlock.Number, "undo lastValidBlock number mismatch at index %d", i)
					assert.Equal(t, expected.lastValidBlockID, undo.LastValidBlock.Id, "undo lastValidBlock ID mismatch at index %d", i)
				} else {
					// Validate data response
					data := resp.GetBlockScopedData()
					require.NotNil(t, data, "expected block scoped data at index %d, got %v", i, resp)
					require.NotNil(t, data.Clock, "clock should not be nil at index %d", i)

					assert.Equal(t, expected.clockNumber, data.Clock.Number, "clock number mismatch at index %d", i)
					assert.Equal(t, expected.clockID, data.Clock.Id, "clock ID mismatch at index %d", i)
					assert.Equal(t, expected.isPartial, data.IsPartial, "isPartial mismatch at index %d", i)

					if expected.isPartial {
						require.NotNil(t, data.PartialIndex, "partialIndex should not be nil when isPartial=true at index %d", i)
						assert.Equal(t, expected.partialIndex, *data.PartialIndex, "partialIndex mismatch at index %d", i)

						require.NotNil(t, data.IsLastPartial, "isLastPartial should not be nil when isPartial=true at index %d", i)
						assert.Equal(t, expected.isLastPartial, *data.IsLastPartial, "isLastPartial mismatch at index %d", i)
					} else {
						// For non-partial blocks, these fields should be nil or false
						if data.PartialIndex != nil {
							assert.Equal(t, uint32(0), *data.PartialIndex, "partialIndex should be 0 for non-partial blocks at index %d", i)
						}
					}
				}
			}
		})
	}
}

// Helper functions

func newTestModuleGraph(t *testing.T, outputModule, blockType string) *exec.Graph {
	eg, err := exec.NewOutputModuleGraph(outputModule, false, &pbsubstreams.Modules{
		Modules: []*pbsubstreams.Module{
			{
				Name: "outmod",
				Kind: &pbsubstreams.Module_KindMap_{},
				Inputs: []*pbsubstreams.Module_Input{
					{
						Input: &pbsubstreams.Module_Input_Source_{
							Source: &pbsubstreams.Module_Input_Source{
								Type: blockType,
							},
						},
					},
				},
			},
		},
		Binaries: []*pbsubstreams.Binary{
			{
				Type:    "test",
				Content: []byte("some-fake-binary-data"),
			},
		},
	}, 0)
	require.NoError(t, err)
	return eg
}

func testBlock(num uint64, id string) *pbbstream.Block {
	return &pbbstream.Block{
		Number: num,
		Id:     id,
	}
}

func testPartialBlock(num uint64, id string, idx int32, last bool) *pbbstream.Block {
	return &pbbstream.Block{
		Number:       num,
		Id:           id,
		PartialIndex: idx,
		LastPartial:  last,
	}
}

type blockWithStep struct {
	blk      *pbbstream.Block
	step     bstream.StepType
	junction bstream.BlockRef
	txs      []string
}

// withTxs sets the transaction hashes carried by the underlying acme block
func withTxs(b blockWithStep, txs ...string) blockWithStep {
	b.txs = txs
	return b
}

func withStepUndo(blk *pbbstream.Block, junctionNum uint64, junctionID string) blockWithStep {
	step := bstream.StepUndo
	if blk.PartialIndex != 0 {
		step = bstream.StepUndoPartial
	}
	junction := bstream.NewBlockRef(junctionID, junctionNum)

	return blockWithStep{
		blk:      blk,
		step:     step,
		junction: junction,
	}
}

func withStepNew(blk *pbbstream.Block) blockWithStep {
	step := bstream.StepNew
	if blk.PartialIndex != 0 {
		step = bstream.StepPartial
		if blk.LastPartial {
			step = bstream.StepNewPartial
		}
	}
	return blockWithStep{
		blk:  blk,
		step: step,
	}
}

func fillTestAcmeBlock(blk *pbbstream.Block, txs []string) error {
	var transactions []*pbacme.Transaction
	for _, hash := range txs {
		transactions = append(transactions, &pbacme.Transaction{Hash: hash})
	}
	acmeBlock := &pbacme.Block{
		Header: &pbacme.BlockHeader{
			Height: blk.Number,
			Hash:   blk.Id,
		},
		Transactions: transactions,
	}

	payload, err := anypb.New(acmeBlock)
	if err != nil {
		return err
	}
	blk.Payload = payload
	return nil
}

// substreamsResp is a helper type for expected responses in tests
type substreamsResp struct {
	isUndo        bool
	clockID       string
	clockNumber   uint64
	isPartial     bool
	partialIndex  uint32
	isLastPartial bool
	// For undo signals
	lastValidBlockNum uint64
	lastValidBlockID  string
}

// partialDataResp creates an expected partial data response
func partialDataResp(clockNumber uint64, clockID string, partialIndex uint32, isLastPartial bool) substreamsResp {
	return substreamsResp{
		isUndo:        false,
		clockID:       clockID,
		clockNumber:   clockNumber,
		isPartial:     true,
		partialIndex:  partialIndex,
		isLastPartial: isLastPartial,
	}
}

// dataResp creates an expected data response
func dataResp(clockNumber uint64, clockID string) substreamsResp {
	return substreamsResp{
		isUndo:        false,
		clockID:       clockID,
		clockNumber:   clockNumber,
		isPartial:     false,
		partialIndex:  0,
		isLastPartial: false,
	}
}

// pendingUndoMsg builds the undo message installed at stream setup when resuming from a partial cursor
func pendingUndoMsg(lastValidBlockNum uint64, lastValidBlockID string) *pbsubstreamsrpc.Response {
	return &pbsubstreamsrpc.Response{
		Message: &pbsubstreamsrpc.Response_BlockUndoSignal{
			BlockUndoSignal: &pbsubstreamsrpc.BlockUndoSignal{
				LastValidBlock: &pbsubstreams.BlockRef{Number: lastValidBlockNum, Id: lastValidBlockID},
			},
		},
	}
}

// undoResp creates an expected undo response
func undoResp(lastValidBlockNum uint64, lastValidBlockID string) substreamsResp {
	return substreamsResp{
		isUndo:            true,
		lastValidBlockNum: lastValidBlockNum,
		lastValidBlockID:  lastValidBlockID,
	}
}

func setupTestContext(t *testing.T, outputModule string, stopBlock uint64) context.Context {
	t.Helper()
	ctx := context.Background()
	ctx = reqctx.WithRequest(ctx, &reqctx.RequestDetails{
		OutputModule:   outputModule,
		ProductionMode: false,
		StopBlockNum:   stopBlock,
		Modules:        &pbsubstreams.Modules{}, // Add empty modules
	})
	ctx = reqctx.WithReqStats(ctx, metrics.NewReqStats(&metrics.Config{}, nil, nil, zap.NewNop()))
	return ctx
}

func setupTestPipeline(t *testing.T, executors [][]exec.ModuleExecutor) *Pipeline {
	t.Helper()

	ctx := setupTestContext(t, "test_module", 200)

	pipe := &Pipeline{
		ctx:                   ctx,
		StagedModuleExecutors: executors,
		forkHandler:           NewForkHandler(),
		stores:                &Stores{logger: zap.NewNop()},
		moduleNameToStage:     make(map[string]int),
		blockStepMap:          make(map[bstream.StepType]uint64),
		executionTimeout:      10000000000, // 10 seconds timeout for tests
	}

	// Build module name to stage mapping
	for stageIdx, layer := range executors {
		for _, executor := range layer {
			pipe.moduleNameToStage[executor.Name()] = stageIdx
		}
	}

	return pipe
}

func testCursor(blk *pbbstream.Block, step bstream.StepType) *bstream.Cursor {
	blockID := blk.Id
	blockNum := blk.Number
	lib := blockNum - 10
	if blockNum < 10 {
		lib = 0
	}
	block := bstream.NewBlockRef(blockID, blockNum)
	headBlock := block
	if step == bstream.StepPartial { // not StepNewPartial, where the cursor would not contain previousID
		headBlock = bstream.NewBlockRef(fmt.Sprintf("%da", blockNum-1), blockNum-1)
	}

	return &bstream.Cursor{
		Step:      step,
		Block:     block,
		HeadBlock: headBlock,
		LIB:       bstream.NewBlockRef("lib-"+blockID, lib),
	}
}

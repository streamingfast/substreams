package pipeline

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/streamingfast/bstream"

	pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
)

func Test_resolveStartBlockNum(t *testing.T) {
	tests := []struct {
		name               string
		req                *pbsubstreamsrpc.Request
		expectedBlockNum   uint64
		headBlock          uint64
		headBlockErr       error
		wantErr            bool
		cursorResolverArgs []interface{}
		wantUndoLastBlock  bstream.BlockRef
		wantCursor         string
	}{
		{
			name: "invalid cursor step",
			req: &pbsubstreamsrpc.Request{
				StartBlockNum: 10,
				StartCursor: (&bstream.Cursor{
					Step:      bstream.StepType(0),
					Block:     bstream.NewBlockRef("10a", 10),
					LIB:       bstream.NewBlockRef("9a", 9),
					HeadBlock: bstream.NewBlockRef("10a", 10),
				}).ToOpaque(),
			},
			expectedBlockNum: 0,
			wantErr:          true,
		},
		{
			name: "step undo", // support for 'undo cursor' is kept for backwards compatibility, these cursors are not sent to client anymore
			req: &pbsubstreamsrpc.Request{
				StartBlockNum: 10,
				StartCursor: (&bstream.Cursor{
					Step:      bstream.StepUndo,
					Block:     bstream.NewBlockRef("10a", 10),
					LIB:       bstream.NewBlockRef("9a", 9),
					HeadBlock: bstream.NewBlockRef("10a", 10),
				}).ToOpaque(),
			},
			expectedBlockNum: 10,
			wantErr:          false,
			cursorResolverArgs: []interface{}{
				"c1:2:10:10a:9:9a", bstream.NewBlockRef("9a", 9), bstream.NewBlockRef("9a", 9), nil,
			},
			wantCursor:        "c1:1:9:9a:9:9a",
			wantUndoLastBlock: bstream.NewBlockRef("9a", 9),
		},
		{
			name: "step new",
			req: &pbsubstreamsrpc.Request{
				StartBlockNum: 10,
				StartCursor: (&bstream.Cursor{
					Step:      bstream.StepNew,
					Block:     bstream.NewBlockRef("10a", 10),
					LIB:       bstream.NewBlockRef("9a", 9),
					HeadBlock: bstream.NewBlockRef("10a", 10),
				}).ToOpaque(),
			},
			expectedBlockNum: 11,
			wantErr:          false,
			wantCursor:       "c1:1:10:10a:9:9a",
		},
		{
			name: "step new on forked cursor",
			req: &pbsubstreamsrpc.Request{
				StartBlockNum: 10,
				StartCursor: (&bstream.Cursor{
					Step:      bstream.StepNew,
					Block:     bstream.NewBlockRef("10a", 10),
					LIB:       bstream.NewBlockRef("6a", 6),
					HeadBlock: bstream.NewBlockRef("10a", 10),
				}).ToOpaque(),
			},
			expectedBlockNum: 9,
			wantErr:          false,
			cursorResolverArgs: []interface{}{
				"c1:1:10:10a:6:6a", bstream.NewBlockRef("8a", 8), bstream.NewBlockRef("11a", 11), nil,
			},
			wantUndoLastBlock: bstream.NewBlockRef("8a", 8),
			wantCursor:        "c3:1:8:8a:11:11a:6:6a",
		},
		{
			name: "step irreversible", // substreams should not receive these cursors now, kept for backwards compatibility
			req: &pbsubstreamsrpc.Request{
				StartBlockNum: 10,
				StartCursor: (&bstream.Cursor{
					Step:      bstream.StepIrreversible,
					Block:     bstream.NewBlockRef("10a", 10),
					LIB:       bstream.NewBlockRef("10a", 10),
					HeadBlock: bstream.NewBlockRef("10a", 10),
				}).ToOpaque(),
			},
			expectedBlockNum: 11,
			wantErr:          false,
		},
		{
			name: "step new irreversible",
			req: &pbsubstreamsrpc.Request{
				StartBlockNum: 10,
				StartCursor: (&bstream.Cursor{
					Step:      bstream.StepNewIrreversible,
					Block:     bstream.NewBlockRef("10a", 10),
					LIB:       bstream.NewBlockRef("9a", 9),
					HeadBlock: bstream.NewBlockRef("10a", 10),
				}).ToOpaque(),
			},
			expectedBlockNum: 11,
			wantErr:          false,
			wantCursor:       "c1:17:10:10a:9:9a",
		},
		{
			name: "negative startblock",
			req: &pbsubstreamsrpc.Request{
				StartBlockNum: -5,
			},
			headBlock:        16,
			expectedBlockNum: 11,
			wantErr:          false,
		},
		{
			name: "negative startblock no head",
			req: &pbsubstreamsrpc.Request{
				StartBlockNum: -5,
			},
			headBlockErr: fmt.Errorf("cannot find head block"),
			wantErr:      true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, outCursor, undoSignal, err := resolveStartBlockNum(
				context.Background(),
				tt.req,
				newTestCursorResolver(tt.cursorResolverArgs...).resolveCursor,
				func() (uint64, error) { return tt.headBlock, tt.headBlockErr },
			)
			if tt.wantUndoLastBlock != nil {
				require.NotNil(t, undoSignal)
				assert.Equal(t, tt.wantUndoLastBlock.ID(), undoSignal.LastValidBlock.Id)
				assert.Equal(t, tt.wantUndoLastBlock.Num(), undoSignal.LastValidBlock.Number)
			} else {
				assert.Nil(t, undoSignal)
			}

			if (err != nil) != tt.wantErr {
				t.Errorf("resolveStartBlockNum() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if outCur, err := bstream.CursorFromOpaque(outCursor); err != nil {
				assert.Empty(t, tt.wantCursor)
			} else {
				assert.Equal(t, tt.wantCursor, outCur.String())
			}

			if got != tt.expectedBlockNum {
				t.Errorf("resolveStartBlockNum() got = %v, want %v", got, tt.expectedBlockNum)
			}
		})
	}
}

func ref(in uint64) *uint64 {
	return &in
}

func Test_computeLinaerHandoffBlockNum(t *testing.T) {
	tests := []struct {
		name             string
		liveHubAvailable bool
		recentBlockNum   uint64
		prodMode         bool
		startBlockNum    uint64
		stopBlockNum     uint64
		expectHandoffNum uint64
		expectError      bool
		stateRequiredAt  *uint64
	}{
		// development mode
		{"g1_start_stop_same_boundary", true, 500, false, 138, 142, 100, false, ref(0)},
		{"g1_start_stop_same_boundary_livehub_fails", false, 500, false, 138, 142, 100, false, ref(0)},
		{"g2_start_stop_across_boundary", true, 500, false, 138, 242, 100, false, ref(0)},
		{"g2_start_stop_across_boundary_livehub_fails", true, 500, false, 138, 242, 100, false, ref(0)},
		{"start_with_state_near", true, 500, false, 138, 242, 135, false, ref(135)},
		{"start_with_state_near_livehub_fails", true, 500, false, 138, 242, 135, false, ref(135)},

		// production mode
		{"g4_start_stop_same_boundary", true, 500, true, 138, 142, 200, false, ref(0)},
		{"g5_start_stop_across_boundary", true, 500, true, 138, 242, 300, false, ref(0)},
		{"g6_lib_between_start_and_stop", true, 342, true, 121, 498, 300, false, ref(0)},
		{"g6_lib_between_start_and_stop_livehub_fails", false, 342, true, 121, 498, 500, false, ref(0)},
		{"g7_stop_block_infinity", true, 342, true, 121, 0, 300, false, ref(0)},
		{"g7_stop_block_infinity_livehub_fails", false, 342, true, 121, 0, 300, true, ref(0)},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := computeLinearHandoffBlockNum(
				test.prodMode,
				test.startBlockNum,
				test.stopBlockNum,
				func() (uint64, error) {
					if !test.liveHubAvailable {
						return 0, fmt.Errorf("live not available")
					}
					return test.recentBlockNum, nil
				}, test.stateRequiredAt, 100)
			if test.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, test.expectHandoffNum, got)
			}
		})
	}
}

func TestBuildRequestDetails(t *testing.T) {
	req, _, err := BuildRequestDetails(
		context.Background(),
		&pbsubstreamsrpc.Request{
			StartBlockNum:  10,
			ProductionMode: false,
			OutputModule:   "nomatch",
		},
		func() (uint64, error) {
			assert.True(t, true, "should pass here")
			return 999, nil
		},
		newTestCursorResolver().resolveCursor,
		func() (uint64, error) {
			t.Error("should not pass here")
			return 0, nil
		},
		100,
	)
	require.NoError(t, err)
	assert.Equal(t, 10, int(req.ResolvedStartBlockNum), "resolved start block")
	assert.Equal(t, 0, int(req.LinearHandoffBlockNum), "linear handoff blocknum")

	req, _, err = BuildRequestDetails(
		context.Background(),
		&pbsubstreamsrpc.Request{
			StartBlockNum:  10,
			ProductionMode: true,
			OutputModule:   "",
		},
		func() (uint64, error) {
			return 999, nil
		},
		newTestCursorResolver().resolveCursor,
		func() (uint64, error) {
			t.Error("should not pass here")
			return 0, nil
		},
		100,
	)
	require.NoError(t, err)
	assert.Equal(t, 10, int(req.ResolvedStartBlockNum))
	assert.Equal(t, 900, int(req.LinearHandoffBlockNum))
}

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

func Test_computeLiveHandoffBlockNum(t *testing.T) {
	tests := []struct {
		liveHubAvailable bool
		recentBlockNum   uint64
		prodMode         bool
		startBlockNum    uint64
		stopBlockNum     uint64
		expectHandoffNum uint64
		expectError      bool
		stateRequired    bool
	}{
		// prod (start-block ignored)
		{true, 100, true, 10, 0, 100, false, true},
		{true, 100, true, 10, 150, 100, false, true},
		{true, 100, true, 10, 50, 50, false, true},
		{false, 0, true, 10, 50, 50, false, true},
		{false, 0, true, 10, 0, 0, true, true},

		// prod (start-block ignored) (state not required)
		{true, 100, true, 10, 0, 100, false, false},
		{true, 100, true, 10, 150, 100, false, false},
		{true, 100, true, 10, 50, 50, false, false},
		{false, 0, true, 10, 50, 50, false, false},
		{false, 0, true, 10, 0, 0, true, false},

		// non-prod (stop-block ignored) (state required)
		{true, 100, false, 10, 0, 10, false, true},
		{true, 100, false, 10, 9999, 10, false, true},
		{true, 100, false, 150, 0, 100, false, true},
		{true, 100, false, 150, 9999, 100, false, true},
		{false, 0, false, 150, 0, 150, false, true},
		{false, 0, false, 150, 9999, 150, false, true},

		// non-prod (stop-block ignored) (state not required)
		{true, 100, false, 10, 0, 10, false, false},
		{true, 100, false, 10, 9999, 10, false, false},
		{true, 100, false, 150, 0, 150, false, false},
		{true, 100, false, 150, 9999, 150, false, false},
		{false, 0, false, 150, 0, 150, false, false},
		{false, 0, false, 150, 9999, 150, false, false},
	}

	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			got, err := computeLinearHandoffBlockNum(
				test.prodMode,
				test.startBlockNum,
				test.stopBlockNum,
				func() (uint64, error) {
					if !test.liveHubAvailable {
						return 0, fmt.Errorf("live not available")
					}
					return test.recentBlockNum, nil
				}, test.stateRequired)
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
	)
	require.NoError(t, err)
	assert.Equal(t, 10, int(req.ResolvedStartBlockNum))
	assert.Equal(t, 10, int(req.LinearHandoffBlockNum))

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
	)
	require.NoError(t, err)
	assert.Equal(t, 10, int(req.ResolvedStartBlockNum))
	assert.Equal(t, 999, int(req.LinearHandoffBlockNum))
}

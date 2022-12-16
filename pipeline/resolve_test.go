package pipeline

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/streamingfast/bstream"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

func Test_resolveStartBlockNum(t *testing.T) {
	tests := []struct {
		name             string
		req              *pbsubstreams.Request
		expectedBlockNum uint64
		wantErr          bool
	}{
		{
			name: "invalid cursor step",
			req: &pbsubstreams.Request{
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
			name: "step undo",
			req: &pbsubstreams.Request{
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
		},
		{
			name: "step new",
			req: &pbsubstreams.Request{
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
		},
		{
			name: "step irreversible",
			req: &pbsubstreams.Request{
				StartBlockNum: 10,
				StartCursor: (&bstream.Cursor{
					Step:      bstream.StepIrreversible,
					Block:     bstream.NewBlockRef("10a", 10),
					LIB:       bstream.NewBlockRef("9a", 9),
					HeadBlock: bstream.NewBlockRef("10a", 10),
				}).ToOpaque(),
			},
			expectedBlockNum: 11,
			wantErr:          false,
		},
		{
			name: "step new irreversible",
			req: &pbsubstreams.Request{
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
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveStartBlockNum(tt.req)
			if (err != nil) != tt.wantErr {
				t.Errorf("resolveStartBlockNum() error = %v, wantErr %v", err, tt.wantErr)
				return
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
	}{
		// prod (start-block ignored)
		{true, 100, true, 10, 0, 100, false},
		{true, 100, true, 10, 150, 100, false},
		{true, 100, true, 10, 50, 50, false},
		{false, 0, true, 10, 50, 50, false},
		{false, 0, true, 10, 0, 0, true},

		// non-prod (stop-block ignored)
		{true, 100, false, 10, 0, 10, false},
		{true, 100, false, 10, 9999, 10, false},
		{true, 100, false, 150, 0, 100, false},
		{true, 100, false, 150, 9999, 100, false},
		{false, 0, false, 150, 0, 150, false},
		{false, 0, false, 150, 9999, 150, false},
	}

	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			got, err := computeLiveHandoffBlockNum(
				test.prodMode,
				test.startBlockNum,
				test.stopBlockNum,
				func() (uint64, error) {
					if !test.liveHubAvailable {
						return 0, fmt.Errorf("live not available")
					}
					return test.recentBlockNum, nil
				})
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
	req, err := BuildRequestDetails(&pbsubstreams.Request{
		StartBlockNum:  10,
		ProductionMode: false,
	}, true, func(name string) bool {
		return false
	}, func() (uint64, error) {
		assert.True(t, true, "should pass here")
		return 999, nil
	})
	require.NoError(t, err)
	assert.Equal(t, 10, int(req.RequestStartBlockNum))
	assert.Equal(t, 10, int(req.LinearHandoffBlockNum))

	req, err = BuildRequestDetails(&pbsubstreams.Request{
		StartBlockNum:  10,
		ProductionMode: true,
	}, true, func(name string) bool {
		return true
	}, func() (uint64, error) {
		return 999, nil
	})
	require.NoError(t, err)
	assert.Equal(t, 10, int(req.RequestStartBlockNum))
	assert.Equal(t, 999, int(req.LinearHandoffBlockNum))
}

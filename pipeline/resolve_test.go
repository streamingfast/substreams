package pipeline

// TODO(abourget): do the tests to split incoming requests
// with the proper LiveHandoff block, and historical stream start block
// and pipeline start block and all, with a simple resolver of the
// highest (hard-coded for now?) final block.

import (
	"testing"

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

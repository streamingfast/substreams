package orchestrator

import (
	"testing"

	"github.com/streamingfast/substreams/block"
	"github.com/stretchr/testify/require"
)

func TestSplitter_Split(t *testing.T) {
	type testCase struct {
		name string

		chunkSize uint64

		moduleStartBlock uint64
		lastSavedBlock   uint64
		inputRange       *block.Range

		expectedOutput []*block.Range
	}

	for _, tt := range []testCase{
		{
			name:             "start at zero",
			moduleStartBlock: 50,
			inputRange: &block.Range{
				StartBlock:        0,
				ExclusiveEndBlock: 300,
			},
			chunkSize: 100,

			expectedOutput: []*block.Range{{50, 100}, {100, 200}, {200, 300}},
		},
		{
			name:             "start at initial block",
			moduleStartBlock: 50,
			inputRange: &block.Range{
				StartBlock:        50,
				ExclusiveEndBlock: 300,
			},
			chunkSize: 100,

			expectedOutput: []*block.Range{{50, 100}, {100, 200}, {200, 300}},
		},
		{
			name:             "start after start block, on boundary",
			moduleStartBlock: 50,
			inputRange: &block.Range{
				StartBlock:        100,
				ExclusiveEndBlock: 300,
			},
			chunkSize: 100,

			expectedOutput: []*block.Range{{100, 200}, {200, 300}},
		},
		{
			name:             "start after start block, random block",
			moduleStartBlock: 50,
			inputRange: &block.Range{
				StartBlock:        127,
				ExclusiveEndBlock: 300,
			},
			chunkSize: 100,

			expectedOutput: []*block.Range{{127, 200}, {200, 300}},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			splitter := NewSplitter(tt.chunkSize)
			result := splitter.Split(tt.moduleStartBlock, tt.lastSavedBlock, tt.inputRange)

			require.Equal(t, tt.expectedOutput, result)
		})
	}
}

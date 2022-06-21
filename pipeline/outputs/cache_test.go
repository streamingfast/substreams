package outputs

import (
	"testing"

	"github.com/streamingfast/substreams/block"
	"github.com/stretchr/testify/require"
)

func TestOutputCache_listContinuousCacheRanges(t *testing.T) {
	testCases := []struct {
		name           string
		fromBlock      uint64
		cachedRanges   block.Ranges
		expectedOutput string
	}{
		{
			name:           "sunny path",
			cachedRanges:   block.Ranges{{100, 200}, {200, 300}, {300, 400}},
			expectedOutput: "[100, 200),[200, 300),[300, 400)",
		},
		{
			name:           "sunny path with from",
			fromBlock:      99,
			cachedRanges:   block.Ranges{{100, 200}, {200, 300}, {300, 400}},
			expectedOutput: "[100, 200),[200, 300),[300, 400)",
		},
		{
			name:           "one",
			cachedRanges:   block.Ranges{{100, 200}},
			expectedOutput: "[100, 200)",
		},
		{
			name:           "none",
			cachedRanges:   nil,
			expectedOutput: "",
		},
		{
			name:           "split",
			cachedRanges:   block.Ranges{{100, 200}, {200, 300}, {400, 500}},
			expectedOutput: "[100, 200),[200, 300)",
		},
		{
			name:           "split and from",
			fromBlock:      300,
			cachedRanges:   block.Ranges{{100, 200}, {300, 400}},
			expectedOutput: "[300, 400)",
		},
	}

	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			ranges := listContinuousCacheRanges(c.cachedRanges, c.fromBlock)
			result := ranges.String()
			require.Equal(t, c.expectedOutput, result)
		})
	}
}

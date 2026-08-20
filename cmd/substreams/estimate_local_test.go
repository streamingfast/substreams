package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCalculateChainSegments(t *testing.T) {
	tests := []struct {
		name        string
		startBlock  uint64
		endBlock    uint64
		numSegments int
		expected    []ChainSegment
	}{
		{
			name:        "basic 3 segments",
			startBlock:  125123,
			endBlock:    250145,
			numSegments: 3,
			expected: []ChainSegment{
				{
					SampledStartBlock: 126000,
					SampledEndBlock:   126999,
					ChainStartBlock:   125123,
					ChainEndBlock:     166797, // 125123 + (250145-125123)/3 = 125123 + 41674
				},
				{
					SampledStartBlock: 167000,
					SampledEndBlock:   167999,
					ChainStartBlock:   166797,
					ChainEndBlock:     208471, // 166797 + 41674
				},
				{
					SampledStartBlock: 209000,
					SampledEndBlock:   209999,
					ChainStartBlock:   208471,
					ChainEndBlock:     250145,
				},
			},
		},
		{
			name:        "aligned blocks",
			startBlock:  100000,
			endBlock:    200000,
			numSegments: 2,
			expected: []ChainSegment{
				{
					SampledStartBlock: 100000,
					SampledEndBlock:   100999,
					ChainStartBlock:   100000,
					ChainEndBlock:     150000,
				},
				{
					SampledStartBlock: 150000,
					SampledEndBlock:   150999,
					ChainStartBlock:   150000,
					ChainEndBlock:     200000,
				},
			},
		},
		{
			name:        "single segment",
			startBlock:  50000,
			endBlock:    60000,
			numSegments: 1,
			expected: []ChainSegment{
				{
					SampledStartBlock: 50000,
					SampledEndBlock:   50999,
					ChainStartBlock:   50000,
					ChainEndBlock:     60000,
				},
			},
		},
		{
			name:        "small range requiring rounding",
			startBlock:  1234,
			endBlock:    5678,
			numSegments: 2,
			expected: []ChainSegment{
				{
					SampledStartBlock: 2000,
					SampledEndBlock:   2999,
					ChainStartBlock:   1234,
					ChainEndBlock:     3456, // 1234 + (5678-1234)/2
				},
				{
					SampledStartBlock: 4000,
					SampledEndBlock:   4999,
					ChainStartBlock:   3456,
					ChainEndBlock:     5678,
				},
			},
		},
		{
			name:        "10 segments over large range",
			startBlock:  1000000,
			endBlock:    2000000,
			numSegments: 10,
			expected: []ChainSegment{
				{SampledStartBlock: 1000000, SampledEndBlock: 1000999, ChainStartBlock: 1000000, ChainEndBlock: 1100000},
				{SampledStartBlock: 1100000, SampledEndBlock: 1100999, ChainStartBlock: 1100000, ChainEndBlock: 1200000},
				{SampledStartBlock: 1200000, SampledEndBlock: 1200999, ChainStartBlock: 1200000, ChainEndBlock: 1300000},
				{SampledStartBlock: 1300000, SampledEndBlock: 1300999, ChainStartBlock: 1300000, ChainEndBlock: 1400000},
				{SampledStartBlock: 1400000, SampledEndBlock: 1400999, ChainStartBlock: 1400000, ChainEndBlock: 1500000},
				{SampledStartBlock: 1500000, SampledEndBlock: 1500999, ChainStartBlock: 1500000, ChainEndBlock: 1600000},
				{SampledStartBlock: 1600000, SampledEndBlock: 1600999, ChainStartBlock: 1600000, ChainEndBlock: 1700000},
				{SampledStartBlock: 1700000, SampledEndBlock: 1700999, ChainStartBlock: 1700000, ChainEndBlock: 1800000},
				{SampledStartBlock: 1800000, SampledEndBlock: 1800999, ChainStartBlock: 1800000, ChainEndBlock: 1900000},
				{SampledStartBlock: 1900000, SampledEndBlock: 1900999, ChainStartBlock: 1900000, ChainEndBlock: 2000000},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := calculateChainSegments(tt.startBlock, tt.endBlock, tt.numSegments)
			require.Equal(t, len(tt.expected), len(result), "number of segments mismatch")

			for i, expected := range tt.expected {
				actual := result[i]
				assert.Equal(t, expected.SampledStartBlock, actual.SampledStartBlock, "segment %d: TestStartBlock mismatch", i)
				assert.Equal(t, expected.SampledEndBlock, actual.SampledEndBlock, "segment %d: TestEndBlock mismatch", i)
				assert.Equal(t, expected.ChainStartBlock, actual.ChainStartBlock, "segment %d: ChainStartBlock mismatch", i)
				assert.Equal(t, expected.ChainEndBlock, actual.ChainEndBlock, "segment %d: ChainEndBlock mismatch", i)

				// Verify 1000-block boundaries
				assert.Equal(t, uint64(0), actual.SampledStartBlock%1000, "segment %d: TestStartBlock not on 1000-block boundary", i)

				// Verify test range is at most 1000 blocks
				testRange := actual.SampledEndBlock - actual.SampledStartBlock + 1
				assert.LessOrEqual(t, testRange, uint64(1000), "segment %d: test range exceeds 1000 blocks", i)
			}
		})
	}
}

func TestCalculateChainSegmentsEdgeCases(t *testing.T) {
	t.Run("zero segments", func(t *testing.T) {
		result := calculateChainSegments(100000, 200000, 0)
		assert.Len(t, result, 1, "should default to 1 segment when 0 requested")
	})

	t.Run("negative segments", func(t *testing.T) {
		result := calculateChainSegments(100000, 200000, -5)
		assert.Len(t, result, 1, "should default to 1 segment when negative requested")
	})

	t.Run("equal start and end", func(t *testing.T) {
		result := calculateChainSegments(100000, 100000, 5)
		assert.Len(t, result, 0, "should return empty when start equals end")
	})

	t.Run("very small range", func(t *testing.T) {
		result := calculateChainSegments(100, 200, 5)
		// Should handle gracefully, possibly fewer segments than requested
		for i, seg := range result {
			assert.LessOrEqual(t, seg.SampledStartBlock, seg.SampledEndBlock, "segment %d: start should be <= end", i)
			assert.Equal(t, uint64(0), seg.SampledStartBlock%1000, "segment %d: should be on 1000-block boundary", i)
		}
	})
}

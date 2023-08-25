package ranges

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_MergeRangeLists(t *testing.T) {
	tests := []struct {
		name                      string
		completedBlockRanges      []*BlockRange
		newlyCompletedBlockRanges []*BlockRange
		expectedBlockRanges       []*BlockRange
	}{
		{
			name: "Nothing new",
			completedBlockRanges: []*BlockRange{
				{Start: 0, End: 99},
				{Start: 100, End: 199},
			},
			newlyCompletedBlockRanges: []*BlockRange{},
			expectedBlockRanges: []*BlockRange{
				{Start: 0, End: 99},
				{Start: 100, End: 199},
			},
		},
		{
			name: "Merging 1 block range",
			completedBlockRanges: []*BlockRange{
				{Start: 0, End: 99},
			},
			newlyCompletedBlockRanges: []*BlockRange{
				{Start: 100, End: 199},
			},
			expectedBlockRanges: []*BlockRange{
				{Start: 0, End: 199},
			},
		},
		{
			name: "Merging multiple block ranges",
			completedBlockRanges: []*BlockRange{
				{Start: 0, End: 99},
			},
			newlyCompletedBlockRanges: []*BlockRange{
				{Start: 100, End: 199},
				{Start: 200, End: 299},
				{Start: 400, End: 499},
				{Start: 500, End: 599},
			},
			expectedBlockRanges: []*BlockRange{
				{Start: 0, End: 299},
				{Start: 400, End: 599},
			},
		},
		{
			name: "Merging multiple block ranges and reduce overlaps",
			completedBlockRanges: []*BlockRange{
				{Start: 0, End: 99},
			},
			newlyCompletedBlockRanges: []*BlockRange{
				{Start: 100, End: 199},
				{Start: 200, End: 299},
				{Start: 400, End: 499},
				{Start: 500, End: 599},
				{Start: 300, End: 399},
			},
			expectedBlockRanges: []*BlockRange{
				{Start: 0, End: 599},
			},
		},
		{
			name: "Badly overlapping",
			completedBlockRanges: []*BlockRange{
				{Start: 0, End: 199},
				{Start: 0, End: 99},
			},
			newlyCompletedBlockRanges: []*BlockRange{
				{Start: 100, End: 220},
			},
			expectedBlockRanges: []*BlockRange{
				{Start: 0, End: 220},
			},
		},
		{
			name: "Unmerging range",
			completedBlockRanges: []*BlockRange{
				{Start: 10000, End: 19998},
				{Start: 20000, End: 37999},
			},
			newlyCompletedBlockRanges: []*BlockRange{
				{Start: 10000, End: 19999},
			},
			expectedBlockRanges: []*BlockRange{
				{Start: 10000, End: 37999},
			},
		},
		{
			name: "Preemptive overlap",
			completedBlockRanges: []*BlockRange{
				{Start: 6950000, End: 6959999},
				{Start: 6990000, End: 6999997},
				{Start: 6990000, End: 6992496},
				{Start: 7000000, End: 7009999},
			},
			newlyCompletedBlockRanges: []*BlockRange{
				{Start: 6990000, End: 6999998},
				{Start: 6990000, End: 6999999},
			},
			expectedBlockRanges: []*BlockRange{
				{Start: 6950000, End: 6959999},
				{Start: 6990000, End: 7009999},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fmt.Println("Executing test", test.name)
			actual := test.completedBlockRanges
			for _, br := range test.newlyCompletedBlockRanges {
				actual = mergeRangeLists(actual, br)
			}
			require.Equal(t, test.expectedBlockRanges, actual)
		})
	}
}

func TestReduce1(t *testing.T) {
	res := reduceOverlaps([]*BlockRange{
		{Start: 6990000, End: 6999997},
		{Start: 6990000, End: 6992496}, // Happens if a process failed and restarted, will re-send lower level, which might appear as stalled, but it's actually re-working.
		{Start: 7000000, End: 7009999},
	})
	assert.Equal(t, ranges{
		{Start: 6990000, End: 6999997},
		{Start: 7000000, End: 7009999},
	}.String(), res.String())
}

func TestReduce2(t *testing.T) {
	res := reduceOverlaps([]*BlockRange{
		{Start: 6990000, End: 6999997},
		{Start: 6990000, End: 6992496},
	})
	assert.Equal(t, ranges{
		{Start: 6990000, End: 6999997},
	}.String(), res.String())
}

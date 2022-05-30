package tui

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_MergeRangeLists(t *testing.T) {
	tests := []struct {
		name                      string
		completedBlockRanges      []*blockRange
		newlyCompletedBlockRanges []*blockRange
		expectedBlockRanges       []*blockRange
	}{
		{
			name: "Nothing new",
			completedBlockRanges: []*blockRange{
				{Start: 0, End: 99},
				{Start: 100, End: 199},
			},
			newlyCompletedBlockRanges: []*blockRange{},
			expectedBlockRanges: []*blockRange{
				{Start: 0, End: 99},
				{Start: 100, End: 199},
			},
		},
		{
			name: "Merging 1 block range",
			completedBlockRanges: []*blockRange{
				{Start: 0, End: 99},
			},
			newlyCompletedBlockRanges: []*blockRange{
				{Start: 100, End: 199},
			},
			expectedBlockRanges: []*blockRange{
				{Start: 0, End: 199},
			},
		},
		{
			name: "Merging multiple block ranges",
			completedBlockRanges: []*blockRange{
				{Start: 0, End: 99},
			},
			newlyCompletedBlockRanges: []*blockRange{
				{Start: 100, End: 199},
				{Start: 200, End: 299},
				{Start: 400, End: 499},
				{Start: 500, End: 599},
			},
			expectedBlockRanges: []*blockRange{
				{Start: 0, End: 299},
				{Start: 400, End: 599},
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

func TestLineBar(t *testing.T) {
	res := linebar(
		ranges{
			{Start: 1, End: 23},
			{Start: 50, End: 100},
			{Start: 200, End: 600},
			{Start: 700, End: 710},
		},
		100,
		1000,
		10,
	)
	fmt.Println("MAMA", res)
}

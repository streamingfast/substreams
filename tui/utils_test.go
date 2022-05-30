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
				{Start: 0, End: 100},
				{Start: 100, End: 200},
			},
			newlyCompletedBlockRanges: []*blockRange{},
			expectedBlockRanges: []*blockRange{
				{Start: 0, End: 100},
				{Start: 100, End: 200},
			},
		},
		{
			name: "Merging 1 block range",
			completedBlockRanges: []*blockRange{
				{Start: 0, End: 100},
			},
			newlyCompletedBlockRanges: []*blockRange{
				{Start: 100, End: 200},
			},
			expectedBlockRanges: []*blockRange{
				{Start: 0, End: 200},
			},
		},
		{
			name: "Merging multiple block ranges",
			completedBlockRanges: []*blockRange{
				{Start: 0, End: 100},
			},
			newlyCompletedBlockRanges: []*blockRange{
				{Start: 100, End: 200},
				{Start: 200, End: 300},
				{Start: 400, End: 500},
				{Start: 500, End: 600},
			},
			expectedBlockRanges: []*blockRange{
				{Start: 0, End: 300},
				{Start: 400, End: 600},
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

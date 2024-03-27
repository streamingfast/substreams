package service

import (
	"testing"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/test-go/testify/require"
)

func TestSortClocksDistributor(t *testing.T) {
	cases := []struct {
		name           string
		clocksMap      map[uint64]*pbsubstreams.Clock
		expectedResult []*pbsubstreams.Clock
	}{
		{
			name: "sunny path",
			clocksMap: map[uint64]*pbsubstreams.Clock{
				2: {Number: 2, Id: "test2"},
				3: {Number: 3, Id: "test3"},
				1: {Number: 1, Id: "test1"},
			},
			expectedResult: []*pbsubstreams.Clock{
				{Number: 1, Id: "test1"},
				{Number: 2, Id: "test2"},
				{Number: 3, Id: "test3"},
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			result := sortClocksDistributor(c.clocksMap)
			require.Equal(t, c.expectedResult, result)
		})
	}
}

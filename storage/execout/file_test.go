package execout

import (
	"testing"

	pboutput "github.com/streamingfast/substreams/storage/execout/pb"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/stretchr/testify/require"
)

func TestExtractClocks(t *testing.T) {
	cases := []struct {
		name              string
		file              File
		clocksDistributor map[uint64]*pbsubstreams.Clock
		expectedResult    map[uint64]*pbsubstreams.Clock
	}{
		{
			name: "sunny path",
			file: File{
				ModuleName: "sunny_path",
				kv:         map[string]*pboutput.Item{"id1": {BlockNum: 1, BlockId: "1"}, "id2": {BlockNum: 2, BlockId: "3"}},
			},
			clocksDistributor: map[uint64]*pbsubstreams.Clock{},
			expectedResult:    map[uint64]*pbsubstreams.Clock{1: {Number: 1, Id: "1"}, 2: {Number: 2, Id: "3"}},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			c.file.ExtractClocks(c.clocksDistributor)
			require.Equal(t, c.expectedResult, c.clocksDistributor)
		})
	}
}

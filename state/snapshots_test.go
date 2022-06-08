package state

import (
	"testing"

	"github.com/test-go/testify/require"

	"github.com/streamingfast/substreams/block"
)

func TestSnapshots_LastBlock(t *testing.T) {
	snapshosts := &Snapshots{
		Files: []Snapshot{
			{
				Range: block.Range{
					StartBlock:        100,
					ExclusiveEndBlock: 200,
				},
				Path:    "",
				Partial: false,
			},
			{
				Range: block.Range{
					StartBlock:        200,
					ExclusiveEndBlock: 300,
				},
				Path:    "",
				Partial: false,
			},
			{
				Range: block.Range{
					StartBlock:        300,
					ExclusiveEndBlock: 400,
				},
				Path:    "",
				Partial: true,
			},
		},
	}
	lastBlock := snapshosts.LastBlock()
	require.Equal(t, uint64(300), lastBlock)
}

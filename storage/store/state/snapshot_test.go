package state

import (
	"testing"

	"github.com/streamingfast/substreams/storage/store"
	"github.com/stretchr/testify/assert"
)

func TestSnapshots_LastCompleted(t *testing.T) {
	assert.Equal(t, 300, int((&storeSnapshots{
		Completes: store.CompleteFiles("100-200,100-300"),
		Partials:  store.PartialFiles("300-400"),
	}).LastCompletedBlock()))

	assert.Equal(t, 0, int((&storeSnapshots{
		Completes: store.CompleteFiles(""),
		Partials:  store.PartialFiles("200-300"),
	}).LastCompletedBlock()))
}

func TestSnapshots_LastCompleteBefore(t *testing.T) {
	tests := []struct {
		name         string
		snapshot     *storeSnapshots
		blockNum     uint64
		expectBrange *store.FileInfo
	}{
		{
			name: "no complete range covering block",
			snapshot: &storeSnapshots{
				Completes: store.CompleteFiles("10-20,10-50,10-1000"),
			},
			blockNum:     0,
			expectBrange: nil,
		},
		{
			name: "no complete range covering block",
			snapshot: &storeSnapshots{
				Completes: store.CompleteFiles("10-20,10-50,10-1000"),
			},
			blockNum:     19,
			expectBrange: nil,
		},
		{
			name: "complete range ending on block",
			snapshot: &storeSnapshots{
				Completes: store.CompleteFiles("10-20,10-50,10-1000"),
			},
			blockNum:     20,
			expectBrange: store.CompleteFile("10-20"),
		},
		{
			name: "complete range ending just before lookup block",
			snapshot: &storeSnapshots{
				Completes: store.CompleteFiles("10-20,10-50,10-1000"),
			},
			blockNum:     21,
			expectBrange: store.CompleteFile("10-20"),
		},
		{
			name: "complete range ending before lookup block",
			snapshot: &storeSnapshots{
				Completes: store.CompleteFiles("10-20,10-50,10-1000"),
			},
			blockNum:     49,
			expectBrange: store.CompleteFile("10-20"),
		},
		{
			name: "better complete range ending on block",
			snapshot: &storeSnapshots{
				Completes: store.CompleteFiles("10-20,10-50,10-1000"),
			},
			blockNum:     50,
			expectBrange: store.CompleteFile("10-50"),
		},
		{
			name: "another test 1",
			snapshot: &storeSnapshots{
				Completes: store.CompleteFiles("10-20,10-50,10-1000"),
			},
			blockNum:     51,
			expectBrange: store.CompleteFile("10-50"),
		},
		{
			name: "another test 2",
			snapshot: &storeSnapshots{
				Completes: store.CompleteFiles("10-20,10-50,10-1000"),
			},
			blockNum:     1003,
			expectBrange: store.CompleteFile("10-1000"),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			blockRange := test.snapshot.LastCompleteSnapshotBefore(test.blockNum)
			assert.Equal(t, test.expectBrange, blockRange)
		})
	}
}

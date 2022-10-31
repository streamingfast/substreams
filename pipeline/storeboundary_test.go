package pipeline

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestNew(t *testing.T) {
	tests := []struct {
		name                string
		isSubRequest        bool
		blockNum            uint64
		reqStopBlock        uint64
		currentBoundary     uint64
		expectedFlushRanges []uint64
	}{
		// Request without Stop blocks
		{
			name:                "request, receive block pre boundary",
			blockNum:            9,
			currentBoundary:     10,
			expectedFlushRanges: []uint64{},
		},
		{
			name:                "request, receive block on boundary",
			blockNum:            30,
			currentBoundary:     30,
			expectedFlushRanges: []uint64{30},
		},
		{
			name:                "request, receive block post boundary",
			blockNum:            25,
			currentBoundary:     20,
			expectedFlushRanges: []uint64{20},
		},
		{
			name:                "request, receive block much past boundary",
			blockNum:            58,
			currentBoundary:     20,
			expectedFlushRanges: []uint64{20, 30, 40, 50},
		},
		// Request with Stop blocks
		{
			name:                "request, hit stop block pre boundary",
			blockNum:            9,
			reqStopBlock:        9,
			currentBoundary:     10,
			expectedFlushRanges: []uint64{},
		},
		{
			name:                "request, hit stop block on boundary",
			blockNum:            30,
			reqStopBlock:        30,
			currentBoundary:     30,
			expectedFlushRanges: []uint64{30},
		},
		{
			name:                "request, passed stop block post boundary",
			blockNum:            25,
			reqStopBlock:        22,
			currentBoundary:     20,
			expectedFlushRanges: []uint64{20},
		},
		{
			name:            "request, passed stop blockmuch past boundary",
			blockNum:        58,
			reqStopBlock:    22,
			currentBoundary: 20,
			//expectedFlushRanges: []uint64{20, 30, 40, 50},
			expectedFlushRanges: []uint64{20},
		},
		// Subrequest
		{
			name:                "request, receive block pre boundary",
			isSubRequest:        true,
			blockNum:            9,
			reqStopBlock:        30, // has no impact on the flow
			currentBoundary:     10,
			expectedFlushRanges: []uint64{},
		},
		{
			name:                "request, receive block on boundary",
			isSubRequest:        true,
			blockNum:            30,
			reqStopBlock:        42, // has no impact on the flow
			currentBoundary:     30,
			expectedFlushRanges: []uint64{30},
		},
		{
			name:                "request, receive block post boundary",
			isSubRequest:        true,
			blockNum:            25,
			reqStopBlock:        45, // has no impact on the flow
			currentBoundary:     20,
			expectedFlushRanges: []uint64{20},
		},
		{
			name:                "request, receive block much past boundary",
			isSubRequest:        true,
			blockNum:            58,
			reqStopBlock:        76, // has no impact on the flow
			currentBoundary:     20,
			expectedFlushRanges: []uint64{20, 30, 40, 50},
		},
		{
			name:                "request, hit stop block pre boundary",
			isSubRequest:        true,
			blockNum:            18,
			reqStopBlock:        18,
			currentBoundary:     20,
			expectedFlushRanges: []uint64{18},
		},
		{
			name:                "request, hit stop block on boundary",
			isSubRequest:        true,
			blockNum:            30,
			reqStopBlock:        30,
			currentBoundary:     30,
			expectedFlushRanges: []uint64{30},
		},
		{
			name:                "request, hit stop block post boundary",
			isSubRequest:        true,
			blockNum:            22,
			reqStopBlock:        22,
			currentBoundary:     20,
			expectedFlushRanges: []uint64{20, 22},
		},
		{
			name:                "request, pass stop block post boundary",
			isSubRequest:        true,
			blockNum:            36,
			reqStopBlock:        34,
			currentBoundary:     20,
			expectedFlushRanges: []uint64{20, 30, 34},
		},
		{
			name:                "request, passed stop blockmuch past boundary",
			isSubRequest:        true,
			blockNum:            58,
			reqStopBlock:        22,
			currentBoundary:     20,
			expectedFlushRanges: []uint64{20, 22},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			b := &storeBoundary{
				interval:     10,
				nextBoundary: test.currentBoundary,
			}
			assert.Equal(t, test.expectedFlushRanges, b.GetStoreFlushRanges(test.isSubRequest, test.reqStopBlock, test.blockNum))
		})
	}
}

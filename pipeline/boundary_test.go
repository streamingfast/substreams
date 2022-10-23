package pipeline

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestNew(t *testing.T) {
	tests := []struct {
		name            string
		isSubrequest    bool
		blockNum        uint64
		reqStopBlock    uint64
		currentBoundary uint64
		expectBlocks    []uint64
	}{
		// Request without Stop blocks
		{
			name:            "request, receive block pre boundary",
			blockNum:        9,
			currentBoundary: 10,
			expectBlocks:    []uint64{},
		},
		{
			name:            "request, receive block on boundary",
			blockNum:        30,
			currentBoundary: 30,
			expectBlocks:    []uint64{30},
		},
		{
			name:            "request, receive block post boundary",
			blockNum:        25,
			currentBoundary: 20,
			expectBlocks:    []uint64{20},
		},
		{
			name:            "request, receive block much past boundary",
			blockNum:        58,
			currentBoundary: 20,
			expectBlocks:    []uint64{20, 30, 40, 50},
		},
		// Request with Stop blocks
		{
			name:            "request, hit stop block pre boundary",
			blockNum:        9,
			reqStopBlock:    9,
			currentBoundary: 10,
			expectBlocks:    []uint64{},
		},
		{
			name:            "request, hit stop block on boundary",
			blockNum:        30,
			reqStopBlock:    30,
			currentBoundary: 30,
			expectBlocks:    []uint64{30},
		},
		{
			name:            "request, passed stop block post boundary",
			blockNum:        25,
			reqStopBlock:    22,
			currentBoundary: 20,
			expectBlocks:    []uint64{20},
		},
		{
			name:            "request, passed stop blockmuch past boundary",
			blockNum:        58,
			reqStopBlock:    22,
			currentBoundary: 20,
			expectBlocks:    []uint64{20, 30, 40, 50},
		},
		// Subrequest
		{
			name:            "request, receive block pre boundary",
			isSubrequest:    true,
			blockNum:        9,
			reqStopBlock:    30, // has no impact on the flow
			currentBoundary: 10,
			expectBlocks:    []uint64{},
		},
		{
			name:            "request, receive block on boundary",
			isSubrequest:    true,
			blockNum:        30,
			reqStopBlock:    42, // has no impact on the flow
			currentBoundary: 30,
			expectBlocks:    []uint64{30},
		},
		{
			name:            "request, receive block post boundary",
			isSubrequest:    true,
			blockNum:        25,
			reqStopBlock:    45, // has no impact on the flow
			currentBoundary: 20,
			expectBlocks:    []uint64{20},
		},
		{
			name:            "request, receive block much past boundary",
			isSubrequest:    true,
			blockNum:        58,
			reqStopBlock:    76, // has no impact on the flow
			currentBoundary: 20,
			expectBlocks:    []uint64{20, 30, 40, 50},
		},
		{
			name:            "request, hit stop block pre boundary",
			isSubrequest:    true,
			blockNum:        18,
			reqStopBlock:    18,
			currentBoundary: 20,
			expectBlocks:    []uint64{18},
		},
		{
			name:            "request, hit stop block on boundary",
			isSubrequest:    true,
			blockNum:        30,
			reqStopBlock:    30,
			currentBoundary: 30,
			expectBlocks:    []uint64{30},
		},
		{
			name:            "request, hit stop block post boundary",
			isSubrequest:    true,
			blockNum:        22,
			reqStopBlock:    22,
			currentBoundary: 20,
			expectBlocks:    []uint64{20, 22},
		},
		{
			name:            "request, pass stop block post boundary",
			isSubrequest:    true,
			blockNum:        36,
			reqStopBlock:    34,
			currentBoundary: 20,
			expectBlocks:    []uint64{20, 30, 34},
		},
		{
			name:            "request, passed stop blockmuch past boundary",
			isSubrequest:    true,
			blockNum:        58,
			reqStopBlock:    22,
			currentBoundary: 20,
			expectBlocks:    []uint64{20, 22},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			b := &StoreBoundary{
				interval:     10,
				nextBoundary: test.currentBoundary,
			}
			assert.Equal(t, test.expectBlocks, b.GetStoreFlushRanges(test.isSubrequest, test.reqStopBlock, test.blockNum))
		})
	}
}

func Test_StoreBoundary(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		isSubrequest bool
		reqStart     uint64
		reqStop      uint64
		blockSkip    int // default 1
		expectSaves  []int
	}{
		{
			name:         "subreq, should flush because stop block",
			isSubrequest: true,
			reqStart:     80,
			reqStop:      90,
			expectSaves:  []int{90},
		},
		{
			name:         "no subreq, should NOT flush because stop block",
			isSubrequest: false,
			reqStart:     80,
			reqStop:      90,
			expectSaves:  []int{90},
		},
		{
			name:         "subreq, current on next boundary",
			isSubrequest: true,
			reqStart:     80,
			reqStop:      95,
			expectSaves:  []int{90, 95},
		},
		{
			name:         "no subreq, current 5+ next boundary",
			isSubrequest: false,
			reqStart:     80,
			reqStop:      95,
			expectSaves:  []int{90},
		},
		{
			name:         "subreq, reqStart off bounds, current one off bound",
			isSubrequest: true,
			reqStart:     85,
			reqStop:      95,
			expectSaves:  []int{90, 95},
		},
		{
			name:         "no subreq, reqStart off bounds, current one off bound",
			isSubrequest: false,
			reqStart:     85,
			reqStop:      95,
			expectSaves:  []int{90},
		},
		// Block skips 2
		{
			name:         "skip 2, subreq, should flush because stop block",
			isSubrequest: true,
			reqStart:     80,
			reqStop:      90,
			blockSkip:    2,
			expectSaves:  []int{90},
		},
		{
			name:         "skip 2, no subreq, should NOT flush because stop block",
			isSubrequest: false,
			reqStart:     80,
			reqStop:      90,
			blockSkip:    2,
			expectSaves:  []int{90},
		},
		{
			name:         "skip 2, subreq, current on next boundary",
			isSubrequest: true,
			reqStart:     80,
			reqStop:      95,
			blockSkip:    2,
			expectSaves:  []int{90, 95},
		},
		{
			name:         "skip 2, no subreq, current 5+ next boundary",
			isSubrequest: false,
			reqStart:     80,
			reqStop:      95,
			blockSkip:    2,
			expectSaves:  []int{90},
		},
		{
			name:         "skip 3, subreq, reqStart off bounds, current one off bound",
			isSubrequest: true,
			reqStart:     85,
			reqStop:      95,
			blockSkip:    3,
			expectSaves:  []int{90, 95},
		},
		{
			name:         "skip 3, no subreq, reqStart off bounds, current one off bound",
			isSubrequest: false,
			reqStart:     85,
			reqStop:      95,
			blockSkip:    3,
			expectSaves:  []int{90},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			b := &StoreBoundary{
				interval: 10,
			}

			b.InitBoundary(test.reqStart)

			blockSkip := test.blockSkip
			if blockSkip == 0 {
				blockSkip = 1
			}

			var res []int
			for blockNum := test.reqStart; blockNum < test.reqStop+5; blockNum += uint64(blockSkip) {
				subrequestStopBlock := test.isSubrequest && isStopBlockReached(blockNum, test.reqStop)
				for b.OverBoundary(blockNum) || subrequestStopBlock {

					boundaryBlock := b.Boundary()
					if subrequestStopBlock {
						boundaryBlock = test.reqStop
					}

					res = append(res, int(boundaryBlock))
					b.BumpBoundary()
					if isStopBlockReached(blockNum, test.reqStop) {
						break
					}
				}
				if isStopBlockReached(blockNum, test.reqStop) {
					break
				}
			}

			assert.Equal(t, test.expectSaves, res)
		})
	}
}

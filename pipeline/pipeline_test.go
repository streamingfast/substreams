package pipeline

import (
	"testing"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/stretchr/testify/assert"
)

func TestStoreSaveBoundaries(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		isSubrequest bool
		fromBlock    uint64
		reqStop      uint64
		expectNext   uint64
	}{
		{
			name:         "from block on boundary, skip to next",
			isSubrequest: true,
			fromBlock:    80,
			reqStop:      90,
			expectNext:   90,
		},
		{
			name:         "from block on boundary, skip to next, stop on boundary",
			isSubrequest: true,
			fromBlock:    80,
			reqStop:      91,
			expectNext:   90,
		},
		{
			name:         "no subreq, from block on boundary, skip to next, stop off boundary",
			isSubrequest: false,
			fromBlock:    80,
			reqStop:      91,
			expectNext:   90,
		},
		{
			name:         "no subreq, from block on boundary, skip to next, stop on boundary",
			isSubrequest: false,
			fromBlock:    80,
			reqStop:      90,
			expectNext:   90,
		},
		{
			name:         "no subreq, from block on boundary, skip to next, stop below boundary",
			isSubrequest: false,
			fromBlock:    80,
			reqStop:      89,
			expectNext:   90,
		},
		{
			name:         "from block on boundary, skip to next, stop below boundary",
			isSubrequest: true,
			fromBlock:    80,
			reqStop:      89,
			expectNext:   89,
		},
		{
			name:         "from block off boundary, skip to next, stop below boundary",
			isSubrequest: true,
			fromBlock:    82,
			reqStop:      89,
			expectNext:   89,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			p := &Pipeline{
				isSubrequest:      test.isSubrequest,
				storeSaveInterval: 10,
				request: &pbsubstreams.Request{
					StopBlockNum: test.reqStop,
				},
			}

			res := p.computeNextStoreSaveBoundary(test.fromBlock)

			assert.Equal(t, int(test.expectNext), int(res))
		})
	}
}

func TestBump(t *testing.T) {
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
			p := &Pipeline{
				isSubrequest:           test.isSubrequest,
				storeSaveInterval:      10,
				requestedStartBlockNum: test.reqStart,
				request: &pbsubstreams.Request{
					StopBlockNum: test.reqStop,
				},
			}

			p.initStoreSaveBoundary()

			blockSkip := test.blockSkip
			if blockSkip == 0 {
				blockSkip = 1
			}

			var res []int
			for blockNum := test.reqStart; blockNum < test.reqStop+5; blockNum += uint64(blockSkip) {
				//fmt.Println("Block", blockNum)
				for p.nextStoreSaveBoundary <= uint64(blockNum) {
					res = append(res, int(p.nextStoreSaveBoundary))
					p.bumpStoreSaveBoundary()
					if isStopBlockReached(uint64(blockNum), test.reqStop) {
						break
					}
				}
				if isStopBlockReached(uint64(blockNum), test.reqStop) {
					break
				}
			}

			assert.Equal(t, test.expectSaves, res)
		})
	}
}

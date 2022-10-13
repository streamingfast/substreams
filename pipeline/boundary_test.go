package pipeline

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

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
				for b.PassedBoundary(blockNum) || subrequestStopBlock {

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

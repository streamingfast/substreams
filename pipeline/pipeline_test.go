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

package service

import (
	"testing"

	"github.com/test-go/testify/assert"
)

func TestRoundToBundleFinalBlock(t *testing.T) {
	cases := []struct {
		name                 string
		finalBlockNum        uint64
		bundleSize           uint64
		firstStreamableBlock uint64
		expected             uint64
	}{
		{"parity with previous behavior at 100", 12345, 100, 0, 12200},
		{"exact boundary at 100", 12300, 100, 0, 12200},
		{"one full bundle behind rounds to previous", 150, 100, 0, 0},
		{"bundle size 1000", 12345, 1000, 0, 11000},
		{"exact boundary at 1000", 12000, 1000, 0, 11000},
		{"one full bundle behind at 1000 rounds to previous", 1500, 1000, 0, 0},
		{"first complete bundle rounds down to first streamable block", 10150, 100, 10000, 10000},
		{"above first complete bundle with first streamable block", 10345, 100, 10000, 10200},
		{"less than one bundle final is untouched", 10050, 100, 10000, 10050},
		{"result floored at unaligned first streamable block", 10250, 100, 10150, 10150},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.expected, roundToBundleFinalBlock(c.finalBlockNum, c.bundleSize, c.firstStreamableBlock))
		})
	}
}

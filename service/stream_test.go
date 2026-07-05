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
		{"below guard at 100 is untouched", 150, 100, 0, 150},
		{"bundle size 1000", 12345, 1000, 0, 11000},
		{"exact boundary at 1000", 12000, 1000, 0, 11000},
		{"below guard at 1000 is untouched", 1500, 1000, 0, 1500},
		{"guard is relative to first streamable block", 10150, 100, 10000, 10150},
		{"above guard with first streamable block", 10345, 100, 10000, 10200},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.expected, roundToBundleFinalBlock(c.finalBlockNum, c.bundleSize, c.firstStreamableBlock))
		})
	}
}

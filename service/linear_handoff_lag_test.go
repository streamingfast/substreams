package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLinearHandoffLagTarget(t *testing.T) {
	tests := []struct {
		name           string
		linearHandoff  uint64
		stopBlock      uint64
		lastFinalBlock uint64
		maxLagSegments uint64
		expectTarget   uint64
		expectTooFar   bool
	}{
		{"final block did not move", 1000, 0, 1050, 2, 1000, false},
		{"exactly max lag", 1000, 0, 1250, 2, 1200, false},
		{"one segment over max lag", 1000, 0, 1300, 2, 1300, true},
		{"stop block caps the target", 1000, 1150, 5000, 2, 1200, false},
		{"stop block above lag", 1000, 1301, 5000, 2, 1400, true},
		{"final block below handoff", 1000, 0, 900, 2, 900, false},
		{"zero max lag", 1000, 0, 1100, 0, 1100, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target, tooFar := linearHandoffLagTarget(tt.linearHandoff, tt.stopBlock, tt.lastFinalBlock, 100, tt.maxLagSegments)
			assert.Equal(t, tt.expectTarget, target)
			assert.Equal(t, tt.expectTooFar, tooFar)
		})
	}
}

func TestParseMaxLinearHandoffLagSegments(t *testing.T) {
	assert.Equal(t, uint64(defaultMaxLinearHandoffLagSegments), parseMaxLinearHandoffLagSegments(""))
	assert.Equal(t, uint64(5), parseMaxLinearHandoffLagSegments("5"))
	assert.Panics(t, func() { parseMaxLinearHandoffLagSegments("abc") })
}

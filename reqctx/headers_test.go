package reqctx

import (
	"context"
	"net/http"
	"testing"

	"github.com/streamingfast/dauth"
	"github.com/stretchr/testify/assert"
)

func TestGetEffectiveHeaderValues(t *testing.T) {
	tests := []struct {
		name                   string
		trustedHeaders         dauth.TrustedHeaders
		normalHeaders          http.Header
		defaultParallelWorkers uint64
		defaultStageExecutors  uint64
		expected               EffectiveParallelism
	}{
		{
			name:                   "uses defaults when no headers provided",
			trustedHeaders:         nil,
			normalHeaders:          http.Header{},
			defaultParallelWorkers: 10,
			defaultStageExecutors:  5,
			expected: EffectiveParallelism{
				RequestedWorkers:    0,
				GrantedWorkers:      10,
				Workers:             10,
				WorkersSource:       WorkersSourceDefault,
				PlanTier:            "",
				StageLayerExecutors: 5,
			},
		},
		{
			name: "trusted headers override defaults",
			trustedHeaders: dauth.TrustedHeaders{
				HeaderParallelWorkers: "20",
			},
			normalHeaders:          http.Header{},
			defaultParallelWorkers: 10,
			defaultStageExecutors:  5,
			expected: EffectiveParallelism{
				RequestedWorkers:    0,
				GrantedWorkers:      20,
				Workers:             20,
				WorkersSource:       WorkersSourceTrustedHeader,
				PlanTier:            "",
				StageLayerExecutors: 2, // default for free
			},
		},
		{
			name:           "normal headers can only lower values",
			trustedHeaders: nil,
			normalHeaders: http.Header{
				http.CanonicalHeaderKey(HeaderParallelWorkers): []string{"5"}, // lower than default 10
			},
			defaultParallelWorkers: 10,
			defaultStageExecutors:  5,
			expected: EffectiveParallelism{
				RequestedWorkers:    5,
				GrantedWorkers:      10,
				Workers:             5, // lowered
				WorkersSource:       WorkersSourceClientHeader,
				PlanTier:            "",
				StageLayerExecutors: 5, // unchanged (cannot increase)
			},
		},
		{
			name: "normal headers cannot override trusted headers upward",
			trustedHeaders: dauth.TrustedHeaders{
				HeaderParallelWorkers:          "20",
				dauth.HeaderSubstreamsPlanTier: "PRO",
			},
			normalHeaders: http.Header{
				http.CanonicalHeaderKey(HeaderParallelWorkers): []string{"15"}, // lower than trusted 20
			},
			defaultParallelWorkers: 10,
			defaultStageExecutors:  5,
			expected: EffectiveParallelism{
				RequestedWorkers:    15,
				GrantedWorkers:      20,
				Workers:             15, // lowered from trusted value
				WorkersSource:       WorkersSourceClientHeader,
				PlanTier:            "PRO",
				StageLayerExecutors: 8, // increased from the "PRO" plan
			},
		},
		{
			name: "client asking for more than granted keeps the granted count",
			trustedHeaders: dauth.TrustedHeaders{
				HeaderParallelWorkers:          "15",
				dauth.HeaderSubstreamsPlanTier: "SCALING",
			},
			normalHeaders: http.Header{
				http.CanonicalHeaderKey(HeaderParallelWorkers): []string{"300"},
			},
			defaultParallelWorkers: 10,
			defaultStageExecutors:  5,
			expected: EffectiveParallelism{
				RequestedWorkers:    300,
				GrantedWorkers:      15,
				Workers:             15,
				WorkersSource:       WorkersSourceTrustedHeader,
				PlanTier:            "SCALING",
				StageLayerExecutors: 5,
			},
		},
		{
			name:           "legacy client header is still honored",
			trustedHeaders: nil,
			normalHeaders: http.Header{
				http.CanonicalHeaderKey(legacyHeaderParallelWorkers): []string{"3"},
			},
			defaultParallelWorkers: 10,
			defaultStageExecutors:  5,
			expected: EffectiveParallelism{
				RequestedWorkers:    3,
				GrantedWorkers:      10,
				Workers:             3,
				WorkersSource:       WorkersSourceClientHeader,
				PlanTier:            "",
				StageLayerExecutors: 5,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			if tt.trustedHeaders != nil {
				ctx = dauth.WithTrustedHeaders(ctx, tt.trustedHeaders)
			}

			actual := GetEffectiveHeaderValues(
				ctx,
				tt.normalHeaders,
				tt.defaultParallelWorkers,
				tt.defaultStageExecutors,
			)

			assert.Equal(t, tt.expected, actual)
		})
	}
}

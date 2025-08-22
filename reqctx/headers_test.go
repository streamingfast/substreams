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
		name                    string
		trustedHeaders          dauth.TrustedHeaders
		normalHeaders           http.Header
		defaultParallelWorkers  uint64
		defaultStageExecutors   uint64
		expectedParallelWorkers uint64
		expectedStageExecutors  uint64
	}{
		{
			name:                    "uses defaults when no headers provided",
			trustedHeaders:          nil,
			normalHeaders:           http.Header{},
			defaultParallelWorkers:  10,
			defaultStageExecutors:   5,
			expectedParallelWorkers: 10,
			expectedStageExecutors:  5,
		},
		{
			name: "trusted headers override defaults",
			trustedHeaders: dauth.TrustedHeaders{
				HeaderParallelWorkers: "20",
			},
			normalHeaders:           http.Header{},
			defaultParallelWorkers:  10,
			defaultStageExecutors:   5,
			expectedParallelWorkers: 20,
			expectedStageExecutors:  2, // default for free
		},
		{
			name:           "normal headers can only lower values",
			trustedHeaders: nil,
			normalHeaders: http.Header{
				http.CanonicalHeaderKey(HeaderParallelWorkers): []string{"5"}, // lower than default 10
			},
			defaultParallelWorkers:  10,
			defaultStageExecutors:   5,
			expectedParallelWorkers: 5, // lowered
			expectedStageExecutors:  5, // unchanged (cannot increase)
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
			defaultParallelWorkers:  10,
			defaultStageExecutors:   5,
			expectedParallelWorkers: 15, // lowered from trusted value
			expectedStageExecutors:  8,  // increased from the "PRO" plan
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			if tt.trustedHeaders != nil {
				ctx = dauth.WithTrustedHeaders(ctx, tt.trustedHeaders)
			}

			parallelJobs, parallelExecutors := GetEffectiveHeaderValues(
				ctx,
				tt.normalHeaders,
				tt.defaultParallelWorkers,
				tt.defaultStageExecutors,
			)

			assert.Equal(t, tt.expectedParallelWorkers, parallelJobs, "parallel jobs mismatch")
			assert.Equal(t, tt.expectedStageExecutors, parallelExecutors, "parallel executors mismatch")
		})
	}
}

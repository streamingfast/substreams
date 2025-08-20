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
		name                      string
		trustedHeaders            dauth.TrustedHeaders
		normalHeaders             http.Header
		defaultParallelJobs       uint64
		defaultParallelExecutors  uint64
		expectedParallelJobs      uint64
		expectedParallelExecutors uint64
	}{
		{
			name:                      "uses defaults when no headers provided",
			trustedHeaders:            nil,
			normalHeaders:             http.Header{},
			defaultParallelJobs:       10,
			defaultParallelExecutors:  5,
			expectedParallelJobs:      10,
			expectedParallelExecutors: 5,
		},
		{
			name: "trusted headers override defaults",
			trustedHeaders: dauth.TrustedHeaders{
				HeaderParallelJobs:     "20",
				HeaderParallelExecutor: "8",
			},
			normalHeaders:             http.Header{},
			defaultParallelJobs:       10,
			defaultParallelExecutors:  5,
			expectedParallelJobs:      20,
			expectedParallelExecutors: 8,
		},
		{
			name:           "normal headers can only lower values",
			trustedHeaders: nil,
			normalHeaders: http.Header{
				http.CanonicalHeaderKey(HeaderParallelJobs):     []string{"5"},  // lower than default 10
				http.CanonicalHeaderKey(HeaderParallelExecutor): []string{"16"}, // higher than default 5, should be ignored
			},
			defaultParallelJobs:       10,
			defaultParallelExecutors:  5,
			expectedParallelJobs:      5, // lowered
			expectedParallelExecutors: 5, // unchanged (cannot increase)
		},
		{
			name: "normal headers cannot override trusted headers upward",
			trustedHeaders: dauth.TrustedHeaders{
				HeaderParallelJobs:     "20",
				HeaderParallelExecutor: "8",
			},
			normalHeaders: http.Header{
				http.CanonicalHeaderKey(HeaderParallelJobs):     []string{"15"}, // lower than trusted 20
				http.CanonicalHeaderKey(HeaderParallelExecutor): []string{"10"}, // higher than trusted 8, should be ignored
			},
			defaultParallelJobs:       10,
			defaultParallelExecutors:  5,
			expectedParallelJobs:      15, // lowered from trusted value
			expectedParallelExecutors: 8,  // unchanged (cannot increase from trusted)
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
				tt.defaultParallelJobs,
				tt.defaultParallelExecutors,
			)

			assert.Equal(t, tt.expectedParallelJobs, parallelJobs, "parallel jobs mismatch")
			assert.Equal(t, tt.expectedParallelExecutors, parallelExecutors, "parallel executors mismatch")
		})
	}
}

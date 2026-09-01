package tools

import (
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"slices"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"
)

func TestClassifyStreamError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want failureReason
	}{
		{
			name: "stream completed without any block",
			err:  io.EOF,
			want: reasonNoData,
		},
		{
			name: "wrapped end of stream",
			err:  fmt.Errorf("receiving message: %w", io.EOF),
			want: reasonNoData,
		},
		{
			name: "local context deadline",
			err:  context.DeadlineExceeded,
			want: reasonRequestTimeout,
		},
		{
			name: "grpc deadline exceeded",
			err:  grpcstatus.Error(codes.DeadlineExceeded, "context deadline exceeded"),
			want: reasonRequestTimeout,
		},
		{
			name: "endpoint unavailable",
			err:  grpcstatus.Error(codes.Unavailable, "no healthy upstream"),
			want: reasonStreamError,
		},
		{
			name: "endpoint refused our credentials",
			err:  grpcstatus.Error(codes.Unauthenticated, "invalid token"),
			want: reasonStreamError,
		},
		{
			name: "plain error",
			err:  errors.New("boom"),
			want: reasonStreamError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, classifyStreamError(tt.err))
		})
	}
}

func TestGRPCCodeOf(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{"no error", nil, noGRPCCode},
		{"not a grpc error", errors.New("boom"), noGRPCCode},
		{"grpc error", grpcstatus.Error(codes.ResourceExhausted, "overloaded"), "ResourceExhausted"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, grpcCodeOf(tt.err))
		})
	}
}

// Endpoints configured with different query parameters must all produce the same number of
// label values, otherwise the prometheus `WithLabelValues` calls panic on inconsistent cardinality.
func TestEndpointLabelValues(t *testing.T) {
	endpoints := []string{
		"a.domain:443?namespace=eth-mainnet",
		"b.domain:443?namespace=sol-mainnet&region=us-east",
		"c.domain:443",
	}

	allLabels := map[string]bool{endpointLabel: true}
	params := map[string]map[string]string{}

	for _, endpoint := range endpoints {
		url, _, endpointParams, err := parseEndpoint(endpoint)
		require.NoError(t, err)

		for k := range endpointParams {
			allLabels[k] = true
		}
		params[url] = endpointParams
	}

	labelNames := slices.Sorted(maps.Keys(allLabels))
	require.Equal(t, []string{"endpoint", "namespace", "region"}, labelNames)

	values := map[string][]string{}
	for url, endpointParams := range params {
		values[url] = endpointLabelValues(url, endpointParams, labelNames)
	}

	assert.Equal(t, []string{"a.domain:443", "eth-mainnet", ""}, values["a.domain:443"])
	assert.Equal(t, []string{"b.domain:443", "sol-mainnet", "us-east"}, values["b.domain:443"])
	assert.Equal(t, []string{"c.domain:443", "", ""}, values["c.domain:443"])

	gauge := prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: "test_status"}, labelNames)
	for _, labelValues := range values {
		assert.NotPanics(t, func() { gauge.WithLabelValues(labelValues...).Set(1) })
	}
}

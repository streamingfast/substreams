package tools

import (
	"context"
	"errors"
	"io"
	"slices"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/streamingfast/dgrpc"
	"github.com/streamingfast/dmetrics"
	"google.golang.org/grpc/codes"
)

// This file holds the metrics of the `prometheus-exporter` command: the failure taxonomy
// that gives them their labels, their declaration and the label plumbing. The polling loop
// that feeds them lives in `prometheus-exporter.go`.

// failureReason categorizes *where* a poll failed, so that an alert firing on
// `substreams_healthcheck_status == 0` can be traced back to a cause without
// having to correlate it with the logs.
type failureReason string

const (
	// reasonConnect means the gRPC client could not even be constructed.
	reasonConnect failureReason = "connect"
	// reasonConnectTimeout means the gRPC channel never reached the READY state within
	// the connect timeout: DNS, TCP, TLS or load-balancer resolution is the problem, the
	// Substreams backend was never reached.
	reasonConnectTimeout failureReason = "connect_timeout"
	// reasonInvalidRequest means the request could not be built or validated, this is a
	// configuration problem on our side, never an endpoint problem.
	reasonInvalidRequest failureReason = "invalid_request"
	// reasonRequestTimeout means the endpoint accepted the request but did not deliver a
	// block within the request timeout.
	reasonRequestTimeout failureReason = "request_timeout"
	// reasonStreamError means the endpoint returned a gRPC error, see the `grpc_code` label.
	reasonStreamError failureReason = "stream_error"
	// reasonStaleBlock means the endpoint answered correctly but the block it returned is
	// older than --max-freshness.
	reasonStaleBlock failureReason = "stale_block"
	// reasonNoData means the stream completed without ever returning block data.
	reasonNoData failureReason = "no_data"
)

// noGRPCCode is the value of the `grpc_code` label for failures that did not carry a gRPC status.
const noGRPCCode = "none"

const (
	// endpointLabel is the one label every endpoint always carries, the others come from the
	// query parameters of the endpoint specification.
	endpointLabel = "endpoint"
	// reasonLabel and grpcCodeLabel are carried by the failure counter only.
	reasonLabel   = "reason"
	grpcCodeLabel = "grpc_code"
)

var healthcheckMetrics = dmetrics.NewSet(dmetrics.PrefixNameWith("substreams_healthcheck"))

var (
	status              *dmetrics.GaugeVec
	requestDurationMs   *dmetrics.GaugeVec
	connectDurationMs   *dmetrics.GaugeVec
	streamDurationMs    *dmetrics.GaugeVec
	blockAgeMs          *dmetrics.GaugeVec
	consecutiveFailures *dmetrics.GaugeVec
	failureCount        *dmetrics.CounterVec
)

// initHealthcheckMetrics declares every healthcheck metric over the given label names, which
// are the union of the labels across all the polled endpoints, and returns the collectors so
// that the caller can register them on its own registry.
func initHealthcheckMetrics(labelNames []string) []prometheus.Collector {
	failureLabelNames := append(slices.Clone(labelNames), reasonLabel, grpcCodeLabel)

	status = healthcheckMetrics.NewGaugeVec("status", labelNames, "Either 1 for successful subtreams request, or 0 for failure")
	requestDurationMs = healthcheckMetrics.NewGaugeVec("duration_ms", labelNames, "Request full processing time in millisecond, connection establishment included")
	connectDurationMs = healthcheckMetrics.NewGaugeVec("connect_duration_ms", labelNames, "Time spent establishing the gRPC connection in millisecond, this excludes the Blocks request itself")
	streamDurationMs = healthcheckMetrics.NewGaugeVec("stream_duration_ms", labelNames, "Time spent on the Blocks request in millisecond, from an established connection to the first block")
	blockAgeMs = healthcheckMetrics.NewGaugeVec("block_age_ms", labelNames, "Age of returned block, NaN when the last poll did not return a block")
	consecutiveFailures = healthcheckMetrics.NewGaugeVec("consecutive_failures", labelNames, "Number of consecutive failed polls, 0 when the last poll succeeded. Alert on this instead of 'status' to ignore single-poll hiccups")
	failureCount = healthcheckMetrics.NewCounterVec("failure_count", failureLabelNames, "Number of failed polls, broken down by 'reason' and by gRPC status code")

	return []prometheus.Collector{status, requestDurationMs, connectDurationMs, streamDurationMs, blockAgeMs, consecutiveFailures, failureCount}
}

// endpointLabelValues orders an endpoint's parameters along labelNames, filling in an empty
// value for the labels this endpoint was not given.
func endpointLabelValues(url string, params map[string]string, labelNames []string) []string {
	values := make([]string, len(labelNames))
	for i, name := range labelNames {
		if name == endpointLabel {
			values[i] = url
			continue
		}
		values[i] = params[name]
	}
	return values
}

// failureLabelValues returns the endpoint label values augmented with the failure
// classification, used by the `substreams_healthcheck_failure_count` counter only.
func failureLabelValues(endpoint string, reason failureReason, grpcCode string) []string {
	return append(slices.Clone(endpointMap[endpoint].labelValues), string(reason), grpcCode)
}

// grpcCodeOf returns the gRPC status code carried by err, or `noGRPCCode` when there is none.
func grpcCodeOf(err error) string {
	if grpcError := dgrpc.AsGRPCError(err); grpcError != nil {
		return grpcError.Code().String()
	}
	return noGRPCCode
}

// classifyStreamError maps an error returned while talking to the endpoint onto a
// failureReason, distinguishing a timeout from an outright refusal, and an empty
// stream from a stream that errored out.
func classifyStreamError(err error) failureReason {
	if errors.Is(err, io.EOF) {
		return reasonNoData
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return reasonRequestTimeout
	}
	if grpcError := dgrpc.AsGRPCError(err); grpcError != nil && grpcError.Code() == codes.DeadlineExceeded {
		return reasonRequestTimeout
	}
	return reasonStreamError
}

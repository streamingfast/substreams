package metrics

import (
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/streamingfast/dmetrics"
	"github.com/stretchr/testify/assert"
)

// installTestRPCCallMetrics points the tier1/tier2 RPC call metrics at a throwaway set for the
// duration of the test, restoring whatever was there before.
func installTestRPCCallMetrics(t *testing.T) {
	t.Helper()

	previousTier1Counter, previousTier1Duration := Tier1RPCCallCounter, Tier1RPCCallDuration
	previousTier2Counter, previousTier2Duration := Tier2RPCCallCounter, Tier2RPCCallDuration
	t.Cleanup(func() {
		Tier1RPCCallCounter, Tier1RPCCallDuration = previousTier1Counter, previousTier1Duration
		Tier2RPCCallCounter, Tier2RPCCallDuration = previousTier2Counter, previousTier2Duration
	})

	set := dmetrics.NewSet()
	Tier1RPCCallCounter = set.NewCounterVec("test_tier1_rpc_call_counter", rpcCallLabels, "test")
	Tier1RPCCallDuration = set.NewHistogramVecCustomBuckets("test_tier1_rpc_call_duration_seconds", rpcCallLabels, rpcCallDurationBuckets, "test")
	Tier2RPCCallCounter = set.NewCounterVec("test_tier2_rpc_call_counter", rpcCallLabels, "test")
	Tier2RPCCallDuration = set.NewHistogramVecCustomBuckets("test_tier2_rpc_call_duration_seconds", rpcCallLabels, rpcCallDurationBuckets, "test")
}

func TestRecordRPCCall_RoutesToTierMetric(t *testing.T) {
	installTestRPCCallMetrics(t)

	RecordRPCCall(false, "eth:call", RPCCallOutcomeSuccess, 10*time.Millisecond)
	RecordRPCCall(false, "eth:call", RPCCallOutcomeSuccess, 10*time.Millisecond)
	RecordRPCCall(true, "eth:call", RPCCallOutcomeSuccess, 10*time.Millisecond)

	assert.Equal(t, float64(2), testutil.ToFloat64(Tier1RPCCallCounter.Native().WithLabelValues("eth:call", RPCCallOutcomeSuccess)))
	assert.Equal(t, float64(1), testutil.ToFloat64(Tier2RPCCallCounter.Native().WithLabelValues("eth:call", RPCCallOutcomeSuccess)))
}

func TestRecordRPCCall_SeparatesOutcomes(t *testing.T) {
	installTestRPCCallMetrics(t)

	RecordRPCCall(false, "eth:call", RPCCallOutcomeSuccess, 10*time.Millisecond)
	RecordRPCCall(false, "eth:call", RPCCallOutcomeError, 30*time.Second)

	assert.Equal(t, float64(1), testutil.ToFloat64(Tier1RPCCallCounter.Native().WithLabelValues("eth:call", RPCCallOutcomeSuccess)))
	assert.Equal(t, float64(1), testutil.ToFloat64(Tier1RPCCallCounter.Native().WithLabelValues("eth:call", RPCCallOutcomeError)))
}

// A process only declares the metrics of the tier it serves, and tests declare none at all, so
// recording against an undeclared tier must be a no-op rather than a nil dereference.
func TestRecordRPCCall_UndeclaredTierIsNoop(t *testing.T) {
	installTestRPCCallMetrics(t)

	Tier2RPCCallCounter, Tier2RPCCallDuration = nil, nil

	assert.NotPanics(t, func() {
		RecordRPCCall(true, "eth:call", RPCCallOutcomeSuccess, 10*time.Millisecond)
	})
}

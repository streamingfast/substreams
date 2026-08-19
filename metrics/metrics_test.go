package metrics

import (
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/streamingfast/dmetrics"
	"github.com/stretchr/testify/assert"
)

// installTestWasmExtensionCallMetrics points the tier1/tier2 WASM extension call metrics at a
// throwaway set for the duration of the test, restoring whatever was there before.
func installTestWasmExtensionCallMetrics(t *testing.T) {
	t.Helper()

	previousTier1Counter, previousTier1Duration := Tier1WasmExtensionCallCounter, Tier1WasmExtensionCallDuration
	previousTier2Counter, previousTier2Duration := Tier2WasmExtensionCallCounter, Tier2WasmExtensionCallDuration
	t.Cleanup(func() {
		Tier1WasmExtensionCallCounter, Tier1WasmExtensionCallDuration = previousTier1Counter, previousTier1Duration
		Tier2WasmExtensionCallCounter, Tier2WasmExtensionCallDuration = previousTier2Counter, previousTier2Duration
	})

	set := dmetrics.NewSet()
	Tier1WasmExtensionCallCounter = set.NewCounterVec("test_tier1_wasm_extension_call_counter", wasmExtensionCallLabels, "test")
	Tier1WasmExtensionCallDuration = set.NewHistogramVecCustomBuckets("test_tier1_wasm_extension_call_duration_seconds", wasmExtensionCallLabels, standardDurationBuckets, "test")
	Tier2WasmExtensionCallCounter = set.NewCounterVec("test_tier2_wasm_extension_call_counter", wasmExtensionCallLabels, "test")
	Tier2WasmExtensionCallDuration = set.NewHistogramVecCustomBuckets("test_tier2_wasm_extension_call_duration_seconds", wasmExtensionCallLabels, standardDurationBuckets, "test")
}

func TestRecordWasmExtensionCall_RoutesToTierMetric(t *testing.T) {
	installTestWasmExtensionCallMetrics(t)

	RecordWasmExtensionCall(false, "eth:call", WasmExtensionCallOutcomeSuccess, 10*time.Millisecond)
	RecordWasmExtensionCall(false, "eth:call", WasmExtensionCallOutcomeSuccess, 10*time.Millisecond)
	RecordWasmExtensionCall(true, "eth:call", WasmExtensionCallOutcomeSuccess, 10*time.Millisecond)

	assert.Equal(t, float64(2), testutil.ToFloat64(Tier1WasmExtensionCallCounter.Native().WithLabelValues("eth:call", WasmExtensionCallOutcomeSuccess)))
	assert.Equal(t, float64(1), testutil.ToFloat64(Tier2WasmExtensionCallCounter.Native().WithLabelValues("eth:call", WasmExtensionCallOutcomeSuccess)))
}

func TestRecordWasmExtensionCall_SeparatesOutcomes(t *testing.T) {
	installTestWasmExtensionCallMetrics(t)

	RecordWasmExtensionCall(false, "eth:call", WasmExtensionCallOutcomeSuccess, 10*time.Millisecond)
	RecordWasmExtensionCall(false, "eth:call", WasmExtensionCallOutcomeError, 30*time.Second)

	assert.Equal(t, float64(1), testutil.ToFloat64(Tier1WasmExtensionCallCounter.Native().WithLabelValues("eth:call", WasmExtensionCallOutcomeSuccess)))
	assert.Equal(t, float64(1), testutil.ToFloat64(Tier1WasmExtensionCallCounter.Native().WithLabelValues("eth:call", WasmExtensionCallOutcomeError)))
}

// A process only declares the metrics of the tier it serves, and tests declare none at all, so
// recording against an undeclared tier must be a no-op rather than a nil dereference.
func TestRecordWasmExtensionCall_UndeclaredTierIsNoop(t *testing.T) {
	installTestWasmExtensionCallMetrics(t)

	Tier2WasmExtensionCallCounter, Tier2WasmExtensionCallDuration = nil, nil

	assert.NotPanics(t, func() {
		RecordWasmExtensionCall(true, "eth:call", WasmExtensionCallOutcomeSuccess, 10*time.Millisecond)
	})
}

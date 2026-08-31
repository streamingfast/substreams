package metrics

import (
	"time"

	"github.com/streamingfast/dmetrics"
	"go.uber.org/zap"
)

var MetricSet = dmetrics.NewSet()

var AppReadinessTier1 *dmetrics.AppReadiness
var Tier1RequestsCounter *dmetrics.Counter

// var Tier1RequestsCounter *dmetrics.Counter
var Tier1RejectedRequestCounter *dmetrics.CounterVec
var Tier1ActiveRequests *dmetrics.Gauge
var Tier1SquashersStarted *dmetrics.Counter
var Tier1SquashersEnded *dmetrics.Counter
var Tier1ActiveWorkerRequest *dmetrics.Gauge
var Tier1WorkerRequestCounter *dmetrics.Counter
var Tier1WorkerRetryCounter *dmetrics.Counter
var Tier1WorkerRejectedOverloadedCounter *dmetrics.Counter

var Tier1WasmExtensionCallCounter *dmetrics.CounterVec
var Tier1WasmExtensionCallDuration *dmetrics.HistogramVec

var FoundationalStoreResolution *dmetrics.CounterVec
var FoundationalStoreResolutionDuration *dmetrics.HistogramVec

var AppReadinessTier2 *dmetrics.AppReadiness
var Tier2ActiveRequests *dmetrics.Gauge
var Tier2RequestCounter *dmetrics.Counter
var Tier2RejectedRequestCounter *dmetrics.CounterVec
var Tier2WasmExtensionCallCounter *dmetrics.CounterVec
var Tier2WasmExtensionCallDuration *dmetrics.HistogramVec

var BlockBeginProcess = MetricSet.NewCounter("substreams_block_process_start_counter", "Counter for total block processes started, used for rate")
var BlockEndProcess = MetricSet.NewCounter("substreams_block_process_end_counter", "Counter for total block processes ended, used for rate")
var ExecutedWasmModules = MetricSet.NewCounter("substreams_executed_wasm_modules", "Counter for total WASM executions for each module on each block")
var SkippedCachedWasmModules = MetricSet.NewCounter("substreams_skipped_cached_wasm_modules", "Counter for total WASM skipped executions for each module on each block due to the shared cache")

// UndoSignalDistance observes how many blocks each BlockUndoSignal sent to clients reverts, by source:
// "reorg" when a fork is detected while streaming, "cursor_resolution" when the cursor
// of an incoming request points to a block that is no longer part of the canonical chain.
// The '_count' gives the total number of undo signals sent, subtracting the 'le="5"' bucket
// from it gives the number of undo signals reverting more than 5 blocks.
var UndoSignalDistance = MetricSet.NewHistogramVecCustomBuckets(
	"substreams_undo_signal_distance_blocks",
	[]string{"source"},
	[]float64{1, 2, 5, 10, 25, 100},
	"Distribution of the number of blocks reverted by the undo signals sent to clients, by source",
)

var Tier1OutputHeadBlockRelativeTime *dmetrics.HeadBlockRelativeTime

// Store backend metrics
var StoreBackendType *dmetrics.GaugeVec
var StoreMmapFileSizeBytes *dmetrics.GaugeVec
var StoreMmapOperationsTotal *dmetrics.CounterVec
var Tier1ActiveRequestsHardLimit *dmetrics.Gauge
var Tier2MaxConcurrentRequests *dmetrics.Gauge

// CPU signals of the tier1 container's own cgroup, sampled by the request evictor
var Tier1CPUQuotaCores *dmetrics.Gauge
var Tier1CPUUsageRatio *dmetrics.Gauge
var Tier1CPUThrottleRatio *dmetrics.Gauge
var Tier1CPUPressureSomeAvg10 *dmetrics.Gauge
var Tier1CPUOverloaded *dmetrics.Gauge
var Tier1EffectiveActiveRequests *dmetrics.Gauge
var Tier1EvictedRequestsCounter *dmetrics.CounterVec

func DeclareTier1Metrics(zlog *zap.Logger) {
	AppReadinessTier1 = MetricSet.NewAppReadiness("substreams_tier1")
	Tier1RequestsCounter = MetricSet.NewCounter("substreams_counter", "Total Substreams requests count on tier1")
	Tier1RejectedRequestCounter = MetricSet.NewCounterVec(
		"substreams_tier1_rejected_request_counter",
		[]string{"reason"},
		"Counter for total Substreams requests the tier1 rejected, by reason (gRPC code reason)",
	)
	Tier1ActiveRequests = MetricSet.NewGauge("substreams_active_requests", "Number of active Substreams requests on tier1")

	Tier1SquashersStarted = MetricSet.NewCounter("substreams_total_squash_processes_launched", "Counter for Total squash processes launched on tier1, used for rate")
	Tier1SquashersEnded = MetricSet.NewCounter("substreams_total_squash_processes_closed", "Counter for Total squash processes closed on tier1, used for active processes")

	Tier1ActiveWorkerRequest = MetricSet.NewGauge("substreams_tier1_active_worker_requests", "Number of active Substreams worker requests a tier1 app is currently doing against tier2 nodes")
	Tier1WorkerRequestCounter = MetricSet.NewCounter("substreams_tier1_worker_request_counter", "Counter for total Substreams worker requests a tier1 app made against tier2 nodes")
	Tier1WorkerRetryCounter = MetricSet.NewCounter("substreams_tier1_worker_retry_counter", "Counter for total retryable errors returned from tier2")
	Tier1WorkerRejectedOverloadedCounter = MetricSet.NewCounter("substreams_tier1_worker_rejected_overloaded_counter", "Counter for number of times a worker rejected a request because it was overloaded (included in RetryCounter)")
	Tier1OutputHeadBlockRelativeTime = MetricSet.NewHeadBlockRelativeTime("substreams_output")
	Tier1ActiveRequestsHardLimit = MetricSet.NewGauge("substreams_tier1_active_requests_hard_limit", "Hard limit of concurrent active requests on tier1 (0 means unlimited)")

	Tier1CPUQuotaCores = MetricSet.NewGauge("substreams_tier1_cpu_quota_cores", "CPU quota of the tier1 container's cgroup in cores (0 means no limit)")
	Tier1CPUUsageRatio = MetricSet.NewGauge("substreams_tier1_cpu_usage_ratio", "CPU used by the tier1 container as a fraction of its cgroup quota, over the last evictor evaluation interval")
	Tier1CPUThrottleRatio = MetricSet.NewGauge("substreams_tier1_cpu_throttle_ratio", "Fraction of CFS periods in which the tier1 container was throttled, over the last evictor evaluation interval")
	Tier1CPUPressureSomeAvg10 = MetricSet.NewGauge("substreams_tier1_cpu_pressure_some_avg10", "PSI 'some' avg10 of the tier1 container's cgroup, fraction 0..1 of time runnable threads waited for CPU")
	Tier1CPUOverloaded = MetricSet.NewGauge("substreams_tier1_cpu_overloaded", "1 while the tier1 pod considers itself CPU-overloaded (unready, may evict requests), 0 otherwise")
	Tier1EffectiveActiveRequests = MetricSet.NewGauge(
		"substreams_tier1_effective_active_requests",
		"Active requests on tier1 adjusted for what they cost: the higher of substreams_active_requests and the number of requests the CPU budget is being spent at. Meant as the horizontal autoscaler input, since it stays at capacity while the pod is evicting instead of dropping with the requests it cancelled",
	)
	Tier1EvictedRequestsCounter = MetricSet.NewCounterVec(
		"substreams_tier1_evicted_requests_counter",
		[]string{"class", "action"},
		"Requests the CPU evictor selected, by class (dev, prod-catchup, prod-live) and action (cancelled, or observed in observe mode)",
	)

	StoreBackendType = MetricSet.NewGaugeVec(
		"substreams_store_backend_type",
		[]string{"backend", "store_name"},
		"Store backend type in use (1=mmap, 2=memory)",
	)
	StoreMmapFileSizeBytes = MetricSet.NewGaugeVec(
		"substreams_store_mmap_file_size_bytes",
		[]string{"store_name"},
		"Size of mmap database files in bytes",
	)
	StoreMmapOperationsTotal = MetricSet.NewCounterVec(
		"substreams_store_mmap_operations_total",
		[]string{"operation", "store_name"},
		"Total number of mmap operations (get, set, delete, scan)",
	)

	Tier1WasmExtensionCallCounter = MetricSet.NewCounterVec(
		"substreams_tier1_wasm_extension_call_counter",
		wasmExtensionCallLabels,
		"Counter for external calls made by WASM extensions (e.g. eth_call) on tier1, by extension and outcome",
	)
	Tier1WasmExtensionCallDuration = MetricSet.NewHistogramVecCustomBuckets(
		"substreams_tier1_wasm_extension_call_duration_seconds",
		wasmExtensionCallLabels,
		standardDurationBuckets,
		"Duration of external calls made by WASM extensions (e.g. eth_call) on tier1, by extension and outcome",
	)

	FoundationalStoreResolution = MetricSet.NewCounterVec(
		"substreams_foundational_store_resolution_total",
		[]string{"result"},
		"Foundational-store identifier resolutions on tier1, by result (success or error)",
	)
	FoundationalStoreResolutionDuration = MetricSet.NewHistogramVecCustomBuckets(
		"substreams_foundational_store_resolution_duration_seconds",
		[]string{"result"},
		standardDurationBuckets,
		"Duration of foundational-store identifier resolutions on tier1, by result",
	)

	zlog.Info("registering substreams tier1 metrics")
}

var wasmExtensionCallLabels = []string{"extension", "outcome"}

// standardDurationBuckets are the Prometheus default buckets extended with a 30s and 60s
// tail, so that slow calls and timeouts (the ones actually clogging a tier) land in a visible
// bucket instead of all collapsing into +Inf.
var standardDurationBuckets = []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10, 30, 60}

const (
	WasmExtensionCallOutcomeSuccess = "success"
	WasmExtensionCallOutcomeError   = "error"

	FoundationalStoreResolutionSuccess = "success"
	FoundationalStoreResolutionError   = "error"
)

// RecordFoundationalStoreResolution records one identifier lookup on tier1.
// Metrics are nil in tests and on processes that never declared tier1 metrics.
func RecordFoundationalStoreResolution(ok bool, elapsed time.Duration) {
	if FoundationalStoreResolution == nil || FoundationalStoreResolutionDuration == nil {
		return
	}
	result := FoundationalStoreResolutionSuccess
	if !ok {
		result = FoundationalStoreResolutionError
	}
	FoundationalStoreResolution.Inc(result)
	FoundationalStoreResolutionDuration.ObserveDuration(elapsed, result)
}

// RecordWasmExtensionCall records a single external call made by a WASM extension, eth_call being
// the main one. Following the convention used throughout this package, the tier is encoded in the
// metric name rather than in a label, so the caller resolves it and passes it in. It cannot be
// resolved here from a context because `reqctx` imports this package.
//
// The metrics are nil when the corresponding tier's metrics were never declared, which is the case
// in tests and, in production, for the tier a process does not serve.
func RecordWasmExtensionCall(isTier2 bool, extension, outcome string, elapsed time.Duration) {
	counter, duration := Tier1WasmExtensionCallCounter, Tier1WasmExtensionCallDuration
	if isTier2 {
		counter, duration = Tier2WasmExtensionCallCounter, Tier2WasmExtensionCallDuration
	}

	if counter == nil || duration == nil {
		return
	}

	counter.Inc(extension, outcome)
	duration.ObserveDuration(elapsed, extension, outcome)
}

func DeclareTier2Metrics(zlog *zap.Logger) {
	AppReadinessTier2 = MetricSet.NewAppReadiness("substreams_tier2")
	Tier2ActiveRequests = MetricSet.NewGauge("substreams_tier2_active_requests", "Number of active Substreams requests the tier2 is currently serving")
	Tier2RequestCounter = MetricSet.NewCounter("substreams_tier2_request_counter", "Counter for total Substreams requests the tier2 served")
	Tier2RejectedRequestCounter = MetricSet.NewCounterVec(
		"substreams_tier2_rejected_request_counter",
		[]string{"reason"},
		"Counter for total Substreams requests the tier2 rejected, by reason (gRPC code reason)",
	)
	Tier2MaxConcurrentRequests = MetricSet.NewGauge("substreams_tier2_max_concurrent_requests", "Hard limit of concurrent requests on tier2 (0 means unlimited)")

	Tier2WasmExtensionCallCounter = MetricSet.NewCounterVec(
		"substreams_tier2_wasm_extension_call_counter",
		wasmExtensionCallLabels,
		"Counter for external calls made by WASM extensions (e.g. eth_call) on tier2, by extension and outcome",
	)
	Tier2WasmExtensionCallDuration = MetricSet.NewHistogramVecCustomBuckets(
		"substreams_tier2_wasm_extension_call_duration_seconds",
		wasmExtensionCallLabels,
		standardDurationBuckets,
		"Duration of external calls made by WASM extensions (e.g. eth_call) on tier2, by extension and outcome",
	)

	zlog.Info("registering tier2 substreams metrics")
}

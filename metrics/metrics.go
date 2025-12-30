package metrics

import (
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

var AppReadinessTier2 *dmetrics.AppReadiness
var Tier2ActiveRequests *dmetrics.Gauge
var Tier2RequestCounter *dmetrics.Counter
var Tier2RejectedRequestCounter *dmetrics.CounterVec

var BlockBeginProcess = MetricSet.NewCounter("substreams_block_process_start_counter", "Counter for total block processes started, used for rate")
var BlockEndProcess = MetricSet.NewCounter("substreams_block_process_end_counter", "Counter for total block processes ended, used for rate")
var ExecutedWasmModules = MetricSet.NewCounter("substreams_executed_wasm_modules", "Counter for total WASM executions for each module on each block")
var SkippedCachedWasmModules = MetricSet.NewCounter("substreams_skipped_cached_wasm_modules", "Counter for total WASM skipped executions for each module on each block due to the shared cache")

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

	zlog.Info("registering substreams tier1 metrics")
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
	zlog.Info("registering tier2 substreams metrics")
}

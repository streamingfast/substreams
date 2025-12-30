package metrics

import (
	"sync"

	"github.com/streamingfast/dmetrics"
	"go.uber.org/zap"
)

var MetricSet = dmetrics.NewSet()

var ActiveRequests = MetricSet.NewGauge("substreams_active_requests", "Number of active Substreams requests")
var SubstreamsCounter = MetricSet.NewCounter("substreams_counter", "Substreams requests count")

var BlockBeginProcess = MetricSet.NewCounter("substreams_block_process_start_counter", "Counter for total block processes started, used for rate")
var BlockEndProcess = MetricSet.NewCounter("substreams_block_process_end_counter", "Counter for total block processes ended, used for rate")

var SquashersStarted = MetricSet.NewCounter("substreams_total_squash_processes_launched", "Counter for Total squash processes launched, used for rate")
var SquashersEnded = MetricSet.NewCounter("substreams_total_squash_processes_closed", "Counter for Total squash processes closed, used for active processes")

var Tier1ActiveWorkerRequest = MetricSet.NewGauge("substreams_tier1_active_worker_requests", "Number of active Substreams worker requests a tier1 app is currently doing against tier2 nodes")
var Tier1RejectedRequestCounter = MetricSet.NewCounterVec(
	"substreams_tier1_rejected_request_counter",
	[]string{"reason"},
	"Counter for total Substreams requests the tier1 rejected, by reason (gRPC code reason)",
)
var Tier1WorkerRequestCounter = MetricSet.NewCounter("substreams_tier1_worker_request_counter", "Counter for total Substreams worker requests a tier1 app made against tier2 nodes")
var Tier1WorkerRetryCounter = MetricSet.NewCounter("substreams_tier1_worker_retry_counter", "Counter for total retryable errors returned from tier2")
var Tier1WorkerRejectedOverloadedCounter = MetricSet.NewCounter("substreams_tier1_worker_rejected_overloaded_counter", "Counter for number of times a worker rejected a request because it was overloaded (included in RetryCounter)")

var Tier2ActiveRequests = MetricSet.NewGauge("substreams_tier2_active_requests", "Number of active Substreams requests the tier2 is currently serving")
var Tier2RequestCounter = MetricSet.NewCounter("substreams_tier2_request_counter", "Counter for total Substreams requests the tier2 served")
var Tier2RejectedRequestCounter = MetricSet.NewCounterVec(
	"substreams_tier2_rejected_request_counter",
	[]string{"reason"},
	"Counter for total Substreams requests the tier2 rejected, by reason (gRPC code reason)",
)

var ExecutedWasmModules = MetricSet.NewCounter("substreams_executed_wasm_modules", "Counter for total WASM executions for each module on each block")
var SkippedCachedWasmModules = MetricSet.NewCounter("substreams_skipped_cached_wasm_modules", "Counter for total WASM skipped executions for each module on each block due to the shared cache")

var AppReadinessTier1 = MetricSet.NewAppReadiness("substreams_tier1")
var AppReadinessTier2 = MetricSet.NewAppReadiness("substreams_tier2")

var registerOnce sync.Once

func RegisterMetricSet(zlog *zap.Logger) {
	registerOnce.Do(func() {
		zlog.Info("registering substreams metrics")
		MetricSet.Register()
	})
}

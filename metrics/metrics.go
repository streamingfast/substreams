package metrics

import (
	"sync"

	"github.com/streamingfast/dmetrics"
	"go.uber.org/zap"
)

var MetricSet = dmetrics.NewSet()

var ActiveSubstreams = MetricSet.NewGauge("substreams_active_requests", "Number of active Substreams requests")
var SubstreamsCounter = MetricSet.NewCounter("substreams_counter", "Substreams requests count")

var BlockBeginProcess = MetricSet.NewCounter("substreams_block_process_start_counter", "Counter for total block processes started, used for rate")
var BlockEndProcess = MetricSet.NewCounter("substreams_block_process_end_counter", "Counter for total block processes ended, used for rate")

var SquashesLaunched = MetricSet.NewCounter("substreams_total_squashes_launched", "Counter for Total squashes launched, used for rate")
var SquashersStarted = MetricSet.NewCounter("substreams_total_squash_processes_launched", "Counter for Total squash processes launched, used for rate")
var SquashersEnded = MetricSet.NewCounter("substreams_total_squash_processes_closed", "Counter for Total squash processes closed, used for active processes")

var Tier1ActiveWorkerRequest = MetricSet.NewGauge("substreams_tier1_active_worker_requests", "Number of active Substreams worker requests a tier1 app is currently doing against tier2 nodes")
var Tier1WorkerRequestCounter = MetricSet.NewCounter("substreams_tier1_worker_request_counter", "Counter for total Substreams worker requests a tier1 app made against tier2 nodes")

var Tier2ActiveRequests = MetricSet.NewGauge("substreams_tier2_active_requests", "Number of active Substreams requests the tier2 is currently serving")
var Tier2RequestCounter = MetricSet.NewCounter("substreams_tier2_request_counter", "Counter for total Substreams requests the tier2 served")

var AppReadinessTier1 = MetricSet.NewAppReadiness("substreams_tier1")
var AppReadinessTier2 = MetricSet.NewAppReadiness("substreams_tier2")

var registerOnce sync.Once

func RegisterMetricSet(zlog *zap.Logger) {
	registerOnce.Do(func() {
		zlog.Info("registering substreams metrics")
		MetricSet.Register()
	})
}

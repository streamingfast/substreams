package metrics

import (
	"github.com/streamingfast/dmetrics"
)

var MetricSet = dmetrics.NewSet()

var BlockBeginProcess = MetricSet.NewCounter("substreams_block_process_start_counter", "Counter for total block processes started, used for rate")
var BlockEndProcess = MetricSet.NewCounter("substreams_block_process_end_counter", "Counter for total block processes ended, used for rate")

var SquashesLaunched = MetricSet.NewCounter("substreams_total_squashes_launched", "Counter for Total squashes launched, used for rate")
var SquashersStarted = MetricSet.NewCounter("substreams_total_squash_processes_launched", "Counter for Total squash processes launched, used for rate")
var SquashersEnded = MetricSet.NewCounter("substreams_total_squash_processes_closed", "Counter for Total squash processes closed, used for active processes")

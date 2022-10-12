package metrics

import (
	"github.com/streamingfast/dmetrics"
)

var Metricset = dmetrics.NewSet()

//var ActiveSquashingProcesses = Metricset.NewGauge("substreams_active_squashes", "Number of Squash Processes Ongoing")

var BlockBeginProcess = Metricset.NewCounter("substreams_block_process_start_counter", "Counter for total block processes started, used for rate")
var BlockEndProcess = Metricset.NewCounter("substreams_block_process_end_counter", "Counter for total block processes ended, used for rate")

var LastSquashDuration = Metricset.NewGauge("substreams_last_squash_process_duration", "Gauge for monitoring most recent complete squash duration")
var LastSquashAvgDuration = Metricset.NewGauge("substreams_last_squash_process_avg_duration", "Gauge for monitoring the average individual duration of the most recent complete squash")

var SquashesLaunched = Metricset.NewCounter("substreams_total_squashes_launched", "Counter for Total squash processes launched, used for rate")

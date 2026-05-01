package db

import (
	"github.com/streamingfast/dmetrics"
)

var metrics = dmetrics.NewSet(dmetrics.PrefixNameWith("substreams_sink_sql"))

var QueryExecutionDuration = metrics.NewCounterVec("tx_query_execution_duration", []string{"query_type"}, "The amount of time spent executing queries by type (normal/undo) in nanoseconds")
var PruneReversibleSegmentDuration = metrics.NewCounter("prune_reversible_segment_duration", "The amount of time spent pruning reversible segment in nanoseconds")

func RegisterMetrics() {
	metrics.Register()
}

package sinker

import (
	"github.com/streamingfast/dmetrics"
	"github.com/streamingfast/substreams/sink/sql/db_changes/db"
)

func RegisterMetrics() {
	metrics.Register()
	db.RegisterMetrics()
}

var metrics = dmetrics.NewSet()

// TODO: these metrics are named `substreams_sink_postgres_*` whatever the dialect, ClickHouse included.
// New ones keep that prefix so they sit with the others; renaming the whole set breaks existing dashboards
// and is to be done in one go.

var FlushCount = metrics.NewCounter("substreams_sink_postgres_store_flush_count", "The amount of flush that happened so far")
var FlushedRowsCount = metrics.NewCounter("substreams_sink_postgres_flushed_rows_count", "The number of flushed rows so far")
var FlushDuration = metrics.NewCounter("substreams_sink_postgres_store_flush_duration", "The amount of time spent flushing cache to db (in nanoseconds)")
var LiveFlushSkippedCount = metrics.NewCounter("substreams_sink_postgres_live_flush_skipped_count", "The amount of live flushes skipped because the average flush took longer than the time between blocks")
var FlushedHeadBlockNumber = metrics.NewHeadBlockNumber("substreams_sink_postgres")
var FlushedHeadBlockTimeDrift = metrics.NewHeadTimeDrift("substreams_sink_postgres")
var WriterLockHeld = metrics.NewGauge("substreams_sink_postgres_writer_lock_held", "1 while this sink holds the writer lock (--standby-lock), 0 while it waits as a standby")

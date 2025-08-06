package sink

import (
	"time"

	"github.com/streamingfast/dmetrics"
)

var Metrics = dmetrics.NewSet()

var HeadBlockNumber = Metrics.NewHeadBlockNumber("substreams_sink")
var HeadBlockTimeDrift = Metrics.NewHeadTimeDrift("substreams_sink")

var AvgBlockWaitTime = dmetrics.NewAvgDurationCounter(30*time.Second, time.Second, "Average duration for BlockWaitTime")
var BlockWaitTime = Metrics.NewGauge("substreams_sink_block_wait_time", "The time that the sinks spends waiting for the next block from substreams -- should converge to the block production time of the chain")

var AvgBlockTimeDelta = dmetrics.NewAvgDurationCounter(30*time.Second, time.Second, "Average duration for BlockTimeDelta")
var BlockTimeDelta = Metrics.NewGauge("substreams_sink_block_time_delta", "The difference between the last received block's BlockTime and the previous block's BlockTime -- can be skewed very high when processing older segments with a BlockFilter or skipped outputs")

var AvgLocalProcessingTime = dmetrics.NewAvgDurationCounter(30*time.Second, time.Second, "Average duration for LocalProcessingTime")
var LocalProcessingTime = Metrics.NewGauge("substreams_sink_local_processing_time", "The time that the sinks spends processing the received block")

var MessageSizeBytes = Metrics.NewCounter("substreams_sink_message_size_bytes", "The number of total bytes of message received from the Substreams backend")

var SubstreamsErrorCount = Metrics.NewCounter("substreams_sink_error", "The error count we encountered when interacting with Substreams for which we had to restart the connection loop")
var DataMessageCount = Metrics.NewCounter("substreams_sink_data_message", "The number of data message received")
var ProgressMessageCount = Metrics.NewGauge("substreams_sink_progress_message", "The number of progress message received")
var ProgressMessageLastBlock = Metrics.NewGaugeVec("substreams_sink_progress_message_last_block", []string{"stage"}, "Latest progress reported processed range end block for each stage (not necessarily contiguous)")
var ProgressMessageRunningJobs = Metrics.NewGaugeVec("substreams_sink_progress_message_running_jobs", []string{"stage"}, "Latest reported number of active jobs for each stage")

var ServerEgressBytes = Metrics.NewCounter("substreams_sink_server_egress_bytes", "Total egress bytes from Substreams server (total uncompressed bytes received)")
var ProcessedBytes = Metrics.NewGauge("substreams_sink_processed_bytes", "Total processed bytes on Substreams server")
var ProcessedBlocks = Metrics.NewGauge("substreams_sink_processed_blocks", "Total processed blocks on Substreams server")

var ProgressMessageLastContiguousBlock = Metrics.NewGaugeVec("substreams_sink_progress_message_last_contiguous_block", []string{"stage"}, "Latest progress reported processed end block for the first completed (contiguous) range")
var UndoMessageCount = Metrics.NewCounter("substreams_sink_undo_message", "The number of block undo message received")
var UnknownMessageCount = Metrics.NewCounter("substreams_sink_unknown_message", "The number of unknown message received")

var BackprocessingCompletion = Metrics.NewGauge("substreams_sink_backprocessing_completion", "Determines if backprocessing is completed, which is if we receive a first data message")

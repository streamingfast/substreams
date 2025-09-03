package sink

import (
	"time"

	"github.com/streamingfast/bstream"
	"github.com/streamingfast/dmetrics"
	"github.com/streamingfast/shutter"
	"go.uber.org/zap"
)

type Stats struct {
	*shutter.Shutter

	dataMsgRate       *dmetrics.AvgRatePromCounter
	progressBlockRate *dmetrics.AvgRatePromGauge
	undoMsgRate       *dmetrics.AvgRatePromCounter
	isLive            *bool

	lastBlock bstream.BlockRef
	logger    *zap.Logger
}

func newStats(logger *zap.Logger) *Stats {
	return &Stats{
		Shutter: shutter.New(),

		dataMsgRate:       dmetrics.MustNewAvgRateFromPromCounter(DataMessageCount, 1*time.Second, 30*time.Second, "msg"),
		progressBlockRate: dmetrics.MustNewAvgRateFromPromGauge(ProcessedBlocks, 1*time.Second, 30*time.Second, "block"),
		undoMsgRate:       dmetrics.MustNewAvgRateFromPromCounter(UndoMessageCount, 1*time.Second, 30*time.Second, "msg"),

		lastBlock: unsetBlockRef{},

		logger: logger,
	}
}

func (s *Stats) RecordBlock(block bstream.BlockRef) {
	s.lastBlock = block
}

func (s *Stats) SetLiveness(isLive *bool) {
	s.isLive = isLive
}

func (s *Stats) Start(each time.Duration) {
	if s.IsTerminating() || s.IsTerminated() {
		panic("already shutdown, refusing to start again")
	}

	go func() {
		ticker := time.NewTicker(each)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				s.LogNow()
			case <-s.Terminating():
				return
			}
		}
	}()
}

func (s *Stats) LogNow() {

	fields := []zap.Field{
		zap.Stringer("data_msg_rate", s.dataMsgRate),
		zap.Stringer("undo_msg_rate", s.undoMsgRate),
		zap.Float64("avg_block_wait_time_sec", BlockWaitTime.Get()),
		zap.Float64("avg_block_time_delta_sec", BlockTimeDelta.Get()),
		zap.Float64("avg_local_block_processing_time_sec", LocalProcessingTime.Get()),
		zap.Stringer("last_block", s.lastBlock),
	}
	if s.isLive != nil {
		fields = append(fields,
			zap.Any("progress_block_rate", s.progressBlockRate),
			zap.Any("progress_last_block", dmetrics.NewValuesFromMetric(ProgressMessageLastBlock).Uints("stage")),
			zap.Any("progress_running_jobs", dmetrics.NewValuesFromMetric(ProgressMessageRunningJobs).Uints("stage")),
			zap.Uint64("progress_total_processed_blocks", dmetrics.NewValueFromMetric(ProcessedBlocks, "blocks").ValueUint()),
			zap.Any("progress_last_contiguous_block", dmetrics.NewValuesFromMetric(ProgressMessageLastContiguousBlock).Uints("stage")),
			zap.Bool("is_live", *s.isLive),
		)
	}

	s.logger.Info("substreams stream stats", fields...)
}

func (s *Stats) Close() {
	s.dataMsgRate.SyncNow()
	s.undoMsgRate.SyncNow()
	s.LogNow()

	s.Shutdown(nil)
	s.dataMsgRate.Stop()
	s.undoMsgRate.Stop()
}

type unsetBlockRef struct{}

func (unsetBlockRef) ID() string     { return "" }
func (unsetBlockRef) Num() uint64    { return 0 }
func (unsetBlockRef) String() string { return "None" }

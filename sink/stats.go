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

	lastBlock bstream.BlockRef
	logger    *zap.Logger
}

func newStats(logger *zap.Logger) *Stats {
	return &Stats{
		Shutter: shutter.New(),

		dataMsgRate:       dmetrics.MustNewAvgRateFromPromCounter(DataMessageCount, 1*time.Second, 30*time.Second, "msg"),
		progressBlockRate: dmetrics.MustNewAvgRateFromPromGauge(ProgressMessageTotalProcessedBlocks, 1*time.Second, 30*time.Second, "block"),
		undoMsgRate:       dmetrics.MustNewAvgRateFromPromCounter(UndoMessageCount, 1*time.Second, 30*time.Second, "msg"),

		lastBlock: unsetBlockRef{},

		logger: logger,
	}
}

func (s *Stats) RecordBlock(block bstream.BlockRef) {
	s.lastBlock = block
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

	// Logging fields order is important as it affects the final rendering, we carefully ordered
	// them so the development logs looks nicer.
	s.logger.Info("substreams stream stats",
		zap.Stringer("data_msg_rate", s.dataMsgRate),
		zap.Any("progress_block_rate", s.progressBlockRate),
		zap.Stringer("undo_msg_rate", s.undoMsgRate),

		zap.Any("progress_last_block", dmetrics.NewValuesFromMetric(ProgressMessageLastBlock).Uints("stage")),
		zap.Any("progress_running_jobs", dmetrics.NewValuesFromMetric(ProgressMessageRunningJobs).Uints("stage")),
		zap.Uint64("progress_total_processed_blocks", dmetrics.NewValueFromMetric(ProgressMessageTotalProcessedBlocks, "blocks").ValueUint()),
		zap.Any("progress_last_contiguous_block", dmetrics.NewValuesFromMetric(ProgressMessageLastContiguousBlock).Uints("stage")),

		zap.Stringer("last_block", s.lastBlock),
	)
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

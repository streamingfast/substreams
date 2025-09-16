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

	dataMsgRate                 *dmetrics.AvgRatePromCounter
	progressBlockRate           *dmetrics.AvgRatePromGauge
	undoMsgRate                 *dmetrics.AvgRatePromCounter
	isLive                      *bool
	isIndexOutputProductionMode bool

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

func (s *Stats) SetIndexOutputProductionMode() {
	s.isIndexOutputProductionMode = true
}

func (s *Stats) Start(each time.Duration) {
	if s.IsTerminating() || s.IsTerminated() {
		panic("already shutdown, refusing to start again")
	}

	go func() {

		if shortDelay := 2 * time.Second; shortDelay < each {
			time.Sleep(shortDelay) // first log message comes faster
			s.LogNow()
		}

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

	progressLastBlock := dmetrics.NewValuesFromMetric(ProgressMessageLastBlock).Uints("stage")
	if s.isIndexOutputProductionMode {
		//  'index' modules do not send any output from cached segments (in production mode), so we approximate prometheus's `HeadBlockNumber` and the logs' `last_block` from the previous `LastBlock` seen in a ProgressMessage
		var lowestStageBlock uint64
		for _, block := range progressLastBlock {
			if block < lowestStageBlock || lowestStageBlock == 0 {
				lowestStageBlock = block
			}
		}
		if s.lastBlock == nil || lowestStageBlock > s.lastBlock.Num() {
			s.lastBlock = bstream.NewBlockRef("", lowestStageBlock)
			HeadBlockNumber.SetUint64(lowestStageBlock)
		}

		// isLive is usually populated from a "handleBlockScopedData" which may not get called for 'index' modules
		if s.isLive == nil {
			falseBool := false
			s.isLive = &falseBool
		}

	}
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
			zap.Any("progress_last_block", progressLastBlock),
			zap.Any("progress_running_jobs", dmetrics.NewValuesFromMetric(ProgressMessageRunningJobs).Uints("stage")),
			zap.Uint64("progress_total_processed_blocks", dmetrics.NewValueFromMetric(ProcessedBlocks, "blocks").ValueUint()),
			// zap.Any("progress_last_contiguous_block", dmetrics.NewValuesFromMetric(ProgressMessageLastContiguousBlock).Uints("stage")), // removed this as it is WRONG in many cases
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

package sinker

import (
	"time"

	"github.com/streamingfast/bstream"
	"github.com/streamingfast/dmetrics"
	"github.com/streamingfast/shutter"
	sink "github.com/streamingfast/substreams/sink"
	"go.uber.org/zap"
)

type Stats struct {
	*shutter.Shutter

	dbFlushRate         *dmetrics.AvgRatePromCounter
	dbFlushAvgDuration  *dmetrics.AvgDurationCounter
	flushedRows         *dmetrics.ValueFromMetric
	dbFlushedRowsRate   *dmetrics.AvgRatePromCounter
	handleBlockDuration *dmetrics.AvgDurationCounter
	handleUndoDuration  *dmetrics.AvgDurationCounter
	hasUndoSegments     bool
	lastBlock           bstream.BlockRef
	logger              *zap.Logger
}

func NewStats(logger *zap.Logger) *Stats {
	return &Stats{
		Shutter: shutter.New(),

		dbFlushRate:         dmetrics.MustNewAvgRateFromPromCounter(FlushCount, 1*time.Second, 30*time.Second, "flush"),
		dbFlushAvgDuration:  dmetrics.NewAvgDurationCounter(30*time.Second, dmetrics.InferUnit, "per flush"),
		flushedRows:         dmetrics.NewValueFromMetric(FlushedRowsCount, "rows"),
		dbFlushedRowsRate:   dmetrics.MustNewAvgRateFromPromCounter(FlushedRowsCount, 1*time.Second, 30*time.Second, "flushed rows"),
		handleBlockDuration: dmetrics.NewAvgDurationCounter(30*time.Second, dmetrics.InferUnit, "per block"),
		handleUndoDuration:  dmetrics.NewAvgDurationCounter(30*time.Second, dmetrics.InferUnit, "per undo"),
		logger:              logger,

		lastBlock: unsetBlockRef{},
	}
}

func (s *Stats) RecordBlock(block bstream.BlockRef) {
	s.lastBlock = block
}

func (s *Stats) AverageFlushDuration() time.Duration {
	return s.dbFlushAvgDuration.Average()
}

func (s *Stats) RecordFlushDuration(duration time.Duration) {
	s.dbFlushAvgDuration.AddDuration(duration)
}

func (s *Stats) RecordHandleBlockDuration(duration time.Duration) {
	s.handleBlockDuration.AddDuration(duration)
}

func (s *Stats) RecordHandleUndoDuration(duration time.Duration) {
	s.handleUndoDuration.AddDuration(duration)
	s.hasUndoSegments = true
}

func (s *Stats) Start(each time.Duration, cursor *sink.Cursor) {
	if !cursor.IsBlank() {
		s.lastBlock = cursor.Block()
	}

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
	fields := []zap.Field{
		zap.Stringer("db_flush_rate", s.dbFlushRate),
		zap.Stringer("db_flush_duration_rate", s.dbFlushAvgDuration),
		zap.Stringer("db_flushed_rows_rate", s.dbFlushedRowsRate),
		zap.Stringer("handle_block_duration", s.handleBlockDuration),
	}

	// Only log undo metrics if we've had any undo operations (typically in live mode)
	if s.hasUndoSegments {
		fields = append(fields, zap.Stringer("handle_undo_duration", s.handleUndoDuration))
	}

	fields = append(fields, zap.Stringer("last_block", s.lastBlock))

	s.logger.Info("postgres sink stats", fields...)
}

func (s *Stats) Close() {
	s.Shutdown(nil)
}

type unsetBlockRef struct{}

func (unsetBlockRef) ID() string     { return "" }
func (unsetBlockRef) Num() uint64    { return 0 }
func (unsetBlockRef) String() string { return "<Unset>" }

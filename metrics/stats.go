package metrics

import (
	"github.com/streamingfast/bstream"
	"github.com/streamingfast/dmetrics"
	"github.com/streamingfast/shutter"
	"go.uber.org/zap"
	"time"
)

type Stats interface {
	RecordBlock(ref bstream.BlockRef)
	RecordFlush(elapsed time.Duration)
	RecordOutputCacheHit()
	RecordOutputCacheMiss()
	Start(each time.Duration)
	Shutdown()
}

func NewNoopStats() *noopstats {
	return &noopstats{}
}

type noopstats struct {
}

func (n noopstats) Shutdown() {
}

func (n noopstats) Start(each time.Duration) {
}

func (n noopstats) RecordBlock(ref bstream.BlockRef) {
}

func (n noopstats) RecordFlush(elapsed time.Duration) {
}

func (n noopstats) RecordOutputCacheHit() {
}

func (n noopstats) RecordOutputCacheMiss() {
}

func NewStats(logger *zap.Logger) *stats {
	return &stats{
		Shutter:           shutter.New(),
		blockRate:         dmetrics.NewAvgLocalRateCounter(1*time.Second, "blocks"),
		flushDurationRate: dmetrics.NewAvgLocalRateCounter(1*time.Second, "flush duration"),
		flushCountRate:    dmetrics.NewAvgLocalRateCounter(1*time.Second, "flush count"),
		outputCacheHit:    uint64(0),
		outputCacheMiss:   uint64(0),
		logger:            logger,
	}
}

type stats struct {
	*shutter.Shutter
	blockRate         *dmetrics.LocalCounter
	flushDurationRate *dmetrics.LocalCounter
	flushCountRate    *dmetrics.LocalCounter
	lastBlock         bstream.BlockRef
	logger            *zap.Logger
	outputCacheMiss   uint64
	outputCacheHit    uint64
}

func (s *stats) RecordBlock(ref bstream.BlockRef) {
	s.blockRate.IncByElapsedTime(time.Now())
	//s.blockRate.Inc()
	s.lastBlock = ref
}

func (s *stats) RecordFlush(elapsed time.Duration) {
	s.flushDurationRate.IncBy(elapsed.Nanoseconds())
	s.flushCountRate.Inc()
}

func (s *stats) RecordOutputCacheHit() {
	s.outputCacheHit++
}

func (s *stats) RecordOutputCacheMiss() {
	s.outputCacheMiss++
}

func (s *stats) Start(each time.Duration) {
	s.logger.Info("starting stats service", zap.Duration("runs_each", each))

	//if s.IsTerminating() || s.IsTerminated() {
	//	panic("already shutdown, refusing to start again")
	//}

	go func() {
		ticker := time.NewTicker(each)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				// Logging fields order is important as it affects the final rendering, we carefully ordered
				// them so the development logs looks nicer.
				fields := []zap.Field{
					zap.Stringer("block_rate", s.blockRate),
					zap.Stringer("flush_count", s.flushCountRate),
					zap.Stringer("flush_duration", s.flushDurationRate),
					zap.Uint64("output_cache_hit", s.outputCacheHit),
					zap.Uint64("output_cache_miss", s.outputCacheMiss),
				}

				if s.lastBlock == nil {
					fields = append(fields, zap.String("last_block", "None"))
				} else {
					fields = append(fields, zap.Stringer("last_block", s.lastBlock))
				}

				s.logger.Info("substreams request stats", fields...)
			case <-s.Terminating():
				return
			}
		}
	}()
}
func (s *stats) Shutdown() {
	s.logger.Info("shutting down request stats")
	s.Shutter.Shutdown(nil)
}

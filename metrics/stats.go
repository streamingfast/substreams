package metrics

import (
	"fmt"
	"sync"
	"time"

	"github.com/streamingfast/bstream"
	"github.com/streamingfast/dmetrics"
	"github.com/streamingfast/shutter"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type Stats interface {
	StartParallelProcessing()
	EndParallelProcessing()
	RecordBlock(ref bstream.BlockRef)
	RecordFlush(elapsed time.Duration)
	RecordStoreSquasherProgress(module string, blockNum uint64)
	RecordOutputCacheHit()
	RecordOutputCacheMiss()
	RecordBytesWritten(n uint64)
	RecordBytesRead(n uint64)
	Start(each time.Duration)
	Shutdown()
}

func NewNoopStats() Stats {
	return &noopstats{}
}

type noopstats struct{}

func (n noopstats) StartParallelProcessing()                                   {}
func (n noopstats) EndParallelProcessing()                                     {}
func (n noopstats) Shutdown()                                                  {}
func (n noopstats) Start(each time.Duration)                                   {}
func (n noopstats) RecordBlock(ref bstream.BlockRef)                           {}
func (n noopstats) RecordFlush(elapsed time.Duration)                          {}
func (n noopstats) RecordOutputCacheHit()                                      {}
func (n noopstats) RecordOutputCacheMiss()                                     {}
func (n noopstats) RecordBytesWritten(x uint64)                                {}
func (n noopstats) RecordBytesRead(x uint64)                                   {}
func (n noopstats) RecordStoreSquasherProgress(module string, blockNum uint64) {}

func NewReqStats(logger *zap.Logger) Stats {
	return &stats{
		Shutter:            shutter.New(),
		blockRate:          dmetrics.MustNewAvgRateCounter(1*time.Second, 30*time.Second, "blocks"),
		flushDurationRate:  dmetrics.NewAvgDurationCounter(1*time.Second, time.Second, "flush duration"),
		bytesWrittenRate:   dmetrics.MustNewAvgRateCounter(1*time.Second, 1*time.Second, "bytes written"),
		bytesReadRate:      dmetrics.MustNewAvgRateCounter(1*time.Second, 1*time.Second, "bytes read"),
		outputCacheHit:     uint64(0),
		outputCacheMiss:    uint64(0),
		parallelProcessing: newParallelProcessingStats(),
		logger:             logger,
	}
}

type stats struct {
	*shutter.Shutter
	blockRate          *dmetrics.AvgRateCounter
	flushDurationRate  *dmetrics.AvgDurationCounter
	bytesReadRate      *dmetrics.AvgRateCounter
	bytesWrittenRate   *dmetrics.AvgRateCounter
	lastBlock          bstream.BlockRef
	outputCacheMiss    uint64
	outputCacheHit     uint64
	parallelProcessing *parallelProcessingStats

	logger *zap.Logger
}

func (s *stats) RecordBlock(ref bstream.BlockRef) {
	s.blockRate.Add(1)
	s.lastBlock = ref
}

func (s *stats) RecordBytesWritten(n uint64) {
	s.bytesWrittenRate.Add(n)
}

func (s *stats) RecordBytesRead(n uint64) {
	s.bytesReadRate.Add(n)
}

func (s *stats) RecordFlush(elapsed time.Duration) {
	s.flushDurationRate.AddDuration(elapsed)
}

func (s *stats) RecordOutputCacheHit() {
	s.outputCacheHit++
}

func (s *stats) RecordOutputCacheMiss() {
	s.outputCacheMiss++
}

func (s *stats) StartParallelProcessing() {
	s.parallelProcessing.start(time.Now())
}

func (s *stats) EndParallelProcessing() {
	s.parallelProcessing.stop(time.Now())
}

func (s *stats) RecordStoreSquasherProgress(moduleName string, blockNum uint64) {
	s.parallelProcessing.squashStoreProgress(moduleName, blockNum)
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
				s.logger.Info("substreams request stats", s.getZapFields()...)
			case <-s.Terminating():
				return
			}
		}
	}()
}

func (s *stats) getZapFields() []zap.Field {
	// Logging fields order is important as it affects the final rendering, we carefully ordered
	// them so the development logs looks nicer.
	fields := []zap.Field{
		zap.Stringer("block_rate", s.blockRate),
	}
	if s.lastBlock == nil {
		fields = append(fields, zap.String("last_block", "None"))
	} else {
		fields = append(fields, zap.Stringer("last_block", s.lastBlock))
	}

	fields = append(fields,
		zap.Stringer("flush_duration", s.flushDurationRate),
		zap.Uint64("output_cache_hit", s.outputCacheHit),
		zap.Uint64("output_cache_miss", s.outputCacheMiss),
		zap.Stringer("bytes_written", s.bytesWrittenRate),
		zap.Stringer("bytes_read", s.bytesReadRate),
		zap.Object("parallelProcessing", s.parallelProcessing),
	)

	return fields
}

func (s *stats) Shutdown() {
	s.logger.Info("shutting down request stats")
	s.Shutter.Shutdown(nil)
}

type parallelProcessingStats struct {
	sync.RWMutex

	startOnce      sync.Once
	stopOnce       sync.Once
	storeSquashers map[string]uint64

	startAt *time.Time
	stopAt  *time.Time
}

func newParallelProcessingStats() *parallelProcessingStats {
	return &parallelProcessingStats{
		storeSquashers: map[string]uint64{},
	}
}

func (b *parallelProcessingStats) start(t0 time.Time) {
	b.startOnce.Do(func() {
		b.startAt = &t0
	})
}

func (b *parallelProcessingStats) stop(t1 time.Time) {
	b.startOnce.Do(func() {
		b.stopAt = &t1
	})
}

func (b *parallelProcessingStats) squashStoreProgress(moduleName string, blockNum uint64) {
	b.Lock()
	defer b.Unlock()

	b.storeSquashers[moduleName] = blockNum
}
func (b *parallelProcessingStats) status() string {
	if b.stopAt != nil {
		return "ran"
	}
	if b.startAt != nil {
		return "running"
	}
	return "not_started"
}
func (b *parallelProcessingStats) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	status := b.status()
	enc.AddString("status", status)
	if status == "not_stated" {
		return nil
	}

	if b.startAt != nil {
		enc.AddTime("stated_at", *b.startAt)
		if b.stopAt != nil {
			enc.AddDuration("elapsed", (*b.stopAt).Sub(*b.startAt))
			return nil
		}
		enc.AddDuration("elapsed", time.Since(*b.startAt))
	}

	b.RLock()
	defer b.RUnlock()
	for moduleName, blockNum := range b.storeSquashers {
		key := fmt.Sprintf("store_squash_%s", moduleName)
		enc.AddUint64(key, blockNum)
	}
	return nil
}

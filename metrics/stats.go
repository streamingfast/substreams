package metrics

import (
	"fmt"
	"github.com/streamingfast/bstream"
	"github.com/streamingfast/dmetrics"
	"github.com/streamingfast/shutter"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"sync"
	"time"
)

type Stats interface {
	StartBackProcessing()
	EndBackProcessing()
	RecordBlock(ref bstream.BlockRef)
	RecordFlush(elapsed time.Duration)
	RecordStoreSquasherProgress(module string, blockNum uint64)
	RecordOutputCacheHit()
	RecordOutputCacheMiss()
	Start(each time.Duration)
	Shutdown()
}

func NewNoopStats() Stats {
	return &noopstats{}
}

type noopstats struct {
}

func (n noopstats) StartBackProcessing() {
}

func (n noopstats) EndBackProcessing() {
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

func (n noopstats) RecordStoreSquasherProgress(module string, blockNum uint64) {
}

func NewReqStats(logger *zap.Logger) Stats {
	return &stats{
		Shutter:           shutter.New(),
		blockRate:         dmetrics.NewAvgLocalRateCounter(1*time.Second, "blocks"),
		flushDurationRate: dmetrics.NewAvgLocalRateCounter(1*time.Second, "flush duration"),
		flushCountRate:    dmetrics.NewAvgLocalRateCounter(1*time.Second, "flush count"),
		outputCacheHit:    uint64(0),
		outputCacheMiss:   uint64(0),
		backprocessing:    newBackprocessStats(),
		logger:            logger,
	}
}

type stats struct {
	*shutter.Shutter
	blockRate         *dmetrics.LocalCounter
	flushDurationRate *dmetrics.LocalCounter
	flushCountRate    *dmetrics.LocalCounter
	lastBlock         bstream.BlockRef
	outputCacheMiss   uint64
	outputCacheHit    uint64
	backprocessing    *backprocessStats

	logger *zap.Logger
}

func (s *stats) RecordBlock(ref bstream.BlockRef) {
	s.blockRate.Inc()
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

func (s *stats) StartBackProcessing() {
	s.backprocessing.start(time.Now())
}

func (s *stats) EndBackProcessing() {
	s.backprocessing.stop(time.Now())
}

func (s *stats) RecordStoreSquasherProgress(moduleName string, blockNum uint64) {
	s.backprocessing.squashStoreProgress(moduleName, blockNum)
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
		zap.Stringer("flush_count", s.flushCountRate),
		zap.Stringer("flush_duration", s.flushDurationRate),
		zap.Uint64("output_cache_hit", s.outputCacheHit),
		zap.Uint64("output_cache_miss", s.outputCacheMiss),
		zap.Object("backprocessing", s.backprocessing),
	)

	return fields
}

func (s *stats) Shutdown() {
	s.logger.Info("shutting down request stats")
	s.Shutter.Shutdown(nil)
}

type backprocessStats struct {
	sync.RWMutex

	startOnce      sync.Once
	stopOnce       sync.Once
	storeSquashers map[string]uint64

	startAt *time.Time
	stopAt  *time.Time
}

func newBackprocessStats() *backprocessStats {
	return &backprocessStats{
		storeSquashers: map[string]uint64{},
	}
}

func (b *backprocessStats) start(t0 time.Time) {
	b.startOnce.Do(func() {
		b.startAt = &t0
	})
}

func (b *backprocessStats) stop(t1 time.Time) {
	b.startOnce.Do(func() {
		b.stopAt = &t1
	})
}

func (b *backprocessStats) squashStoreProgress(moduleName string, blockNum uint64) {
	b.Lock()
	defer b.Unlock()

	b.storeSquashers[moduleName] = blockNum
}
func (b *backprocessStats) status() string {
	if b.stopAt != nil {
		return "ran"
	}
	if b.startAt != nil {
		return "running"
	}
	return "not_started"
}
func (b *backprocessStats) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	status := b.status()
	enc.AddString("status", status)
	if status == "not_stated" {
		return nil
	}

	enc.AddTime("stated_at", *b.startAt)
	if b.stopAt != nil {
		enc.AddDuration("elapsed", (*b.stopAt).Sub(*b.startAt))
		return nil
	}

	enc.AddDuration("elapsed", time.Since(*b.startAt))
	b.RLock()
	defer b.RUnlock()
	for moduleName, blockNum := range b.storeSquashers {
		key := fmt.Sprintf("store_squash_%s", moduleName)
		enc.AddUint64(key, blockNum)
	}
	return nil
}

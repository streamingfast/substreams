package metrics

import (
	"fmt"
	"github.com/streamingfast/bstream"
	"github.com/streamingfast/dmetrics"
	"github.com/streamingfast/shutter"
	"go.uber.org/zap"
	"sync"
	"time"
)

type Stats interface {
	StartBackProcessing()
	FinishBackProcessing()
	RecordBlock(ref bstream.BlockRef)
	RecordFlush(elapsed time.Duration)
	RecordStoreSquasherProgress(module string, blockNum uint64)
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

func (n noopstats) StartBackProcessing() {
}

func (n noopstats) FinishBackProcessing() {
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

func NewStats(logger *zap.Logger) *stats {
	return &stats{
		Shutter:           shutter.New(),
		blockRate:         dmetrics.NewAvgLocalRateCounter(1*time.Second, "blocks"),
		flushDurationRate: dmetrics.NewAvgLocalRateCounter(1*time.Second, "flush duration"),
		flushCountRate:    dmetrics.NewAvgLocalRateCounter(1*time.Second, "flush count"),
		outputCacheHit:    uint64(0),
		outputCacheMiss:   uint64(0),
		backprocessing:    false,
		storeSquashers:    map[string]uint64{},
		logger:            logger,
	}
}

type stats struct {
	sync.RWMutex
	*shutter.Shutter
	blockRate         *dmetrics.LocalCounter
	flushDurationRate *dmetrics.LocalCounter
	flushCountRate    *dmetrics.LocalCounter
	lastBlock         bstream.BlockRef
	outputCacheMiss   uint64
	outputCacheHit    uint64
	backprocessing    bool

	storeSquashers map[string]uint64
	logger         *zap.Logger
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
	s.backprocessing = true
}

func (s *stats) FinishBackProcessing() {
	s.backprocessing = false
}

func (s *stats) RecordStoreSquasherProgress(moduleName string, blockNum uint64) {
	s.Lock()
	defer s.Unlock()

	s.storeSquashers[moduleName] = blockNum
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
	s.RLock()
	defer s.RUnlock()
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
		zap.Bool("backprocessing", s.backprocessing),
	)

	if s.backprocessing {
		for moduleName, blockNum := range s.storeSquashers {
			key := fmt.Sprintf("store_squash_%s", moduleName)
			fields = append(fields, zap.Uint64(key, blockNum))
		}
	}

	return fields
}

func (s *stats) Shutdown() {
	s.logger.Info("shutting down request stats")
	s.Shutter.Shutdown(nil)
}

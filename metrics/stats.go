package metrics

import (
	"github.com/streamingfast/bstream"
	"github.com/streamingfast/dmetrics"
	"github.com/streamingfast/shutter"
	"go.uber.org/zap"
	"time"
)

type Stats interface {
	NewBlock(ref bstream.BlockRef)
	Start(each time.Duration)
	Shutdown()
}

func NewNoopStats() Stats {
	return &noopstats{}
}

type noopstats struct {
}

func (n noopstats) Shutdown() {
}

func (n noopstats) Start(each time.Duration) {
}

func (n noopstats) NewBlock(ref bstream.BlockRef) {
}

func NewStats(logger *zap.Logger) Stats {
	return &stats{
		Shutter:   shutter.New(),
		blockRate: dmetrics.NewLocalRateCounter(1*time.Second, "blocks"),
		logger:    logger,
	}
}

type stats struct {
	*shutter.Shutter
	blockRate *dmetrics.LocalCounter
	lastBlock bstream.BlockRef
	logger    *zap.Logger
}

func (s *stats) NewBlock(ref bstream.BlockRef) {
	s.blockRate.Inc()
	s.lastBlock = ref
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
				}

				if s.lastBlock == nil {
					fields = append(fields, zap.String("last_block", "None"))
				} else {
					fields = append(fields, zap.Stringer("last_block", s.lastBlock))
				}

				s.logger.Info("substreams request stats", fields...)
			case <-s.Terminating():
				break
			}
		}
	}()
}
func (s *stats) Shutdown() {
	s.Shutter.Shutdown(nil)
}

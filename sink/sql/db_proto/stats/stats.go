package stats

import (
	"time"

	"github.com/streamingfast/logging/zapx"
	"go.uber.org/zap"
)

type Average struct {
	Duration   []time.Duration
	windowSize int
	title      string
	lastX      int
}

func NewAverage(title string, windowSize int, lastX int) *Average {
	return &Average{
		title:      title,
		windowSize: windowSize,
		lastX:      lastX,
	}
}
func (a *Average) Add(d time.Duration) {
	a.Duration = append(a.Duration, d)
	if len(a.Duration) > a.windowSize {
		a.Duration = a.Duration[1:]
	}
}

func (a *Average) Average() time.Duration {
	if len(a.Duration) == 0 {
		return 0
	}
	var total time.Duration
	for _, d := range a.Duration {
		total += d
	}
	return time.Duration(total / time.Duration(len(a.Duration)))
}

func (a *Average) LastItemsAverage(count int) time.Duration {
	if len(a.Duration) == 0 {
		return 0
	}
	if count <= 0 || count > len(a.Duration) {
		count = len(a.Duration)
	}
	var total int64
	for _, d := range a.Duration[len(a.Duration)-count:] {
		total += d.Nanoseconds()
	}
	return time.Duration(total / int64(count))
}

func (a *Average) Log(logger *zap.Logger) {
	logger.Info(a.title,
		zapx.HumanDuration("average", a.Average()),
		zapx.HumanDuration("last X average", a.LastItemsAverage(a.lastX)),
	)
}

type Stats struct {
	logger                    *zap.Logger
	BlockCount                int
	WaitDurationBetweenBlocks *Average
	BlockProcessingDuration   *Average
	UnmarshallingDuration     *Average
	BlockInsertDuration       *Average
	EntitiesInsertDuration    *Average
	FlushDuration             *Average
	LastBlockProcessAt        time.Time
	TotalProcessingDuration   time.Duration
	TotalDurationBetween      time.Duration
}

func NewStats(logger *zap.Logger) *Stats {
	s := &Stats{
		logger:                    logger,
		WaitDurationBetweenBlocks: NewAverage("   Wait Duration Between Blocks", 250_000, 1000),
		BlockProcessingDuration:   NewAverage("      Block Processing Duration", 250_000, 1000),
		UnmarshallingDuration:     NewAverage("         Unmarshalling Duration", 250_000, 1000),
		BlockInsertDuration:       NewAverage("          Block Insert Duration", 250_000, 1000),
		EntitiesInsertDuration:    NewAverage("       Entities Insert Duration", 250_000, 1000),
		FlushDuration:             NewAverage("                 Flush duration", 1000, 10),
	}

	go func() {
		for {
			time.Sleep(30 * time.Second)
			s.Log()
		}
	}()

	return s
}

func (s *Stats) Log() {
	s.logger.Info("-----------------------------------")

	if s.BlockCount == 0 {
		s.logger.Info("Stats: no blocks processed yet")
	} else {
		s.logger.Info("Stats",
			zap.Int("block_count", s.BlockCount),
			zapx.HumanDuration("Processing Time", s.TotalProcessingDuration),
			zapx.HumanDuration("Total Wait Duration", s.TotalDurationBetween),
			zapx.HumanDuration("Total Duration", s.TotalDurationBetween+s.TotalProcessingDuration),
			zap.Time("Last Block Process At", s.LastBlockProcessAt),
		)

		s.WaitDurationBetweenBlocks.Log(s.logger)
		s.BlockProcessingDuration.Log(s.logger)
		s.UnmarshallingDuration.Log(s.logger)
		s.BlockInsertDuration.Log(s.logger)
		s.EntitiesInsertDuration.Log(s.logger)
		s.FlushDuration.Log(s.logger)
	}

	s.logger.Info("-----------------------------------")
}

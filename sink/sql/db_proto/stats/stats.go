package stats

import (
	"time"

	"github.com/streamingfast/logging/zapx"
	protosql "github.com/streamingfast/substreams/sink/sql/db_proto/sql"
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

// blocksPerSample is what the durations are scaled to before they are printed.
//
// Every sample here is one block's cost, and a per-block mean lands in tenths of a
// microsecond — a column of "0.02ms" and "0.08ms" that no one can weigh against each
// other or against a wall clock. Times a hundred, the same numbers read in milliseconds
// and their ratios are obvious at a glance. It is presentation only: Average and
// LastItemsAverage still return the per-block value that the live-flush heuristic
// compares against a block time.
const blocksPerSample = 100

func (a *Average) Log(logger *zap.Logger) { a.LogAs(logger, a.title) }

// LogAs renders under a caller-supplied title, for a measurement whose name depends on
// which write path is in use.
func (a *Average) LogAs(logger *zap.Logger, title string) {
	logger.Info(title,
		zapx.HumanDuration("per_100_blocks", a.Average()*blocksPerSample),
		zapx.HumanDuration("recent_per_100_blocks", a.LastItemsAverage(a.lastX)*blocksPerSample),
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

	// Progress is how far the download is ahead of the database.
	Progress *Progress

	// lastCommit is the write path's account as of the previous RecordBuffered, so that
	// FlushDuration can be fed the interval rather than the running total.
	lastCommit protosql.WriteStats
}

// RecordBuffered notes what sits between the stream and the database, and folds the
// database's side of it into the flush timing.
//
// With a spool the sink's own flush only queues rows — the commit happens later, on the
// applier's goroutine — so timing that call measures nothing and reads as "the database
// is instant". Feeding the same Average from the segments the applier finished keeps one
// flush timing in the panel that means the same thing in both modes: how long it took to
// get one block into the database.
func (s *Stats) RecordBuffered(held int, buffered protosql.WriteStats, spooling bool) {
	s.Progress.RecordBuffered(held, buffered, spooling)

	if !spooling {
		return
	}

	if blocks := buffered.Blocks - s.lastCommit.Blocks; blocks > 0 {
		s.FlushDuration.Add((buffered.ApplyDuration - s.lastCommit.ApplyDuration) / time.Duration(blocks))
	}
	s.lastCommit = buffered
}

func NewStats(logger *zap.Logger, blockBatchSize int) *Stats {
	s := &Stats{
		logger:                    logger,
		Progress:                  NewProgress(blockBatchSize),
		WaitDurationBetweenBlocks: NewAverage("   Wait Duration Between Blocks", 250_000, 1000),
		BlockProcessingDuration:   NewAverage("      Block Processing Duration", 250_000, 1000),
		UnmarshallingDuration:     NewAverage("         Unmarshalling Duration", 250_000, 1000),
		BlockInsertDuration:       NewAverage("          Block Insert Duration", 250_000, 1000),
		EntitiesInsertDuration:    NewAverage("          Message Walk Duration", 250_000, 1000),
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
		// Named for what it measures on the path actually in use. With a spool, Replay
		// writes the rows to disk and nothing is inserted into a database here.
		if s.Progress.Spooling() {
			s.BlockInsertDuration.LogAs(s.logger, "           Spool Write Duration")
		} else {
			s.BlockInsertDuration.Log(s.logger)
		}

		// The walk builds the row values and touches neither the database nor the spool,
		// so it is named for the walk in both modes.
		s.EntitiesInsertDuration.LogAs(s.logger, "          Message Walk Duration")

		s.FlushDuration.Log(s.logger)
		s.Progress.Log(s.logger)
	}

	s.logger.Info("-----------------------------------")
}

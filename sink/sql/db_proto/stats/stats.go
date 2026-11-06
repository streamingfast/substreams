package stats

import (
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/streamingfast/logging/zapx"
	protosql "github.com/streamingfast/substreams/sink/sql/db_proto/sql"
	"go.uber.org/zap"
)

// Average is a rolling window of per-block durations.
//
// It is appended to from the sinker's goroutine and read from the logging ticker's, so it
// locks. A slice is not safe to grow and range over at the same time: the reader can pick
// up a header whose length has been published but whose backing array has not, and index
// past the end of the old one.
type Average struct {
	mutex      sync.Mutex
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
	a.mutex.Lock()
	defer a.mutex.Unlock()

	a.Duration = append(a.Duration, d)
	if len(a.Duration) > a.windowSize {
		a.Duration = a.Duration[1:]
	}
}

func (a *Average) Average() time.Duration {
	a.mutex.Lock()
	defer a.mutex.Unlock()

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
	a.mutex.Lock()
	defer a.mutex.Unlock()

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

// Samples copies the window out, for a caller that wants the values rather than a mean.
func (a *Average) Samples() []time.Duration {
	a.mutex.Lock()
	defer a.mutex.Unlock()

	return slices.Clone(a.Duration)
}

// blocksPerSample is what the durations are scaled to before they are printed.
//
// Every sample here is one block's cost, and a per-block mean lands in tenths of a
// microsecond — a column of "0.02ms" and "0.08ms" that no one can weigh against each
// other or against a wall clock. Scaled up, the same numbers read in milliseconds and
// their ratios are obvious at a glance. It is presentation only: Average and
// LastItemsAverage still return the per-block value that the live-flush heuristic
// compares against a block time.
const blocksPerSample = 1000

// The field names carry the scale, derived from it rather than written out, so the two
// cannot drift apart when the constant is retuned.
var (
	scaledField       = fmt.Sprintf("per_%d_blocks", blocksPerSample)
	scaledRecentField = fmt.Sprintf("recent_per_%d_blocks", blocksPerSample)
)

func (a *Average) Log(logger *zap.Logger) { a.LogAs(logger, a.title) }

// LogAs renders under a caller-supplied title, for a measurement whose name depends on
// which write path is in use.
func (a *Average) LogAs(logger *zap.Logger, title string) {
	logger.Info(title,
		zapx.HumanDuration(scaledField, a.Average()*blocksPerSample),
		zapx.HumanDuration(scaledRecentField, a.LastItemsAverage(a.lastX)*blocksPerSample),
	)
}

type Stats struct {
	logger                    *zap.Logger
	WaitDurationBetweenBlocks *Average
	BlockProcessingDuration   *Average
	UnmarshallingDuration     *Average
	BlockInsertDuration       *Average
	EntitiesInsertDuration    *Average
	FlushDuration             *Average

	// The running totals are written per block from the sinker's goroutine and read from
	// the logging ticker's, so they are guarded too. LastBlockProcessAt is three words, so
	// a reader can otherwise catch a wall clock and a monotonic reading from either side
	// of the same write.
	totalsMutex             sync.Mutex
	blockCount              int
	lastBlockProcessAt      time.Time
	totalProcessingDuration time.Duration
	totalDurationBetween    time.Duration
	totalQuotaWait          time.Duration

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

	// The baseline advances only when a sample is taken from it. A snapshot can catch the
	// applier having counted a segment before its blocks land, and moving the baseline past
	// that one would drop its applying time from every interval that follows.
	if blocks := buffered.Blocks - s.lastCommit.Blocks; blocks > 0 {
		s.FlushDuration.Add((buffered.ApplyDuration - s.lastCommit.ApplyDuration) / time.Duration(blocks))
		s.lastCommit = buffered
	}
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

// RecordBlockProcessed folds one block's wall clock into the running totals.
//
// held is the part of it the sinker spent blocked on the spool's disk quota. That is the
// database refusing more work, not the cost of the block, and leaving it inside the
// processing figure makes "processing" grow precisely when the database slows down —
// while the wait between blocks shrinks to nothing, because the stream was never what
// the sinker was waiting for. It is reported on its own instead.
func (s *Stats) RecordBlockProcessed(elapsed, held time.Duration) {
	processing := max(elapsed-held, 0)

	s.BlockProcessingDuration.Add(processing)

	s.totalsMutex.Lock()
	defer s.totalsMutex.Unlock()

	s.lastBlockProcessAt = time.Now()
	s.totalProcessingDuration += processing
	s.totalQuotaWait += held
}

// RecordBlockReceived notes a block arriving, and how long the sinker waited on the
// stream for it. It reports whether any block had been seen before this one.
func (s *Stats) RecordBlockReceived() (waited time.Duration, first bool) {
	s.totalsMutex.Lock()
	defer s.totalsMutex.Unlock()

	first = s.blockCount == 0
	s.blockCount++
	if first {
		return 0, true
	}

	waited = time.Since(s.lastBlockProcessAt)
	s.totalDurationBetween += waited

	return waited, false
}

// Start marks the sinker as running, so the wait before the first block is measured from
// here rather than from the zero time.
func (s *Stats) Start() {
	s.totalsMutex.Lock()
	defer s.totalsMutex.Unlock()

	s.lastBlockProcessAt = time.Now()
}

// BlockCount is how many blocks have reached the sinker.
func (s *Stats) BlockCount() int {
	s.totalsMutex.Lock()
	defer s.totalsMutex.Unlock()

	return s.blockCount
}

func (s *Stats) Log() {
	s.logger.Info("-----------------------------------")

	s.totalsMutex.Lock()
	blockCount, lastBlock := s.blockCount, s.lastBlockProcessAt
	processing, between, quotaWait := s.totalProcessingDuration, s.totalDurationBetween, s.totalQuotaWait
	s.totalsMutex.Unlock()

	if blockCount == 0 {
		s.logger.Info("Stats: no blocks processed yet")
	} else {
		fields := []zap.Field{
			zap.Int("block_count", blockCount),
			zapx.HumanDuration("Processing Time", processing),
			zapx.HumanDuration("Total Wait Duration", between),
		}
		// Only when there is one, so a run that never filled its spool is not told about
		// a category that did not apply to it.
		if quotaWait > 0 {
			fields = append(fields, zapx.HumanDuration("Held By Database", quotaWait))
		}
		fields = append(fields,
			zapx.HumanDuration("Total Duration", between+processing+quotaWait),
			zap.Time("Last Block Process At", lastBlock),
		)

		s.logger.Info("Stats", fields...)

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

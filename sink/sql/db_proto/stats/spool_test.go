package stats

import (
	"testing"
	"time"

	protosql "github.com/streamingfast/substreams/sink/sql/db_proto/sql"
	"github.com/streamingfast/substreams/sink/sql/db_proto/sql/spool"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func rendered(logs *observer.ObservedLogs) []string {
	var out []string
	for _, entry := range logs.All() {
		out = append(out, entry.Message)
	}

	return out
}

// committed is a snapshot of a spool that has applied something.
func committed(segments, blocks int64, applying time.Duration) protosql.WriteStats {
	return protosql.WriteStats{Stats: spool.Stats{
		Segments: segments, Blocks: blocks, Rows: 10, ApplyDuration: applying, ApplierBusy: time.Second,
	}}
}

// logAfter renders the panel as though elapsed had passed since the previous snapshot.
// Rates are differenced between ticks, so a Log with no interval behind it says nothing.
func logAfter(t *testing.T, progress *Progress, elapsed time.Duration) *observer.ObservedLogs {
	t.Helper()

	core, logs := observer.New(zapcore.InfoLevel)
	progress.mutex.Lock()
	progress.previousAt = time.Now().Add(-elapsed)
	progress.mutex.Unlock()
	progress.Log(zap.New(core))

	return logs
}

func fields(t *testing.T, logs *observer.ObservedLogs, title string) map[string]any {
	t.Helper()

	entries := logs.FilterMessage(title).All()
	require.Len(t, entries, 1, "expected exactly one %q line in %v", title, rendered(logs))

	return entries[0].ContextMap()
}

func TestNothingSpoolingKeepsThePanelToTheProgressLine(t *testing.T) {
	progress := NewProgress(10)
	progress.RecordDownloaded(100)
	progress.RecordBuffered(1, protosql.WriteStats{}, false)

	logs := logAfter(t, progress, 30*time.Second)

	require.NotEmpty(t, rendered(logs), "the progress line itself must still be there")
	for _, title := range rendered(logs) {
		require.NotContains(t, title, "Spool")
	}
}

func TestSpoolingRendersTheApplierPanel(t *testing.T) {
	progress := NewProgress(10)
	progress.RecordDownloaded(100)
	progress.RecordBuffered(0, committed(3, 30, time.Second), true)

	titles := rendered(logAfter(t, progress, 30*time.Second))

	require.Contains(t, titles, "                  Spool applier")
	require.Contains(t, titles, "               Spool throughput")
	require.Contains(t, titles, "                 Spool pressure")
}

// The panel's whole reason for existing is the delta between two ticks: a lifetime
// average reports a healthy rate straight through a stall.
func TestRatesAreDifferencedBetweenTicks(t *testing.T) {
	progress := NewProgress(10)
	progress.RecordDownloaded(100)

	first := protosql.WriteStats{Stats: spool.Stats{
		Segments: 10, Blocks: 1000, Rows: 5000, Bytes: 1 << 20, ApplyDuration: 10 * time.Second,
	}}
	progress.RecordBuffered(0, first, true)
	logAfter(t, progress, time.Minute)

	// One more minute: 2 segments, 400 blocks, 1200 rows, 4s of applying.
	second := protosql.WriteStats{Stats: spool.Stats{
		Segments: 12, Blocks: 1400, Rows: 6200, Bytes: 3 << 20, ApplyDuration: 14 * time.Second,
	}}
	progress.RecordBuffered(0, second, true)
	applier := fields(t, logAfter(t, progress, time.Minute), "                  Spool applier")

	require.Equal(t, int64(12), applier["segments"], "the count is cumulative")
	require.Equal(t, "2.0 seg/min", applier["rate"], "the rate is not")
	require.Equal(t, int64(200), applier["blocks_per_seg"], "400 blocks over 2 segments")
	require.Equal(t, int64(600), applier["rows_per_seg"], "1200 rows over 2 segments")
	require.Equal(t, "2s", applier["apply_duration"], "4s of applying over 2 segments")

	throughput := fields(t, logAfter(t, progress, time.Minute), "               Spool throughput")
	require.Contains(t, throughput["rows"], "(6,200 total)", "the total stays cumulative")
	require.Equal(t, "3.0 MiB", throughput["spooled_bytes"], "bytes are cumulative; only the rate is differenced")
}

// The applier's occupancy is the one number that says whether the database or the stream
// is the limit, so its arithmetic is worth pinning down rather than assuming.
func TestApplierBusyIsTheShareOfTheTickSpentApplying(t *testing.T) {
	progress := NewProgress(10)
	progress.RecordDownloaded(100)

	progress.RecordBuffered(0, protosql.WriteStats{}, true)
	busy := protosql.WriteStats{Stats: spool.Stats{
		Segments: 1, Blocks: 10, ApplierBusy: 30 * time.Second, ApplierIdle: 10 * time.Second,
	}}
	progress.RecordBuffered(0, busy, true)

	require.Equal(t, "75.0%", fields(t, logAfter(t, progress, time.Minute), "                 Spool pressure")["applier_busy"])
}

// Reaching the chain head closes the spool and switches to direct inserts. From there the
// rates describe an applier that has stopped, so only the totals survive.
func TestClosingTheSpoolReducesThePanelToItsTotals(t *testing.T) {
	progress := NewProgress(10)
	progress.RecordDownloaded(100)
	progress.RecordBuffered(0, committed(3, 30, time.Second), true)

	// Closing drains what was queued, so the driver's parting snapshot holds more than
	// the last spooling one did. Those segments have to reach the totals.
	progress.RecordBuffered(1, committed(9, 90, 3*time.Second), false)

	logs := logAfter(t, progress, 30*time.Second)
	titles := rendered(logs)
	require.Contains(t, titles, "                 Spool (closed)")
	require.NotContains(t, titles, "                  Spool applier")
	require.NotContains(t, titles, "                 Spool pressure")

	require.Equal(t, int64(9), fields(t, logs, "                 Spool (closed)")["segments"],
		"the totals include the drain that closing performed")
}

// A driver that never spooled must not be handed the closed-spool line.
func TestNeverSpooledNeverPrintsTheClosedLine(t *testing.T) {
	progress := NewProgress(10)
	progress.RecordDownloaded(100)
	progress.RecordBuffered(1, protosql.WriteStats{}, false)

	require.NotContains(t, rendered(logAfter(t, progress, 30*time.Second)), "                 Spool (closed)")
}

// With a spool, Database.Flush returns before anything reaches the server. The timing has
// to come from what the applier committed, or the panel reports a database that is
// instant for the whole backfill.
func TestFlushDurationIsFedByTheApplierWhileSpooling(t *testing.T) {
	stats := NewStats(zap.NewNop(), 10)

	// The counters start at zero with the process, so the first snapshot is itself an
	// interval: 1s of applying over the 10 blocks it committed.
	stats.RecordBuffered(0, committed(1, 10, time.Second), true)
	require.Equal(t, []time.Duration{100 * time.Millisecond}, stats.FlushDuration.Duration)

	stats.RecordBuffered(0, committed(2, 20, 3*time.Second), true)
	require.Equal(t, []time.Duration{100 * time.Millisecond, 200 * time.Millisecond}, stats.FlushDuration.Duration,
		"the second interval is 2s over 10 blocks, not the running total")
}

// A snapshot caught between the applier's counter updates has a segment whose blocks have
// not landed. Advancing the baseline past it would drop that segment's cost for good.
func TestFlushDurationDefersASnapshotWithNoNewBlocks(t *testing.T) {
	stats := NewStats(zap.NewNop(), 10)

	stats.RecordBuffered(0, committed(1, 10, time.Second), true)
	require.Len(t, stats.FlushDuration.Duration, 1)

	// Segment counted, its blocks and duration not yet.
	stats.RecordBuffered(0, committed(2, 10, time.Second), true)
	require.Len(t, stats.FlushDuration.Duration, 1, "nothing to measure yet")

	// The rest lands: the whole interval is still measured, not just what came after.
	stats.RecordBuffered(0, committed(2, 20, 3*time.Second), true)
	require.Equal(t, []time.Duration{100 * time.Millisecond, 200 * time.Millisecond}, stats.FlushDuration.Duration)
}

func TestFlushDurationIgnoresTheSpoolWhenNothingSpools(t *testing.T) {
	stats := NewStats(zap.NewNop(), 10)

	stats.RecordBuffered(5, protosql.WriteStats{}, false)
	require.Empty(t, stats.FlushDuration.Duration, "flushHolding owns the timing when the write is inline")
}

// A per-block mean lands in tenths of a microsecond, which no one can weigh against
// anything. The stored value stays per-block, because the live-flush heuristic compares
// it against a block time; only the rendering scales.
func TestAveragesRenderPerHundredBlocks(t *testing.T) {
	core, logs := observer.New(zapcore.InfoLevel)

	average := NewAverage("title", 100, 10)
	average.Add(60 * time.Microsecond)
	average.Log(zap.New(core))

	require.Equal(t, "6.00ms", logs.All()[0].ContextMap()["per_100_blocks"])
	require.Equal(t, 60*time.Microsecond, average.Average(), "the stored value is untouched")
}

// With a spool, Replay writes rows to disk; nothing is inserted into a database there.
func TestInsertTimingIsNamedForThePathInUse(t *testing.T) {
	for _, test := range []struct {
		spooling bool
		title    string
	}{
		{spooling: true, title: "           Spool Write Duration"},
		{spooling: false, title: "          Block Insert Duration"},
	} {
		core, logs := observer.New(zapcore.InfoLevel)
		stats := NewStats(zap.New(core), 10)
		stats.BlockCount = 1
		stats.RecordBuffered(0, committed(1, 10, time.Second), test.spooling)
		stats.Log()

		require.Contains(t, rendered(logs), test.title)
	}
}

// The walk builds row values and touches neither the database nor the spool, so it is
// named for the walk whichever path is in use.
func TestWalkTimingIsNamedForTheWalkInBothModes(t *testing.T) {
	for _, spooling := range []bool{true, false} {
		core, logs := observer.New(zapcore.InfoLevel)
		stats := NewStats(zap.New(core), 10)
		stats.BlockCount = 1
		stats.RecordBuffered(0, committed(1, 10, time.Second), spooling)
		stats.Log()

		require.Contains(t, rendered(logs), "          Message Walk Duration")
		require.NotContains(t, rendered(logs), "       Entities Insert Duration")
	}
}

// pgx owns the connection, so the PostgreSQL path cannot count its own socket. Printing
// that zero reads as a database receiving nothing while a COPY is in flight.
func TestWireBytesAreOmittedWhenTheDriverCannotCountThem(t *testing.T) {
	progress := NewProgress(10)
	progress.RecordDownloaded(100)
	progress.RecordBuffered(0, committed(3, 30, time.Second), true)

	throughput := fields(t, logAfter(t, progress, 30*time.Second), "               Spool throughput")
	require.NotContains(t, throughput, "db_bytes")
	require.NotContains(t, throughput, "db_write_rate")

	// A driver that does measure it still reports it.
	counted := committed(3, 30, time.Second)
	counted.DatabaseBytesWritten = 4 << 30
	progress = NewProgress(10)
	progress.RecordDownloaded(100)
	progress.RecordBuffered(0, counted, true)

	throughput = fields(t, logAfter(t, progress, 30*time.Second), "               Spool throughput")
	require.Equal(t, "4.0 GiB", throughput["db_bytes"])
}

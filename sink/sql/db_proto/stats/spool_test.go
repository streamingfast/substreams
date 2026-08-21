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

func TestNothingSpoolingKeepsThePanelToTheProgressLine(t *testing.T) {
	core, logs := observer.New(zapcore.InfoLevel)
	progress := NewProgress(10)

	progress.RecordDownloaded(100)
	progress.RecordBuffered(1, protosql.WriteStats{}, false)
	progress.Log(zap.New(core))

	for _, title := range rendered(logs) {
		require.NotContains(t, title, "Spool")
	}
}

func TestSpoolingRendersTheApplierPanel(t *testing.T) {
	core, logs := observer.New(zapcore.InfoLevel)
	progress := NewProgress(10)

	progress.RecordDownloaded(100)
	progress.RecordBuffered(0, committed(3, 30, time.Second), true)
	progress.Log(zap.New(core))

	require.Contains(t, rendered(logs), "                  Spool applier")
	require.Contains(t, rendered(logs), "               Spool throughput")
	require.Contains(t, rendered(logs), "                 Spool pressure")
}

// Reaching the chain head closes the spool and switches to direct inserts. From there the
// rates describe an applier that has stopped, so only the totals survive.
func TestClosingTheSpoolReducesThePanelToItsTotals(t *testing.T) {
	core, logs := observer.New(zapcore.InfoLevel)
	progress := NewProgress(10)

	progress.RecordDownloaded(100)
	progress.RecordBuffered(0, committed(3, 30, time.Second), true)
	progress.RecordBuffered(1, protosql.WriteStats{}, false)
	progress.Log(zap.New(core))

	titles := rendered(logs)
	require.Contains(t, titles, "                 Spool (closed)")
	require.NotContains(t, titles, "                  Spool applier")
	require.NotContains(t, titles, "                 Spool pressure")

	closed := logs.FilterMessage("                 Spool (closed)").All()[0].ContextMap()
	require.Equal(t, int64(3), closed["segments"], "the totals are the last true reading, not zero")
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
	core, logs := observer.New(zapcore.InfoLevel)
	progress := NewProgress(10)

	progress.RecordDownloaded(100)
	progress.RecordBuffered(0, committed(3, 30, time.Second), true)
	progress.Log(zap.New(core))

	throughput := logs.FilterMessage("               Spool throughput").All()[0].ContextMap()
	require.NotContains(t, throughput, "db_bytes")
	require.NotContains(t, throughput, "db_write_rate")

	// A driver that does measure it still reports it.
	core, logs = observer.New(zapcore.InfoLevel)
	progress = NewProgress(10)
	counted := committed(3, 30, time.Second)
	counted.DatabaseBytesWritten = 4 << 30
	progress.RecordDownloaded(100)
	progress.RecordBuffered(0, counted, true)
	progress.Log(zap.New(core))

	require.Equal(t, "4.0 GiB", logs.FilterMessage("               Spool throughput").All()[0].ContextMap()["db_bytes"])
}

package stats

import (
	"testing"

	protosql "github.com/streamingfast/substreams/sink/sql/db_proto/sql"
	"github.com/streamingfast/substreams/sink/sql/db_proto/sql/spool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestProgressBlocksAhead(t *testing.T) {
	progress := NewProgress(25)

	assert.Equal(t, uint64(0), progress.BlocksAhead(), "nothing seen yet")

	// Before the first commit there is no applied mark. Subtracting zero from the block
	// number would report the whole chain height as backlog and warn on every startup.
	progress.RecordDownloaded(20_000_050)
	progress.RecordBuffered(3, protosql.WriteStats{}, false)
	assert.Equal(t, uint64(3), progress.BlocksAhead(), "with nothing applied, the gap is what is buffered")

	progress.RecordDownloaded(20_000_100)
	progress.RecordApplied(20_000_075)
	assert.Equal(t, uint64(25), progress.BlocksAhead())

	// The applied mark can momentarily equal the downloaded one, and must never report
	// a negative gap as a huge unsigned number.
	progress.RecordApplied(20_000_100)
	assert.Equal(t, uint64(0), progress.BlocksAhead())

	progress.RecordApplied(20_000_200)
	assert.Equal(t, uint64(0), progress.BlocksAhead(), "applied ahead of downloaded is still no gap")
}

func TestProgressPeakIsRetained(t *testing.T) {
	progress := NewProgress(25)

	progress.RecordApplied(400)
	progress.RecordDownloaded(1000)
	require.Equal(t, uint64(600), progress.BlocksAhead())

	progress.RecordApplied(995)
	assert.Equal(t, uint64(5), progress.BlocksAhead())
	assert.Equal(t, uint64(600), progress.peakBlocksAhead.Load(), "the peak is what tells the operator it backed up earlier")
}

func TestProgressWarnsOnlyWhenTheBufferStopsLookingLikeAWorkingSet(t *testing.T) {
	core, logs := observer.New(zapcore.DebugLevel)
	logger := zap.New(core)

	// Threshold is 4 batches, floored at 100.
	progress := NewProgress(500)

	progress.RecordApplied(1500)
	progress.RecordDownloaded(3000) // 1500 behind, under 2000
	progress.RecordBuffered(1500, protosql.WriteStats{}, false)
	progress.Log(logger)

	require.Equal(t, 1, logs.Len())
	assert.Equal(t, zapcore.InfoLevel, logs.All()[0].Level, "a few batches in flight is normal")

	progress.RecordApplied(500) // 2500 behind, over 2000
	progress.RecordBuffered(2500, protosql.WriteStats{}, false)
	progress.Log(logger)

	// The spool lines follow the progress one, so pick the warning out rather than
	// counting entries.
	warnings := logs.FilterLevelExact(zapcore.WarnLevel).All()
	require.Len(t, warnings, 1)
	entry := warnings[0]
	assert.Contains(t, entry.Message, "falling behind")

	fields := entry.ContextMap()
	assert.Equal(t, uint64(2500), fields["blocks_ahead"])
	assert.Equal(t, uint64(3000), fields["downloaded_through"])
	assert.Equal(t, uint64(500), fields["applied_through"])
	assert.NotContains(t, fields, "buffered_on_disk", "nothing is on disk when nothing spools")
}

func TestProgressStaysQuietBeforeTheFirstBlock(t *testing.T) {
	core, logs := observer.New(zapcore.DebugLevel)

	NewProgress(25).Log(zap.New(core))

	assert.Equal(t, 0, logs.Len(), "nothing useful to say before the first block arrives")
}

// TestProgressResumeBlockAvoidsStartupBacklog covers the shape of a real restart: the
// cursor puts the sink 20 million blocks in, and the first block downloaded must not be
// reported as 20 million blocks of backlog.
func TestProgressResumeBlockAvoidsStartupBacklog(t *testing.T) {
	core, logs := observer.New(zapcore.DebugLevel)
	logger := zap.New(core)

	progress := NewProgress(25)
	progress.SetResumeBlock(20_000_000)

	progress.RecordDownloaded(20_000_010)
	progress.RecordBuffered(10, protosql.WriteStats{}, false)
	progress.Log(logger)

	require.Equal(t, uint64(10), progress.BlocksAhead())
	require.Equal(t, 1, logs.Len())
	assert.Equal(t, zapcore.InfoLevel, logs.All()[0].Level, "resuming mid-chain is not a backlog")
}

// TestFallingBehindMeasuresTheSpoolAgainstItsQuota covers the warning that fired through
// the whole sparse start of every large backfill.
//
// A block count cannot say whether a spool is coping: the early blocks of a chain carry
// almost no rows, so the spool spans millions of them holding a few hundred kilobytes of
// an eight gigabyte budget. What says it is how full the spool is against what it was
// given.
func TestFallingBehindMeasuresTheSpoolAgainstItsQuota(t *testing.T) {
	t.Run("a huge block span holding almost nothing is not behind", func(t *testing.T) {
		progress := NewProgress(10)
		progress.RecordApplied(1)
		progress.RecordDownloaded(5_000_000)
		progress.RecordBuffered(0, spooled(4_999_999, 150<<10, 8<<30), true)

		require.False(t, progress.fallingBehind(progress.BlocksAhead(), 8<<30))
	})

	t.Run("past half the quota is behind", func(t *testing.T) {
		progress := NewProgress(10)
		progress.RecordApplied(1)
		progress.RecordDownloaded(1000)
		progress.RecordBuffered(0, spooled(999, 5<<30, 8<<30), true)

		require.True(t, progress.fallingBehind(progress.BlocksAhead(), 8<<30))

		core, logs := observer.New(zapcore.DebugLevel)
		progress.Log(zap.New(core))

		fields := logs.All()[0].ContextMap()
		require.Equal(t, "5.0 GiB", fields["buffered_on_disk"])
		require.Equal(t, "62.5%", fields["quota_used"])
	})

	t.Run("without a spool the block count still decides", func(t *testing.T) {
		progress := NewProgress(10)
		progress.RecordApplied(1)
		progress.RecordDownloaded(5000)
		progress.RecordBuffered(4999, protosql.WriteStats{}, false)

		require.True(t, progress.fallingBehind(progress.BlocksAhead(), 0))
	})

	// The quota outlives the spool: after the switch to direct inserts nothing is
	// buffered, and comparing a permanent zero against it would mean the sink could never
	// report falling behind again for the rest of the run.
	t.Run("once the spool is closed the block count decides again", func(t *testing.T) {
		progress := NewProgress(10)
		progress.RecordApplied(1)
		progress.RecordDownloaded(5000)
		progress.RecordBuffered(4999, protosql.WriteStats{}, false)

		require.True(t, progress.fallingBehind(progress.BlocksAhead(), 8<<30))
	})
}

// spooled builds the snapshot a spool holding this much would report.
func spooled(blocks, bytes, quota int64) protosql.WriteStats {
	return protosql.WriteStats{Stats: spool.Stats{BlocksBuffered: blocks, BytesOnDisk: bytes, Quota: quota}}
}

package stats

import (
	"testing"

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
	progress.RecordBuffered(3, 0)
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
	progress.RecordBuffered(1500, 0)
	progress.Log(logger)

	require.Equal(t, 1, logs.Len())
	assert.Equal(t, zapcore.InfoLevel, logs.All()[0].Level, "a few batches in flight is normal")

	progress.RecordApplied(500) // 2500 behind, over 2000
	progress.RecordBuffered(2500, 4<<20)
	progress.Log(logger)

	require.Equal(t, 2, logs.Len())
	entry := logs.All()[1]
	assert.Equal(t, zapcore.WarnLevel, entry.Level)
	assert.Contains(t, entry.Message, "falling behind")

	fields := entry.ContextMap()
	assert.Equal(t, uint64(2500), fields["blocks_ahead"])
	assert.Equal(t, uint64(3000), fields["downloaded_through"])
	assert.Equal(t, uint64(500), fields["applied_through"])
	assert.Equal(t, "4.0MiB", fields["buffered_on_disk"])
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
	progress.RecordBuffered(10, 0)
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
		progress.SetBufferQuota(8 << 30)
		progress.RecordApplied(1)
		progress.RecordDownloaded(5_000_000)
		progress.RecordBuffered(4_999_999, 150<<10)

		require.False(t, progress.fallingBehind(progress.BlocksAhead()))
	})

	t.Run("past half the quota is behind", func(t *testing.T) {
		progress := NewProgress(10)
		progress.SetBufferQuota(8 << 30)
		progress.RecordApplied(1)
		progress.RecordDownloaded(1000)
		progress.RecordBuffered(999, 5<<30)

		require.True(t, progress.fallingBehind(progress.BlocksAhead()))
	})

	t.Run("without a spool the block count still decides", func(t *testing.T) {
		progress := NewProgress(10)
		progress.RecordApplied(1)
		progress.RecordDownloaded(5000)
		progress.RecordBuffered(4999, 0)

		require.True(t, progress.fallingBehind(progress.BlocksAhead()))
	})
}

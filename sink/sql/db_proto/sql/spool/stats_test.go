package spool

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

// countingCodec writes nothing the applier reads back: this exercises the spool's own
// accounting, which is derived from the manifest and never from the rows themselves.
type countingCodec struct{}

func (countingCodec) Format() Format { return FormatValues }

func (countingCodec) OpenSegment(dir string) (SegmentWriter, error) {
	return &countingWriter{dir: dir, rows: map[string]int64{}}, nil
}

func (countingCodec) Verify(string, *Manifest) error { return nil }

type countingWriter struct {
	dir   string
	rows  map[string]int64
	order []string
	bytes int64
}

func (w *countingWriter) WriteRow(table string, values []any) error {
	if _, seen := w.rows[table]; !seen {
		w.order = append(w.order, table)
	}
	w.rows[table]++
	w.bytes += int64(len(values))

	return nil
}

func (w *countingWriter) PendingBytes() int64 { return w.bytes }

func (w *countingWriter) Seal(manifest *Manifest) error {
	for _, table := range w.order {
		file := table + ".values"
		if err := os.WriteFile(filepath.Join(w.dir, file), []byte{}, 0o644); err != nil {
			return err
		}
		manifest.Tables = append(manifest.Tables, TableRecord{
			Name:  table,
			File:  file,
			Rows:  w.rows[table],
			Bytes: w.rows[table],
		})
	}

	return nil
}

func (w *countingWriter) Discard() {}

type recordingApplier struct{ applied []*Manifest }

func (a *recordingApplier) EnsureSchema(context.Context) error { return nil }

func (a *recordingApplier) AlreadyApplied(context.Context, *Manifest) (bool, error) {
	return false, nil
}

func (a *recordingApplier) Apply(_ context.Context, _ string, manifest *Manifest) error {
	a.applied = append(a.applied, manifest)

	return nil
}

func newTestSpool(t *testing.T, options Options) (*Spool, *recordingApplier) {
	t.Helper()

	applier := &recordingApplier{}
	options.Dir = t.TempDir()
	// The idle timer would seal on its own schedule and make the seal counts racy.
	options.MaxIdle = -1

	spool, err := New(context.Background(), options, countingCodec{}, applier, "test", zap.NewNop())
	require.NoError(t, err)

	return spool, applier
}

func TestStatsCountsWhatTheApplierCommitted(t *testing.T) {
	spool, applier := newTestSpool(t, Options{})

	spool.RecordBlock(100)
	require.NoError(t, spool.Insert("transfers", []any{1, 2, 3}))
	require.NoError(t, spool.Insert("transfers", []any{1, 2, 3}))
	require.NoError(t, spool.Insert("accounts", []any{1, 2}))
	spool.RecordBlock(104)
	spool.RecordCursor("some-cursor")

	require.NoError(t, spool.Close(context.Background()))
	require.Len(t, applier.applied, 1)

	stats := spool.Stats()
	require.Equal(t, int64(1), stats.Segments)
	require.Equal(t, int64(5), stats.Blocks, "100 through 104 inclusive")
	require.Equal(t, int64(3), stats.Rows)
	require.Equal(t, int64(3), stats.Bytes, "one byte per row, per countingWriter")
	require.Equal(t, uint64(104), stats.AppliedBlock)
	require.Positive(t, stats.ApplyDuration)
	require.Positive(t, stats.ApplierBusy)

	// Everything committed, so nothing is left waiting.
	require.Zero(t, stats.BytesOnDisk)
	require.Zero(t, stats.BlocksBuffered)
	require.Zero(t, stats.QueueDepth)
	require.Zero(t, stats.QuotaWait)
}

func TestStatsAttributesTheSealToItsReason(t *testing.T) {
	// A segment target of one byte seals on the first flush, so the size path is taken
	// without waiting on the sizer to grow.
	spool, _ := newTestSpool(t, Options{SegmentMaxBytes: 1})

	spool.RecordBlock(7)
	require.NoError(t, spool.Insert("transfers", []any{1}))
	spool.RecordCursor("cursor-a")
	require.NoError(t, spool.MaybeSeal(context.Background()))

	spool.RecordBlock(8)
	require.NoError(t, spool.Insert("transfers", []any{1}))
	spool.RecordCursor("cursor-b")
	require.NoError(t, spool.Drain(context.Background()))

	spool.RecordBlock(9)
	require.NoError(t, spool.Insert("transfers", []any{1}))
	spool.RecordCursor("cursor-c")
	require.NoError(t, spool.Close(context.Background()))

	stats := spool.Stats()
	require.Equal(t, int64(1), stats.Sealed[SealBySize])
	require.Equal(t, int64(1), stats.Sealed[SealByDrain])
	require.Equal(t, int64(1), stats.Sealed[SealByClose])
	require.Zero(t, stats.Sealed[SealByIdle])
	require.Equal(t, int64(3), stats.Segments)
}

func TestStatsReportsTheOpenSegmentAsStillWaiting(t *testing.T) {
	spool, _ := newTestSpool(t, Options{})
	t.Cleanup(func() { _ = spool.Close(context.Background()) })

	spool.RecordBlock(10)
	require.NoError(t, spool.Insert("transfers", []any{1, 2, 3, 4}))
	spool.RecordBlock(12)

	stats := spool.Stats()
	require.Zero(t, stats.Segments, "nothing sealed yet")
	require.Equal(t, int64(4), stats.OpenBytes)
	require.Equal(t, int64(3), stats.OpenBlocks)
	require.Positive(t, stats.OpenAge)

	// The open segment counts as waiting: it is what a kill right now would cost.
	require.Equal(t, int64(4), stats.BytesOnDisk)
	require.Equal(t, int64(3), stats.BlocksBuffered)
}

func TestSealReasonNamesEveryCountItIndexes(t *testing.T) {
	var stats Stats
	for reason := range stats.Sealed {
		require.NotEqual(t, "unknown", SealReason(reason).String())
	}
}

// Recovery runs inside Open, before the sinker starts and before anything else logs, and
// replaying the spool a killed backfill left behind is minutes of COPY. It has to say so
// on the way in: a summary at the end cannot be read until the wait is already over.
func TestRecoveryAnnouncesItselfBeforeReplaying(t *testing.T) {
	root := t.TempDir()
	segment := filepath.Join(root, "test", "seg-000000000001")
	require.NoError(t, os.MkdirAll(segment, 0o755))
	require.NoError(t, WriteManifest(segment, &Manifest{
		FirstBlock: 100, LastBlock: 199, Cursor: "a-cursor", Sealed: true, Format: FormatValues,
	}))

	core, logs := observer.New(zapcore.InfoLevel)
	applier := &recordingApplier{}

	spool, err := New(context.Background(), Options{Dir: root, MaxIdle: -1}, countingCodec{}, applier, "test", zap.New(core))
	require.NoError(t, err)
	t.Cleanup(func() { _ = spool.Close(context.Background()) })

	require.Len(t, applier.applied, 1)
	require.NoDirExists(t, segment, "a replayed segment is removed")

	// Order is the whole point: the announcement has to precede the COPY it is warning
	// about, not follow it. Asserting only presence would pass with it moved to the end.
	announced, perSegment, summary := -1, -1, -1
	for index, entry := range logs.All() {
		switch entry.Message {
		case "recovering the local spool, the stream does not start until this finishes":
			announced = index
			require.Equal(t, int64(1), entry.ContextMap()["segments_on_disk"])
		case "replaying a spool segment":
			perSegment = index
			require.Equal(t, "1/1", entry.ContextMap()["progress"])
		case "recovered the local spool":
			summary = index
			require.Equal(t, int64(1), entry.ContextMap()["segments_replayed"])
		}
	}
	require.NotEqual(t, -1, announced, "the wait has to be announced")
	require.Less(t, announced, perSegment, "announced before the first segment is replayed")
	require.Less(t, perSegment, summary, "each segment reports before the summary")
}

// A spool with nothing on disk is the common case and must stay silent.
func TestRecoveryStaysQuietWithAnEmptySpool(t *testing.T) {
	core, logs := observer.New(zapcore.InfoLevel)

	spool, err := New(context.Background(), Options{Dir: t.TempDir(), MaxIdle: -1}, countingCodec{}, &recordingApplier{}, "test", zap.New(core))
	require.NoError(t, err)
	t.Cleanup(func() { _ = spool.Close(context.Background()) })

	require.Zero(t, logs.Len())
}

// blockingApplier holds one Apply open so a snapshot can be taken mid-segment.
type blockingApplier struct {
	entered chan struct{}
	release chan struct{}
}

func (a *blockingApplier) EnsureSchema(context.Context) error { return nil }

func (a *blockingApplier) AlreadyApplied(context.Context, *Manifest) (bool, error) {
	return false, nil
}

func (a *blockingApplier) Apply(context.Context, string, *Manifest) error {
	close(a.entered)
	<-a.release

	return nil
}

// A segment that takes longer than the logging interval must still count towards the
// ratio while it runs. Counting only on completion leaves an applier pinned on a degraded
// database reporting itself as doing nothing — the exact inversion of the reading this
// number exists to give.
func TestApplierBusyCountsTheSegmentStillInFlight(t *testing.T) {
	applier := &blockingApplier{entered: make(chan struct{}), release: make(chan struct{})}
	options := Options{Dir: t.TempDir(), MaxIdle: -1}

	spool, err := New(context.Background(), options, countingCodec{}, applier, "test", zap.NewNop())
	require.NoError(t, err)

	spool.RecordBlock(1)
	require.NoError(t, spool.Insert("transfers", []any{1}))
	spool.RecordCursor("a-cursor")
	require.NoError(t, spool.Seal(context.Background(), SealBySize))

	<-applier.entered
	time.Sleep(5 * time.Millisecond)

	stats := spool.Stats()
	require.Positive(t, stats.ApplierBusy, "the segment in flight counts while it is in flight")
	require.Zero(t, stats.Segments, "and is not counted as applied until it is")
	require.Positive(t, stats.ApplierBusyRatioForTest(), "so the ratio is not zero at the worst moment")

	close(applier.release)
	require.NoError(t, spool.Close(context.Background()))
}

// Close is bounded by its context: sealed segments are durable and recovery replays them,
// so a database that has stopped responding must not hold the process open indefinitely.
func TestCloseGivesUpOnTheDrainWhenTheDeadlinePasses(t *testing.T) {
	applier := &blockingApplier{entered: make(chan struct{}), release: make(chan struct{})}
	t.Cleanup(func() { close(applier.release) })

	spool, err := New(context.Background(), Options{Dir: t.TempDir(), MaxIdle: -1}, countingCodec{}, applier, "test", zap.NewNop())
	require.NoError(t, err)

	spool.RecordBlock(1)
	require.NoError(t, spool.Insert("transfers", []any{1}))
	spool.RecordCursor("a-cursor")
	require.NoError(t, spool.Seal(context.Background(), SealBySize))
	<-applier.entered

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- spool.Close(ctx) }()

	select {
	case err := <-done:
		require.NoError(t, err, "an abandoned drain is not a failure, its segments are replayed")
	case <-time.After(5 * time.Second):
		t.Fatal("Close ignored its deadline and blocked on the applier")
	}
}

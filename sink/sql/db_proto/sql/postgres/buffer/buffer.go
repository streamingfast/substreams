package buffer

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/streamingfast/substreams/sink/sql/db_proto/sql/postgres/pgcopy"
	"go.uber.org/zap"
)

// Options configures the on-disk buffer. Everything not set here is derived at runtime.
type Options struct {
	// Dir is where segments live. Required.
	Dir string
	// MaxBytes is the disk quota. Writes block once the buffer holds this much waiting to
	// be applied, which is what turns a slow database into backpressure rather than a
	// full disk. Zero picks 16GiB.
	MaxBytes int64
	// TargetFlushDuration is what the segment sizer aims each COPY to take. Zero picks 3s.
	TargetFlushDuration time.Duration
	// SegmentMinBytes and SegmentMaxBytes bound the sizer. Zero picks 8MiB and 512MiB.
	SegmentMinBytes int64
	SegmentMaxBytes int64
	// QueueDepth is how many sealed segments may wait to be applied. Zero picks 2.
	QueueDepth int
}

func (o Options) withDefaults() Options {
	if o.MaxBytes <= 0 {
		o.MaxBytes = 8 << 30
	}
	if o.TargetFlushDuration <= 0 {
		o.TargetFlushDuration = 3 * time.Second
	}
	if o.SegmentMinBytes <= 0 {
		o.SegmentMinBytes = 8 << 20
	}
	if o.SegmentMaxBytes <= 0 {
		o.SegmentMaxBytes = 512 << 20
	}
	if o.QueueDepth <= 0 {
		o.QueueDepth = 2
	}

	return o
}

// Buffer holds rows on disk and applies whole segments to PostgreSQL in the background.
//
// Rows go in through Insert on the sinker's goroutine; sealed segments leave through a
// bounded queue that one applier goroutine drains. The queue and the disk quota are what
// bound memory and disk when the database cannot keep up.
type Buffer struct {
	options  Options
	applier  *Applier
	logger   *zap.Logger
	schema   string
	tables   map[string]*pgcopy.Table
	segments chan *sealedSegment

	current       *segment
	segmentTarget int64
	nextSequence  uint64

	// bytesOnDisk counts sealed segments waiting plus the one being written, so the
	// operator sees the buffer, not just the queue length.
	bytesOnDisk atomic.Int64
	blocksAhead atomic.Int64
	// appliedBlock is the last block the applier actually committed. It is the only
	// honest answer to "what is in the database": with a buffer, a block reaching the
	// sinker's flush means it was queued, not stored.
	appliedBlock atomic.Uint64

	applyErr  atomic.Pointer[error]
	waitGroup sync.WaitGroup
	closeOnce sync.Once
}

type sealedSegment struct {
	dir      string
	manifest *Manifest
	bytes    int64

	// barrier marks a segment that carries no data: the applier closes it and moves on,
	// which tells Drain that everything queued ahead of it has reached the database.
	barrier chan struct{}
}

// New prepares the buffer directory and starts the applier.
//
// Recovery runs first: anything already on disk is either replayed or discarded before a
// single new row is written, so the applied state and the resume cursor cannot disagree.
func New(ctx context.Context, options Options, applier *Applier, schema string, tables map[string]*pgcopy.Table, logger *zap.Logger) (*Buffer, error) {
	options = options.withDefaults()

	root := filepath.Join(options.Dir, schema)
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, fmt.Errorf("creating buffer directory %s: %w", root, err)
	}

	b := &Buffer{
		options:       options,
		applier:       applier,
		logger:        logger.Named("buffer"),
		schema:        schema,
		tables:        tables,
		segments:      make(chan *sealedSegment, options.QueueDepth),
		segmentTarget: options.SegmentMinBytes,
	}
	b.options.Dir = root

	if err := b.recover(ctx); err != nil {
		return nil, fmt.Errorf("recovering buffer at %s: %w", root, err)
	}

	b.waitGroup.Add(1)
	go b.applyLoop(ctx)

	return b, nil
}

// Insert buffers one row. It blocks when the buffer is full, which is the backpressure
// that keeps a slow database from filling the disk.
func (b *Buffer) Insert(table string, values []any) error {
	if err := b.pendingError(); err != nil {
		return err
	}

	target, ok := b.tables[table]
	if !ok {
		return fmt.Errorf("no column layout known for table %q", table)
	}

	if b.current == nil {
		if err := b.startSegment(); err != nil {
			return err
		}
	}

	return b.current.writeRow(table, target, values)
}

// RecordBlock notes which block the rows now being written belong to.
func (b *Buffer) RecordBlock(blockNum uint64) {
	if b.current == nil {
		if err := b.startSegment(); err != nil {
			b.setError(err)
			return
		}
	}
	if b.current.firstBlock == 0 {
		b.current.firstBlock = blockNum
	}
	b.current.lastBlock = blockNum
}

// RecordCursor notes the cursor covering everything written so far. The segment carries
// it so that applying the segment and advancing the cursor commit together.
func (b *Buffer) RecordCursor(cursor string) {
	if b.current != nil {
		b.current.cursor = cursor
	}
}

// MaybeSeal hands the current segment to the applier once it is big enough. It is called
// at every sink flush, so it is also where backpressure is applied.
func (b *Buffer) MaybeSeal(ctx context.Context) error {
	if err := b.pendingError(); err != nil {
		return err
	}
	if b.current == nil {
		return nil
	}

	if b.current.pendingBytes() < b.segmentTarget {
		return nil
	}

	return b.Seal(ctx)
}

// Seal closes the current segment and queues it, blocking if the queue is full or the
// disk quota is reached.
func (b *Buffer) Seal(ctx context.Context) error {
	if b.current == nil {
		return nil
	}
	if b.current.cursor == "" {
		// Without a cursor the segment could not be resumed from, so it must not be
		// applied on its own.
		return nil
	}

	pending := b.current
	b.current = nil

	manifest, err := pending.seal()
	if err != nil {
		pending.discard()
		return fmt.Errorf("sealing segment %s: %w", pending.dir, err)
	}

	var bytes int64
	for _, table := range manifest.Tables {
		bytes += table.Bytes
	}

	if err := b.awaitQuota(ctx, bytes); err != nil {
		return err
	}

	b.bytesOnDisk.Add(bytes)
	b.blocksAhead.Add(int64(manifest.LastBlock-manifest.FirstBlock) + 1)

	select {
	case b.segments <- &sealedSegment{dir: pending.dir, manifest: manifest, bytes: bytes}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// awaitQuota blocks until the buffer is under its disk quota again.
func (b *Buffer) awaitQuota(ctx context.Context, incoming int64) error {
	warned := false
	for b.bytesOnDisk.Load()+incoming > b.options.MaxBytes {
		if err := b.pendingError(); err != nil {
			return err
		}
		if !warned {
			b.logger.Warn("local buffer is full, holding the stream until the database catches up",
				zap.String("on_disk", humanBytes(b.bytesOnDisk.Load())),
				zap.String("quota", humanBytes(b.options.MaxBytes)))
			warned = true
		}

		select {
		case <-time.After(100 * time.Millisecond):
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	return nil
}

func (b *Buffer) startSegment() error {
	b.nextSequence++
	dir := filepath.Join(b.options.Dir, fmt.Sprintf("seg-%012d", b.nextSequence))

	// A directory left over from a previous run under the same name was already handled
	// by recovery; removing it keeps a restart from appending to a stale stream.
	os.RemoveAll(dir)

	created, err := newSegment(dir, 0)
	if err != nil {
		return err
	}
	b.current = created

	return nil
}

// BytesOnDisk is how much is buffered waiting for the database, including the segment
// still being written. Counting only sealed segments would badly under-report: a segment
// grows to hundreds of megabytes before it is handed over.
func (b *Buffer) BytesOnDisk() int64 {
	total := b.bytesOnDisk.Load()
	if b.current != nil {
		total += b.current.pendingBytes()
	}

	return total
}

// BlocksBuffered is how many blocks are waiting, sealed or still being written.
func (b *Buffer) BlocksBuffered() int64 {
	total := b.blocksAhead.Load()
	if b.current != nil {
		total += b.current.blockCount()
	}

	return total
}

// AppliedBlock is the last block committed to the database, zero before the first one.
func (b *Buffer) AppliedBlock() uint64 { return b.appliedBlock.Load() }

func (b *Buffer) pendingError() error {
	if err := b.applyErr.Load(); err != nil {
		return *err
	}

	return nil
}

func (b *Buffer) setError(err error) {
	b.applyErr.CompareAndSwap(nil, &err)
}

// Close seals whatever is left, drains the applier and reports the first failure.
// Drain seals what is being written and returns once every segment queued before it has
// been applied, so the caller can act on a database that holds everything the buffer has
// accepted so far. An undo needs that: rows still in flight would otherwise land after
// the delete that was supposed to remove them.
func (b *Buffer) Drain(ctx context.Context) error {
	if err := b.Seal(ctx); err != nil {
		return err
	}

	barrier := make(chan struct{})
	select {
	case b.segments <- &sealedSegment{barrier: barrier}:
	case <-ctx.Done():
		return ctx.Err()
	}

	select {
	case <-barrier:
		return b.pendingError()
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (b *Buffer) Close(ctx context.Context) error {
	var err error
	b.closeOnce.Do(func() {
		err = b.Seal(ctx)
		close(b.segments)
		b.waitGroup.Wait()
	})

	if err != nil {
		return err
	}

	return b.pendingError()
}

func (b *Buffer) applyLoop(ctx context.Context) {
	defer b.waitGroup.Done()

	for pending := range b.segments {
		if pending.barrier != nil {
			close(pending.barrier)
			continue
		}

		if b.pendingError() != nil {
			// Keep draining so Close does not deadlock, but do not touch the database
			// again after a failure.
			continue
		}

		startAt := time.Now()
		if err := b.applier.Apply(ctx, pending.dir, pending.manifest); err != nil {
			b.setError(fmt.Errorf("applying segment %s: %w", pending.dir, err))
			continue
		}
		elapsed := time.Since(startAt)

		b.appliedBlock.Store(pending.manifest.LastBlock)
		b.bytesOnDisk.Add(-pending.bytes)
		b.blocksAhead.Add(-(int64(pending.manifest.LastBlock-pending.manifest.FirstBlock) + 1))
		os.RemoveAll(pending.dir)

		b.resize(pending.bytes, elapsed)

		b.logger.Debug("applied segment",
			zap.Uint64("first_block", pending.manifest.FirstBlock),
			zap.Uint64("last_block", pending.manifest.LastBlock),
			zap.String("bytes", humanBytes(pending.bytes)),
			zap.Duration("elapsed", elapsed))
	}
}

// resize steers the segment size toward the target COPY duration. Sizing by bytes rather
// than by blocks matters because block payload size varies by orders of magnitude across
// chains and modules, so a block count gives wildly unstable durations.
func (b *Buffer) resize(bytes int64, elapsed time.Duration) {
	if elapsed <= 0 {
		return
	}

	ratio := b.options.TargetFlushDuration.Seconds() / elapsed.Seconds()
	// Clamp per step so the sizer converges instead of oscillating.
	ratio = min(max(ratio, 0.5), 2.0)

	next := int64(float64(b.segmentTarget) * ratio)
	b.segmentTarget = min(max(next, b.options.SegmentMinBytes), b.options.SegmentMaxBytes)
}

func humanBytes(n int64) string {
	value := float64(n)
	for _, unit := range []string{"B", "KiB", "MiB", "GiB"} {
		if value < 1024 {
			return fmt.Sprintf("%.1f%s", value, unit)
		}
		value /= 1024
	}

	return fmt.Sprintf("%.1fTiB", value)
}

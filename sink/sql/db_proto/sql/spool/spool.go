package spool

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dustin/go-humanize"
	"go.uber.org/zap"
)

// Options configures the on-disk spool. Everything not set here is derived at runtime.
type Options struct {
	// Dir is where segments live. Required.
	Dir string
	// MaxBytes is the disk quota, and the only bound on how far ahead of the database the
	// stream may run. Writes are held once the spool holds this much waiting to be
	// applied, which is what turns a slow database into backpressure rather than a full
	// disk. Zero picks 8GiB.
	MaxBytes int64
	// WriteTargetDuration is how long one commit to the database should take. The sizer
	// measures each commit and steers the next segment toward it. Zero picks 3s.
	WriteTargetDuration time.Duration
	// SegmentMaxBytes is the ceiling the sizer may choose, whatever the target duration
	// would allow. Zero picks 512MiB. The floor is segmentFloorBytes and not
	// configurable, see sizer.go.
	SegmentMaxBytes int64
	// MaxIdle commits the open segment once no new row has reached it for this long, short
	// of its size target. Without it a stalled stream sits on those rows indefinitely,
	// leaving the cursor where it was. Zero picks 10s; negative disables idle sealing.
	MaxIdle time.Duration
}

func (o Options) withDefaults() Options {
	if o.MaxBytes <= 0 {
		o.MaxBytes = 8 << 30
	}
	if o.WriteTargetDuration <= 0 {
		o.WriteTargetDuration = 3 * time.Second
	}
	if o.SegmentMaxBytes <= 0 {
		o.SegmentMaxBytes = 512 << 20
	}
	// A normal segment must fit within the total spool budget. A single row can still
	// exceed it; that one segment is allowed through once the spool is otherwise empty.
	o.SegmentMaxBytes = min(o.SegmentMaxBytes, o.MaxBytes)
	if o.MaxIdle == 0 {
		o.MaxIdle = 10 * time.Second
	}

	return o
}

// Spool holds rows on disk and applies whole segments in the background.
//
// Rows go in through Insert on the sinker's goroutine; sealed segments leave through a
// queue that one applier goroutine drains. The disk quota is what bounds how far ahead of
// the database the stream may run — there is deliberately no second ceiling on the number
// of queued segments, which would otherwise be the limit that binds while the operator
// watches the one they set.
type Spool struct {
	options Options
	codec   Codec
	applier Applier
	logger  *zap.Logger
	schema  string
	sizer   *sizer

	// mutex guards the open segment. It is not only the sinker's goroutine any more: the
	// idle timer seals from its own, and when the stream stalls the sinker is blocked in a
	// read with no next opportunity to check anything.
	mutex        sync.Mutex
	current      *openSegment
	lastWriteAt  time.Time
	nextSequence uint64

	// sealMutex serializes seal-and-enqueue so segments reach the applier in the order
	// they were written, whichever goroutine sealed them. It is deliberately not the same
	// lock as mutex: sealing waits on the disk quota, and holding the open segment's lock
	// across that wait would stop the sinker for the whole time.
	sealMutex sync.Mutex

	queueMutex sync.Mutex
	queueCond  *sync.Cond
	queue      []*sealedSegment
	queueDone  bool

	// bytesOnDisk counts sealed segments waiting plus the one being written, so the
	// operator sees the spool, not just the queue length.
	bytesOnDisk atomic.Int64
	blocksAhead atomic.Int64
	// appliedBlock is the last block the applier actually committed. It is the only
	// honest answer to "what is in the database": with a spool, a block reaching the
	// sinker's flush means it was queued, not stored.
	appliedBlock atomic.Uint64

	// What the applier has committed, cumulative. The gap the operator watches says how
	// far behind the database is; these say what it is actually doing to get there, which
	// is the difference between a database that is slow and one that is not being asked.
	appliedSegments atomic.Int64
	appliedBlocks   atomic.Int64
	appliedRows     atomic.Int64
	appliedBytes    atomic.Int64
	applyDuration   atomic.Int64

	// applierBusy and applierIdle split the applier goroutine's wall clock between
	// working and waiting for a segment. Nothing else here answers whether the database
	// or the stream is the limit: a gap can be large because the database is slow, or
	// merely because the stream burst, and only the ratio tells those apart.
	applierBusy atomic.Int64
	applierIdle atomic.Int64

	// quotaWait is how long the stream has been held because the spool was full. Any
	// non-zero value means the database is gating download rather than the other way
	// round, which is otherwise reported once as a warning and then never again.
	quotaWait atomic.Int64

	// sealed counts segments by what closed them. A run whose segments are mostly sealed
	// by the idle timer is committing short segments, which is what starves the sizer and
	// multiplies the per-segment overhead the floor exists to bound.
	sealed [sealReasonCount]atomic.Int64

	applyErr  atomic.Pointer[error]
	waitGroup sync.WaitGroup
	closeOnce sync.Once
	done      chan struct{}
}

type sealedSegment struct {
	dir      string
	manifest *Manifest
	bytes    int64

	// barrier marks a segment that carries no data: the applier closes it and moves on,
	// which tells Drain that everything queued ahead of it has reached the database.
	barrier chan struct{}
}

// New prepares the spool directory and starts the applier.
//
// Recovery runs first: anything already on disk is either replayed or discarded before a
// single new row is written, so the applied state and the resume cursor cannot disagree.
func New(ctx context.Context, options Options, codec Codec, applier Applier, schema string, logger *zap.Logger) (*Spool, error) {
	options = options.withDefaults()

	root := filepath.Join(options.Dir, schema)
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, fmt.Errorf("creating spool directory %s: %w", root, err)
	}

	b := &Spool{
		options: options,
		codec:   codec,
		applier: applier,
		logger:  logger.Named("spool"),
		schema:  schema,
		sizer:   newSizer(options.WriteTargetDuration, options.SegmentMaxBytes),
		done:    make(chan struct{}),
	}
	b.queueCond = sync.NewCond(&b.queueMutex)
	b.options.Dir = root

	if err := b.recover(ctx); err != nil {
		return nil, fmt.Errorf("recovering spool at %s: %w", root, err)
	}

	b.waitGroup.Add(1)
	go b.applyLoop(ctx)

	if options.MaxIdle > 0 {
		b.waitGroup.Add(1)
		go b.idleLoop(ctx)
	}

	return b, nil
}

// Insert buffers one row. It blocks when the spool is full, which is the backpressure
// that keeps a slow database from filling the disk.
func (b *Spool) Insert(table string, values []any) error {
	if err := b.pendingError(); err != nil {
		return err
	}

	b.mutex.Lock()
	defer b.mutex.Unlock()

	if b.current == nil {
		if err := b.startSegmentLocked(); err != nil {
			return err
		}
	}
	b.lastWriteAt = time.Now()

	return b.current.writer.WriteRow(table, values)
}

// RecordBlock notes which block the rows now being written belong to.
func (b *Spool) RecordBlock(blockNum uint64) {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	if b.current == nil {
		if err := b.startSegmentLocked(); err != nil {
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
//
// A flush that produced no rows still has to advance the cursor, or a long stretch of
// blocks whose module output is empty is streamed, and paid for, again on restart. Such a
// flush opens a segment carrying nothing but the cursor.
func (b *Spool) RecordCursor(cursor string) {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	if b.current == nil {
		if err := b.startSegmentLocked(); err != nil {
			b.setError(err)
			return
		}
	}
	b.current.cursor = cursor
}

// SealReason says what closed a segment. It is counted rather than logged because the
// mix is the diagnosis: segments sealed at their size target are the intended path, and a
// run dominated by any other reason is committing short of what the sizer chose.
type SealReason int

const (
	// SealBySize is the intended path: the segment reached the size the sizer chose.
	SealBySize SealReason = iota
	// SealByIdle is the idle timer, which bounds what a stalled stream leaves unsealed.
	SealByIdle
	// SealByDrain is an undo, which has to see everything queued reach the database.
	SealByDrain
	// SealByClose is shutdown.
	SealByClose

	sealReasonCount
)

// String is what the stats panel labels the count with.
func (r SealReason) String() string {
	switch r {
	case SealBySize:
		return "size"
	case SealByIdle:
		return "idle"
	case SealByDrain:
		return "drain"
	case SealByClose:
		return "close"
	}

	return "unknown"
}

// MaybeSeal hands the current segment to the applier once it is big enough. It is called
// at every sink flush, so it is also where backpressure is applied.
func (b *Spool) MaybeSeal(ctx context.Context) error {
	if err := b.pendingError(); err != nil {
		return err
	}

	b.mutex.Lock()
	big := b.current != nil && b.current.writer.PendingBytes() >= b.sizer.size()
	b.mutex.Unlock()

	if !big {
		return nil
	}

	return b.Seal(ctx, SealBySize)
}

// Seal closes the current segment and queues it, holding the stream if the disk quota is
// reached. The reason is only counted, so that the mix of size, idle, drain and shutdown
// seals is visible without a log line per segment.
func (b *Spool) Seal(ctx context.Context, reason SealReason) error {
	// Serialized so segments reach the applier in the order they were written, whichever
	// goroutine sealed them.
	b.sealMutex.Lock()
	defer b.sealMutex.Unlock()

	b.mutex.Lock()
	pending := b.current
	if pending == nil || pending.cursor == "" {
		// Without a cursor the segment could not be resumed from, so it must not be
		// applied on its own.
		b.mutex.Unlock()
		return nil
	}
	b.current = nil
	b.mutex.Unlock()

	// The quota is checked before the manifest is written rather than after, and against
	// this segment's own bytes. An individually oversized segment cannot be split without
	// breaking row atomicity, so it is allowed once the spool is otherwise empty.
	if err := b.awaitQuota(ctx, pending.writer.PendingBytes()); err != nil {
		pending.writer.Discard()
		return err
	}

	manifest, err := pending.seal(b.codec.Format())
	if err != nil {
		pending.writer.Discard()
		return fmt.Errorf("sealing segment %s: %w", pending.dir, err)
	}

	bytes := segmentBytes(manifest)

	b.bytesOnDisk.Add(bytes)
	b.blocksAhead.Add(manifest.BlockCount())

	b.enqueue(&sealedSegment{dir: pending.dir, manifest: manifest, bytes: bytes})
	b.sealed[reason].Add(1)

	return nil
}

// awaitQuota blocks until the spool has room for the incoming segment.
func (b *Spool) awaitQuota(ctx context.Context, incoming int64) error {
	warned := false
	// Held time is accumulated rather than only warned about once: the warning says the
	// spool filled, this says how much download the database has cost since.
	heldFrom := time.Time{}
	defer func() {
		if !heldFrom.IsZero() {
			b.quotaWait.Add(int64(time.Since(heldFrom)))
		}
	}()

	for {
		onDisk := b.bytesOnDisk.Load()
		if onDisk+incoming <= b.options.MaxBytes {
			return nil
		}

		// A single row or segment cannot be split without breaking row atomicity. Once
		// previously queued data has drained, allow that one oversized segment through
		// rather than waiting forever for room it can never fit into.
		if onDisk == 0 && incoming > b.options.MaxBytes {
			b.logger.Warn("local spool segment exceeds its disk budget, allowing it because the spool is empty",
				zap.String("segment", humanize.IBytes(uint64(incoming))),
				zap.String("quota", humanize.IBytes(uint64(b.options.MaxBytes))))
			return nil
		}

		if err := b.pendingError(); err != nil {
			return err
		}
		if !warned {
			heldFrom = time.Now()
			b.logger.Warn("local spool is full, holding the stream until the database catches up",
				zap.String("on_disk", humanize.IBytes(uint64(onDisk))),
				zap.String("quota", humanize.IBytes(uint64(b.options.MaxBytes))))
			warned = true
		}

		select {
		case <-time.After(100 * time.Millisecond):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// startSegmentLocked opens a new segment. The caller holds mutex.
func (b *Spool) startSegmentLocked() error {
	b.nextSequence++
	dir := filepath.Join(b.options.Dir, fmt.Sprintf("seg-%012d", b.nextSequence))

	// A directory left over from a previous run under the same name was already handled
	// by recovery; removing it keeps a restart from appending to a stale stream.
	os.RemoveAll(dir)

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating segment directory %s: %w", dir, err)
	}

	writer, err := b.codec.OpenSegment(dir)
	if err != nil {
		return err
	}
	b.current = &openSegment{dir: dir, writer: writer, startedAt: time.Now()}
	b.lastWriteAt = time.Now()

	return nil
}

// BytesOnDisk is how much is buffered waiting for the database, including the segment
// still being written. Counting only sealed segments would badly under-report: a segment
// grows to hundreds of megabytes before it is handed over.
func (b *Spool) BytesOnDisk() int64 {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	total := b.bytesOnDisk.Load()
	if b.current != nil {
		total += b.current.writer.PendingBytes()
	}

	return total
}

// BlocksBuffered is how many blocks are waiting, sealed or still being written.
func (b *Spool) BlocksBuffered() int64 {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	total := b.blocksAhead.Load()
	if b.current != nil {
		total += b.current.blockCount()
	}

	return total
}

// AppliedBlock is the last block committed to the database, zero before the first one.
func (b *Spool) AppliedBlock() uint64 { return b.appliedBlock.Load() }

func (b *Spool) pendingError() error {
	if err := b.applyErr.Load(); err != nil {
		return *err
	}

	return nil
}

func (b *Spool) setError(err error) {
	b.applyErr.CompareAndSwap(nil, &err)
}

// Drain seals what is being written and returns once every segment queued before it has
// been applied, so the caller can act on a database that holds everything the spool has
// accepted so far. An undo needs that: rows still in flight would otherwise land after
// the delete that was supposed to remove them.
func (b *Spool) Drain(ctx context.Context) error {
	if err := b.Seal(ctx, SealByDrain); err != nil {
		return err
	}

	barrier := make(chan struct{})
	b.enqueue(&sealedSegment{barrier: barrier})

	select {
	case <-barrier:
		return b.pendingError()
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Close seals whatever is left, drains the applier and reports the first failure.
func (b *Spool) Close(ctx context.Context) error {
	var err error
	b.closeOnce.Do(func() {
		close(b.done)
		err = b.Seal(ctx, SealByClose)

		b.queueMutex.Lock()
		b.queueDone = true
		b.queueMutex.Unlock()
		b.queueCond.Broadcast()

		b.waitGroup.Wait()
	})

	if err != nil {
		return err
	}

	return b.pendingError()
}

func (b *Spool) enqueue(pending *sealedSegment) {
	b.queueMutex.Lock()
	b.queue = append(b.queue, pending)
	b.queueMutex.Unlock()

	b.queueCond.Signal()
}

// dequeue waits for the next segment. It reports false once the queue is closed and
// empty, which is the applier's signal to stop.
func (b *Spool) dequeue() (*sealedSegment, bool) {
	b.queueMutex.Lock()
	defer b.queueMutex.Unlock()

	for len(b.queue) == 0 && !b.queueDone {
		b.queueCond.Wait()
	}

	if len(b.queue) == 0 {
		return nil, false
	}

	pending := b.queue[0]
	b.queue = b.queue[1:]

	return pending, true
}

// idleLoop commits the open segment when the stream goes quiet.
//
// The size trigger buys throughput; this one bounds what is lost when the producer stops.
// A stream that stalls mid-backfill would otherwise leave the open segment unsealed
// indefinitely, so the cursor never advances and those blocks are streamed, and paid for,
// a second time on restart.
func (b *Spool) idleLoop(ctx context.Context) {
	defer b.waitGroup.Done()

	// Checking more often than the window keeps the worst-case delay to a fraction of it
	// rather than to twice it.
	ticker := time.NewTicker(max(b.options.MaxIdle/4, 100*time.Millisecond))
	defer ticker.Stop()

	for {
		select {
		case <-b.done:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			b.sealIfIdle(ctx)
		}
	}
}

func (b *Spool) sealIfIdle(ctx context.Context) {
	if b.pendingError() != nil {
		return
	}

	b.mutex.Lock()
	idle := b.current != nil && b.current.cursor != "" && time.Since(b.lastWriteAt) >= b.options.MaxIdle
	b.mutex.Unlock()

	if !idle {
		return
	}

	if err := b.Seal(ctx, SealByIdle); err != nil {
		b.setError(fmt.Errorf("sealing an idle segment: %w", err))
	}
}

func (b *Spool) applyLoop(ctx context.Context) {
	defer b.waitGroup.Done()

	for {
		// The wait is measured, not just endured: an applier that spends its time here is
		// keeping up and the stream is the limit, one that never waits is the limit.
		waitFrom := time.Now()
		pending, ok := b.dequeue()
		b.applierIdle.Add(int64(time.Since(waitFrom)))
		if !ok {
			return
		}

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
		b.applierBusy.Add(int64(elapsed))
		b.appliedSegments.Add(1)
		b.appliedBlocks.Add(pending.manifest.BlockCount())
		b.appliedRows.Add(pending.manifest.RowCount())
		b.appliedBytes.Add(pending.bytes)
		b.applyDuration.Add(int64(elapsed))

		b.appliedBlock.Store(pending.manifest.LastBlock)
		b.bytesOnDisk.Add(-pending.bytes)
		b.blocksAhead.Add(-pending.manifest.BlockCount())
		os.RemoveAll(pending.dir)

		b.sizer.observe(pending.bytes, elapsed)

		b.logger.Debug("applied segment",
			zap.Uint64("first_block", pending.manifest.FirstBlock),
			zap.Uint64("last_block", pending.manifest.LastBlock),
			zap.String("bytes", humanize.IBytes(uint64(pending.bytes))),
			zap.Duration("elapsed", elapsed))
	}
}

func segmentBytes(manifest *Manifest) int64 {
	bytes := manifest.LogBytes
	for _, table := range manifest.Tables {
		bytes += table.Bytes
	}

	return bytes
}

// openSegment is the segment being written: the driver's writers plus the block range and
// cursor the spool tracks itself, since those are the same whatever the bytes look like.
type openSegment struct {
	dir        string
	writer     SegmentWriter
	firstBlock uint64
	lastBlock  uint64
	cursor     string
	// startedAt is when the segment was opened, which is how long its rows have been
	// held without reaching the database — and therefore what a kill right now costs.
	startedAt time.Time
}

// blockCount is how many blocks this segment covers so far.
func (s *openSegment) blockCount() int64 {
	if s.firstBlock == 0 || s.lastBlock < s.firstBlock {
		return 0
	}

	return int64(s.lastBlock-s.firstBlock) + 1
}

// seal closes the writers and writes the manifest last, so a segment is either fully
// described or visibly incomplete.
//
// Only the manifest is fsynced. A process crash is fully covered by that; a machine crash
// may leave a data file short, which recovery catches through the codec's own check.
func (s *openSegment) seal(format Format) (*Manifest, error) {
	manifest := &Manifest{
		FirstBlock: s.firstBlock,
		LastBlock:  s.lastBlock,
		Cursor:     s.cursor,
		Sealed:     true,
		Format:     format,
	}

	if err := s.writer.Seal(manifest); err != nil {
		return nil, err
	}

	if err := WriteManifest(s.dir, manifest); err != nil {
		return nil, err
	}

	return manifest, nil
}

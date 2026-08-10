package cache

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

// Options configures the on-disk cache. Everything not set here is derived at runtime.
type Options struct {
	// Dir is where segments live. Required.
	Dir string
	// MaxBytes is the disk quota. Writes block once the cache holds this much waiting to
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
		o.MaxBytes = 16 << 30
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

// Cache buffers rows on disk and applies whole segments to PostgreSQL in the background.
//
// Rows go in through Insert on the sinker's goroutine; sealed segments leave through a
// bounded queue that one applier goroutine drains. The queue and the disk quota are what
// bound memory and disk when the database cannot keep up.
type Cache struct {
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
	// honest answer to "what is in the database": with a cache, a block reaching the
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
}

// New prepares the cache directory and starts the applier.
//
// Recovery runs first: anything already on disk is either replayed or discarded before a
// single new row is written, so the applied state and the resume cursor cannot disagree.
func New(ctx context.Context, options Options, applier *Applier, schema string, tables map[string]*pgcopy.Table, logger *zap.Logger) (*Cache, error) {
	options = options.withDefaults()

	root := filepath.Join(options.Dir, schema)
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, fmt.Errorf("creating cache directory %s: %w", root, err)
	}

	c := &Cache{
		options:       options,
		applier:       applier,
		logger:        logger.Named("cache"),
		schema:        schema,
		tables:        tables,
		segments:      make(chan *sealedSegment, options.QueueDepth),
		segmentTarget: options.SegmentMinBytes,
	}
	c.options.Dir = root

	if err := c.recover(ctx); err != nil {
		return nil, fmt.Errorf("recovering cache at %s: %w", root, err)
	}

	c.waitGroup.Add(1)
	go c.applyLoop(ctx)

	return c, nil
}

// Insert buffers one row. It blocks when the cache is full, which is the backpressure
// that keeps a slow database from filling the disk.
func (c *Cache) Insert(table string, values []any) error {
	if err := c.pendingError(); err != nil {
		return err
	}

	target, ok := c.tables[table]
	if !ok {
		return fmt.Errorf("no column layout known for table %q", table)
	}

	if c.current == nil {
		if err := c.startSegment(); err != nil {
			return err
		}
	}

	return c.current.writeRow(table, target, values)
}

// RecordBlock notes which block the rows now being written belong to.
func (c *Cache) RecordBlock(blockNum uint64) {
	if c.current == nil {
		if err := c.startSegment(); err != nil {
			c.setError(err)
			return
		}
	}
	if c.current.firstBlock == 0 {
		c.current.firstBlock = blockNum
	}
	c.current.lastBlock = blockNum
}

// RecordCursor notes the cursor covering everything written so far. The segment carries
// it so that applying the segment and advancing the cursor commit together.
func (c *Cache) RecordCursor(cursor string) {
	if c.current != nil {
		c.current.cursor = cursor
	}
}

// MaybeSeal hands the current segment to the applier once it is big enough. It is called
// at every sink flush, so it is also where backpressure is applied.
func (c *Cache) MaybeSeal(ctx context.Context) error {
	if err := c.pendingError(); err != nil {
		return err
	}
	if c.current == nil {
		return nil
	}

	if c.current.pendingBytes() < c.segmentTarget {
		return nil
	}

	return c.Seal(ctx)
}

// Seal closes the current segment and queues it, blocking if the queue is full or the
// disk quota is reached.
func (c *Cache) Seal(ctx context.Context) error {
	if c.current == nil {
		return nil
	}
	if c.current.cursor == "" {
		// Without a cursor the segment could not be resumed from, so it must not be
		// applied on its own.
		return nil
	}

	pending := c.current
	c.current = nil

	manifest, err := pending.seal()
	if err != nil {
		pending.discard()
		return fmt.Errorf("sealing segment %s: %w", pending.dir, err)
	}

	var bytes int64
	for _, table := range manifest.Tables {
		bytes += table.Bytes
	}

	if err := c.awaitQuota(ctx, bytes); err != nil {
		return err
	}

	c.bytesOnDisk.Add(bytes)
	c.blocksAhead.Add(int64(manifest.LastBlock-manifest.FirstBlock) + 1)

	select {
	case c.segments <- &sealedSegment{dir: pending.dir, manifest: manifest, bytes: bytes}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// awaitQuota blocks until the cache is under its disk quota again.
func (c *Cache) awaitQuota(ctx context.Context, incoming int64) error {
	warned := false
	for c.bytesOnDisk.Load()+incoming > c.options.MaxBytes {
		if err := c.pendingError(); err != nil {
			return err
		}
		if !warned {
			c.logger.Warn("local cache is full, holding the stream until the database catches up",
				zap.String("on_disk", humanBytes(c.bytesOnDisk.Load())),
				zap.String("quota", humanBytes(c.options.MaxBytes)))
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

func (c *Cache) startSegment() error {
	c.nextSequence++
	dir := filepath.Join(c.options.Dir, fmt.Sprintf("seg-%012d", c.nextSequence))

	// A directory left over from a previous run under the same name was already handled
	// by recovery; removing it keeps a restart from appending to a stale stream.
	os.RemoveAll(dir)

	created, err := newSegment(dir, 0)
	if err != nil {
		return err
	}
	c.current = created

	return nil
}

// BytesOnDisk is how much is buffered waiting for the database, including the segment
// still being written. Counting only sealed segments would badly under-report: a segment
// grows to hundreds of megabytes before it is handed over.
func (c *Cache) BytesOnDisk() int64 {
	total := c.bytesOnDisk.Load()
	if c.current != nil {
		total += c.current.pendingBytes()
	}

	return total
}

// BlocksBuffered is how many blocks are waiting, sealed or still being written.
func (c *Cache) BlocksBuffered() int64 {
	total := c.blocksAhead.Load()
	if c.current != nil {
		total += c.current.blockCount()
	}

	return total
}

// AppliedBlock is the last block committed to the database, zero before the first one.
func (c *Cache) AppliedBlock() uint64 { return c.appliedBlock.Load() }

func (c *Cache) pendingError() error {
	if err := c.applyErr.Load(); err != nil {
		return *err
	}

	return nil
}

func (c *Cache) setError(err error) {
	c.applyErr.CompareAndSwap(nil, &err)
}

// Close seals whatever is left, drains the applier and reports the first failure.
func (c *Cache) Close(ctx context.Context) error {
	var err error
	c.closeOnce.Do(func() {
		err = c.Seal(ctx)
		close(c.segments)
		c.waitGroup.Wait()
	})

	if err != nil {
		return err
	}

	return c.pendingError()
}

func (c *Cache) applyLoop(ctx context.Context) {
	defer c.waitGroup.Done()

	for pending := range c.segments {
		if c.pendingError() != nil {
			// Keep draining so Close does not deadlock, but do not touch the database
			// again after a failure.
			continue
		}

		startAt := time.Now()
		if err := c.applier.Apply(ctx, pending.dir, pending.manifest); err != nil {
			c.setError(fmt.Errorf("applying segment %s: %w", pending.dir, err))
			continue
		}
		elapsed := time.Since(startAt)

		c.appliedBlock.Store(pending.manifest.LastBlock)
		c.bytesOnDisk.Add(-pending.bytes)
		c.blocksAhead.Add(-(int64(pending.manifest.LastBlock-pending.manifest.FirstBlock) + 1))
		os.RemoveAll(pending.dir)

		c.resize(pending.bytes, elapsed)

		c.logger.Debug("applied segment",
			zap.Uint64("first_block", pending.manifest.FirstBlock),
			zap.Uint64("last_block", pending.manifest.LastBlock),
			zap.String("bytes", humanBytes(pending.bytes)),
			zap.Duration("elapsed", elapsed))
	}
}

// resize steers the segment size toward the target COPY duration. Sizing by bytes rather
// than by blocks matters because block payload size varies by orders of magnitude across
// chains and modules, so a block count gives wildly unstable durations.
func (c *Cache) resize(bytes int64, elapsed time.Duration) {
	if elapsed <= 0 {
		return
	}

	ratio := c.options.TargetFlushDuration.Seconds() / elapsed.Seconds()
	// Clamp per step so the sizer converges instead of oscillating.
	ratio = min(max(ratio, 0.5), 2.0)

	next := int64(float64(c.segmentTarget) * ratio)
	c.segmentTarget = min(max(next, c.options.SegmentMinBytes), c.options.SegmentMaxBytes)
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

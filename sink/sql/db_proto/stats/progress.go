package stats

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/streamingfast/logging/zapx"
	protosql "github.com/streamingfast/substreams/sink/sql/db_proto/sql"
	"github.com/streamingfast/substreams/sink/sql/db_proto/sql/spool"
	"go.uber.org/zap"
)

// Progress tracks the two ends of the sink pipeline: what has been downloaded from
// Substreams, and what has actually been committed to the database — together with what
// the write path in between is doing about the distance.
//
// The distance is the number the operator needs. Substreams throughput is paid for, so a
// run should be limited by the stream, not by the database — and when it is not, the gap
// is where that shows. But a gap alone does not say whose fault it is: it grows both when
// the database is slow and when the stream merely bursts. That is what the spool's own
// account is here for, and why it lives on the same type rather than beside it — the two
// were being fed the same numbers from the same call and rendering them twice.
//
// The counters are written from the sinker's goroutine and read from the logging ticker,
// hence the atomics; the write-path snapshot is a struct, hence the mutex.
type Progress struct {
	downloadedBlock atomic.Uint64
	appliedBlock    atomic.Uint64
	heldBlocks      atomic.Int64
	bufferedBytes   atomic.Int64
	peakBlocksAhead atomic.Uint64

	// warnAboveBlocks is the gap past which the buffer is reported as a problem rather
	// than as the normal working set. It only decides anything while nothing spools: a
	// block count says how much memory is held when the blocks are held in memory.
	warnAboveBlocks uint64

	mutex sync.Mutex

	// spooling is whether rows are going to disk *now*, and everSpooled whether they ever
	// did. They differ for the whole second half of a normal run: reaching the chain head
	// closes the spool and switches to direct inserts, after which the rates, the pressure
	// and what is in flight all describe something that no longer exists, while the totals
	// committed during the backfill are still worth reporting.
	spooling    bool
	everSpooled bool
	current     protosql.WriteStats

	// previous is the snapshot rendered at the last tick. Rates are differenced against
	// it rather than divided by the run's total time, because a lifetime average says
	// nothing once anything has changed: a backfill that was fast for an hour and has been
	// stalled for ten minutes reports a healthy rate while nothing is moving.
	previous   protosql.WriteStats
	previousAt time.Time
}

// NewProgress returns a tracker whose "falling behind" threshold is derived from the
// batch size: a few batches in flight is normal, an order of magnitude more is not.
func NewProgress(blockBatchSize int) *Progress {
	warnAbove := uint64(blockBatchSize) * 4
	if warnAbove < 100 {
		warnAbove = 100
	}

	return &Progress{warnAboveBlocks: warnAbove, previousAt: time.Now()}
}

// fallingBehind reports a buffer that has stopped looking like a working set.
//
// With a spool that is a share of the disk budget it was given, not a number of blocks:
// the budget is the whole point of the spool, and the operator set it. Without one — or
// once the switch to direct inserts has closed it — the blocks are held in memory and
// their count is what matters.
func (p *Progress) fallingBehind(ahead uint64, quota int64) bool {
	if buffered := p.bufferedBytes.Load(); buffered > 0 && quota > 0 {
		return buffered*2 >= quota
	}

	return ahead > p.warnAboveBlocks
}

// RecordDownloaded notes a block received from the stream.
func (p *Progress) RecordDownloaded(blockNum uint64) {
	p.downloadedBlock.Store(blockNum)
	p.trackPeak()
}

// RecordApplied notes a block committed to the database.
func (p *Progress) RecordApplied(blockNum uint64) {
	p.appliedBlock.Store(blockNum)
	p.trackPeak()
}

// RecordBuffered notes everything sitting between the stream and the database: the blocks
// held in memory for the next flush, plus whatever the write path has queued on disk.
//
// It takes the whole snapshot rather than the two numbers the gap needs so that there is
// one source for both. spooling is reported on every call, including the negative one:
// the switch to direct inserts turns it off mid-run, and a panel that only ever hears
// "yes" goes on describing a spool that has been closed.
func (p *Progress) RecordBuffered(held int, buffered protosql.WriteStats, spooling bool) {
	p.heldBlocks.Store(int64(held) + buffered.BlocksBuffered)
	p.bufferedBytes.Store(buffered.BytesOnDisk)
	p.trackPeak()

	p.mutex.Lock()
	defer p.mutex.Unlock()

	p.spooling = spooling
	if spooling {
		p.everSpooled = true
		p.current = buffered
	}
	// When it is off, current is deliberately left where it was: it is the last true
	// reading, and it is what the closed-spool totals are printed from.
}

// Spooling reports whether rows are going to disk right now, which is what decides that a
// flush no longer means the rows are stored.
func (p *Progress) Spooling() bool {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	return p.spooling
}

// SetResumeBlock seeds the applied mark from the cursor the run resumes at, so the
// first gap is measured against real progress rather than against zero.
func (p *Progress) SetResumeBlock(blockNum uint64) {
	p.appliedBlock.Store(blockNum)
}

// BlocksAhead is how far the download is in front of the database.
func (p *Progress) BlocksAhead() uint64 {
	applied := p.appliedBlock.Load()
	if applied == 0 {
		// Nothing committed yet in this run and no cursor to resume from, so the
		// downloaded block number says nothing about a gap — subtracting zero from it
		// would report the whole chain height as backlog. What is buffered is the gap.
		if held := p.heldBlocks.Load(); held > 0 {
			return uint64(held)
		}
		return 0
	}

	downloaded := p.downloadedBlock.Load()
	if downloaded <= applied {
		return 0
	}

	return downloaded - applied
}

func (p *Progress) trackPeak() {
	ahead := p.BlocksAhead()
	for {
		peak := p.peakBlocksAhead.Load()
		if ahead <= peak || p.peakBlocksAhead.CompareAndSwap(peak, ahead) {
			return
		}
	}
}

// Log reports the gap, at warning level once the buffer stops looking like a working set,
// then what the write path has been doing about it since the previous tick.
func (p *Progress) Log(logger *zap.Logger) {
	downloaded := p.downloadedBlock.Load()
	if downloaded == 0 {
		return
	}

	p.mutex.Lock()
	current, previous, spooling, everSpooled := p.current, p.previous, p.spooling, p.everSpooled
	elapsed := time.Since(p.previousAt)
	if spooling {
		p.previous, p.previousAt = current, time.Now()
	}
	p.mutex.Unlock()

	ahead := p.BlocksAhead()
	fields := []zap.Field{
		zap.Uint64("downloaded_through", downloaded),
		zap.Uint64("applied_through", p.appliedBlock.Load()),
		zap.Uint64("blocks_ahead", ahead),
		zap.Int64("blocks_buffered", p.heldBlocks.Load()),
		zap.Uint64("peak_blocks_ahead", p.peakBlocksAhead.Load()),
	}
	if buffered := p.bufferedBytes.Load(); buffered > 0 {
		fields = append(fields, zap.String("buffered_on_disk", humanBytes(buffered)))
		if current.Quota > 0 {
			fields = append(fields, zap.String("quota_used", percent(float64(buffered)/float64(current.Quota))))
		}
	}

	if p.fallingBehind(ahead, current.Quota) {
		logger.Warn("database is falling behind the stream, the buffer is over half of what it was given", fields...)
	} else {
		logger.Info("              Pipeline progress", fields...)
	}

	switch {
	case spooling && elapsed > 0:
		p.logSpool(logger, current, previous, elapsed)
	case everSpooled:
		// The spool was closed at the chain head. Rates over an applier that has stopped
		// are zero and would read as one that has stalled, and what is "in flight" is
		// whatever was there when it closed, so only the totals survive the switch.
		closed := []zap.Field{
			zap.Int64("segments", current.Segments),
			zap.Int64("blocks", current.Blocks),
			zap.String("rows", humanize.Comma(current.Rows)),
			zap.String("spooled_bytes", humanBytes(current.Bytes)),
			zapx.HumanDuration("applying", current.ApplyDuration),
			zapx.HumanDuration("quota_wait", current.QuotaWait),
		}
		if current.DatabaseBytesWritten > 0 {
			closed = append(closed, zap.String("db_bytes", humanBytes(current.DatabaseBytesWritten)))
		}
		logger.Info("                 Spool (closed)", closed...)
	}
}

// logSpool renders what the applier did between the two snapshots.
func (p *Progress) logSpool(logger *zap.Logger, current, previous protosql.WriteStats, elapsed time.Duration) {
	var (
		segments = current.Segments - previous.Segments
		blocks   = current.Blocks - previous.Blocks
		rows     = current.Rows - previous.Rows
		bytes    = current.Bytes - previous.Bytes
		wire     = current.DatabaseBytesWritten - previous.DatabaseBytesWritten
		applying = current.ApplyDuration - previous.ApplyDuration
		busy     = current.ApplierBusy - previous.ApplierBusy
		idle     = current.ApplierIdle - previous.ApplierIdle
	)

	applier := []zap.Field{
		zap.Int64("segments", current.Segments),
		zap.String("rate", fmt.Sprintf("%.1f seg/min", float64(segments)/elapsed.Minutes())),
	}
	if segments > 0 {
		applier = append(applier,
			zap.Int64("blocks_per_seg", blocks/segments),
			zap.Int64("rows_per_seg", rows/segments),
			// The duration of one commit, which is what --db-write-target-duration steers
			// and the sizer converges on.
			zapx.HumanDuration("apply_duration", applying/time.Duration(segments)),
		)
	}
	logger.Info("                  Spool applier", append(applier, zap.Int("queue_depth", current.QueueDepth))...)

	throughput := []zap.Field{
		zap.String("rows", fmt.Sprintf("%s/s (%s total)", humanize.Comma(perSecond(rows, elapsed)), humanize.Comma(current.Rows))),
		// Named for what it is: the spooled payload the applier consumed, which is what
		// the sizer and the disk quota are denominated in. db_bytes is what reached the
		// server, after the driver's encoding and any compression.
		zap.String("spool_rate", humanBytes(perSecond(bytes, elapsed))+"/s"),
		zap.String("spooled_bytes", humanBytes(current.Bytes)),
	}
	// Only a driver that counts its own socket can answer this — pgx owns the connection,
	// so the PostgreSQL path leaves it zero. Printing that zero would read as a database
	// receiving nothing while a COPY is in flight.
	if current.DatabaseBytesWritten > 0 {
		throughput = append(throughput,
			zap.String("db_write_rate", humanBytes(perSecond(wire, elapsed))+"/s"),
			zap.String("db_bytes", humanBytes(current.DatabaseBytesWritten)))
	}
	logger.Info("               Spool throughput", throughput...)

	pressure := []zap.Field{
		zap.String("applier_busy", percent(float64(busy)/float64(max(busy+idle, 1)))),
		zapx.HumanDuration("quota_wait", current.QuotaWait),
		zap.String("segment_target", humanBytes(current.SegmentTarget)),
		zap.String("open", fmt.Sprintf("%s / %d blocks", humanBytes(current.OpenBytes), current.OpenBlocks)),
		zapx.HumanDuration("open_age", current.OpenAge),
	}
	for reason, count := range current.Sealed {
		pressure = append(pressure, zap.Int64("sealed_by_"+spool.SealReason(reason).String(), count))
	}
	logger.Info("                 Spool pressure", pressure...)
}

func perSecond(n int64, elapsed time.Duration) int64 {
	if elapsed <= 0 {
		return 0
	}

	return int64(float64(n) / elapsed.Seconds())
}

func percent(ratio float64) string { return fmt.Sprintf("%.1f%%", ratio*100) }

func humanBytes(n int64) string {
	if n < 0 {
		return "0 B"
	}

	return humanize.IBytes(uint64(n))
}

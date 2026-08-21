package spool

import (
	"sync/atomic"
	"time"
)

// Stats is one consistent read of what the spool has committed and what it is still
// holding.
//
// It is a snapshot rather than a set of accessors because the numbers only mean anything
// together: rows without the time they took is not a rate, and a gap without the
// applier's occupancy does not say whose fault it is. Everything cumulative counts from
// process start, so the caller derives intervals by differencing two snapshots.
type Stats struct {
	// Applied, cumulative. Bytes is the segment size on disk, in whatever format the
	// codec writes — it is what the sizer steers and what the disk quota bounds, not what
	// crossed the network.
	Segments      int64
	Blocks        int64
	Rows          int64
	Bytes         int64
	ApplyDuration time.Duration

	// ApplierBusy and ApplierIdle split the applier goroutine's wall clock. Their ratio
	// is the answer to "is the database or the stream the limit": an applier that is busy
	// essentially all the time is the ceiling, one that mostly waits is not.
	ApplierBusy time.Duration
	ApplierIdle time.Duration

	// QuotaWait is how long the stream was held because the spool was full. Any non-zero
	// value means the database is gating download.
	QuotaWait time.Duration

	// Sealed counts segments by what closed them, indexed by SealReason.
	Sealed [sealReasonCount]int64

	// In flight right now.
	QueueDepth    int
	SegmentTarget int64
	OpenBytes     int64
	OpenBlocks    int64
	OpenAge       time.Duration

	// Queued totals, sealed segments plus the one being written.
	BlocksBuffered int64
	BytesOnDisk    int64
	AppliedBlock   uint64
	Quota          int64
}

// since is how long an activity marked as in progress has been running, or zero when
// none is. The marker is a wall-clock stamp, so a clock stepped backwards is clamped to
// zero rather than reported as negative time.
func since(marker *atomic.Int64) time.Duration {
	started := marker.Load()
	if started == 0 {
		return 0
	}

	if elapsed := time.Now().UnixNano() - started; elapsed > 0 {
		return time.Duration(elapsed)
	}

	return 0
}

// Stats reads the counters and the open segment together.
//
// The open segment is included in the queued totals for the same reason BytesOnDisk
// includes it: a segment grows to hundreds of megabytes before it is handed over, so
// counting only what is sealed badly under-reports what is waiting.
func (b *Spool) Stats() Stats {
	b.mutex.Lock()
	var (
		openBytes  int64
		openBlocks int64
		openAge    time.Duration
	)
	if b.current != nil {
		openBytes = b.current.writer.PendingBytes()
		openBlocks = b.current.blockCount()
		openAge = time.Since(b.current.startedAt)
	}
	// Read under the same lock as the open segment they are added to, or a segment that
	// seals between the two reads is counted once as open and once as sealed.
	sealedBytes, sealedBlocks := b.bytesOnDisk.Load(), b.blocksAhead.Load()
	b.mutex.Unlock()

	depth := b.queueDepth()

	stats := Stats{
		Segments:      b.appliedSegments.Load(),
		Blocks:        b.appliedBlocks.Load(),
		Rows:          b.appliedRows.Load(),
		Bytes:         b.appliedBytes.Load(),
		ApplyDuration: time.Duration(b.applyDuration.Load()),

		// Each folds in the activity still in progress, so a segment or a hold that
		// outlasts the logging interval is reported while it is happening rather than
		// only once it ends.
		ApplierBusy: time.Duration(b.applierBusy.Load()) + since(&b.applyingSince),
		ApplierIdle: time.Duration(b.applierIdle.Load()) + since(&b.waitingSince),
		QuotaWait:   time.Duration(b.quotaWait.Load()) + since(&b.quotaHeldSince),

		QueueDepth:    depth,
		SegmentTarget: b.sizer.size(),
		OpenBytes:     openBytes,
		OpenBlocks:    openBlocks,
		OpenAge:       openAge,

		BlocksBuffered: sealedBlocks + openBlocks,
		BytesOnDisk:    sealedBytes + openBytes,
		AppliedBlock:   b.appliedBlock.Load(),
		Quota:          b.options.MaxBytes,
	}
	for reason := range stats.Sealed {
		stats.Sealed[reason] = b.sealed[reason].Load()
	}

	return stats
}

// ApplierBusyRatioForTest is the share of the applier's wall clock spent applying. It
// exists for the test that guards against the in-flight segment going uncounted; the
// panel differences two snapshots instead, which a single ratio cannot express.
func (s Stats) ApplierBusyRatioForTest() float64 {
	total := s.ApplierBusy + s.ApplierIdle
	if total <= 0 {
		return 0
	}

	return float64(s.ApplierBusy) / float64(total)
}

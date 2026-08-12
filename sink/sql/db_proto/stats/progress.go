package stats

import (
	"fmt"
	"sync/atomic"

	"go.uber.org/zap"
)

// Progress tracks the two ends of the sink pipeline: what has been downloaded from
// Substreams, and what has actually been committed to the database.
//
// The distance between them is the number the operator needs. Substreams throughput is
// paid for, so a run should be limited by the stream, not by the database — and when it
// is not, the gap is where that shows. A gap that sits around one batch is the normal
// working set; a gap that keeps growing means the database cannot keep up and blocks are
// piling up in the buffer waiting for it.
//
// Every field is written from the sinker's goroutine and read from the logging ticker,
// hence the atomics.
type Progress struct {
	downloadedBlock atomic.Uint64
	appliedBlock    atomic.Uint64
	heldBlocks      atomic.Int64
	bufferedBytes   atomic.Int64
	peakBlocksAhead atomic.Uint64

	// warnAboveBlocks is the gap past which the buffer is reported as a problem rather
	// than as the normal working set.
	warnAboveBlocks uint64
}

// NewProgress returns a tracker whose "falling behind" threshold is derived from the
// batch size: a few batches in flight is normal, an order of magnitude more is not.
func NewProgress(blockBatchSize int) *Progress {
	warnAbove := uint64(blockBatchSize) * 4
	if warnAbove < 100 {
		warnAbove = 100
	}

	return &Progress{warnAboveBlocks: warnAbove}
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

// RecordBuffered notes how much is waiting between the two, in blocks and in bytes on
// disk. Bytes stay zero until blocks are buffered somewhere other than memory.
func (p *Progress) RecordBuffered(blocks int, bytes int64) {
	p.heldBlocks.Store(int64(blocks))
	p.bufferedBytes.Store(bytes)
	p.trackPeak()
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

// Log reports the gap, at warning level once the buffer stops looking like a working set.
func (p *Progress) Log(logger *zap.Logger) {
	downloaded := p.downloadedBlock.Load()
	if downloaded == 0 {
		return
	}

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
	}

	if ahead > p.warnAboveBlocks {
		logger.Warn("database is falling behind the stream, blocks are piling up in the buffer", fields...)
		return
	}

	logger.Info("             Pipeline progress", fields...)
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

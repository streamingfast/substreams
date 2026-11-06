package service

import (
	"time"

	"connectrpc.com/connect"
)

// segmentWatchdog decides when a tier2 segment execution must be aborted. It enforces two
// independent deadlines:
//
//   - a stall deadline, which resets every time the segment reports block progress. This is the
//     one that normally fires, and it targets segments that are wedged rather than segments that
//     are merely expensive.
//   - an absolute deadline measured from the start of the segment, as a backstop for a segment
//     that keeps inching forward but will never realistically complete.
//
// The distinction matters because killing a slow-but-advancing segment is expensive: the segment
// is never cached, so the retry redoes all of it and races the same clock again. A workload that
// needs slightly longer than the budget therefore never completes, no matter how many times it is
// retried.
//
// A single block is already bounded by the block execution timeout, so a stall timeout kept well
// above it cannot be tripped by one legitimately slow block.
type segmentWatchdog struct {
	startedAt        time.Time
	stallTimeout     time.Duration
	executionTimeout time.Duration

	lastProgressAt  time.Time
	processedBlocks uint64
}

func newSegmentWatchdog(startedAt time.Time, processedBlocks uint64, stallTimeout, executionTimeout time.Duration) *segmentWatchdog {
	return &segmentWatchdog{
		startedAt:        startedAt,
		stallTimeout:     stallTimeout,
		executionTimeout: executionTimeout,
		lastProgressAt:   startedAt,
		processedBlocks:  processedBlocks,
	}
}

// check records the segment's current block count and returns the error the segment must be
// canceled with, or nil if it may keep running. A zero timeout disables that deadline.
func (w *segmentWatchdog) check(now time.Time, processedBlocks uint64) error {
	if processedBlocks != w.processedBlocks {
		w.processedBlocks = processedBlocks
		w.lastProgressAt = now
	}

	if w.stallTimeout != 0 && now.Sub(w.lastProgressAt) > w.stallTimeout {
		return connect.NewError(connect.CodeDeadlineExceeded, ErrRequestStalled)
	}

	if w.executionTimeout != 0 && now.Sub(w.startedAt) > w.executionTimeout {
		return connect.NewError(connect.CodeDeadlineExceeded, ErrRequestActiveForTooLong)
	}

	return nil
}

func (w *segmentWatchdog) sinceLastProgress(now time.Time) time.Duration {
	return now.Sub(w.lastProgressAt)
}

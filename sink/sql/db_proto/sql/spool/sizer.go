package spool

import (
	"sync"
	"time"
)

// segmentFloorBytes is the smallest segment the sizer may choose. It is a constant rather
// than a flag because it guards the controller, not a policy: a database stalled on lock
// contention returns a long elapsed time, the loop halves the segment each round, and
// without a floor it converges on segments whose cost is entirely per-segment overhead —
// manifest write, fsync, transaction, one statement or COPY setup per table.
//
// It is deliberately not exposed. In the slower write modes the floor can exceed what the
// mode pushes within the target duration, so a flag named for the sizer would sometimes
// silently override the sizer it appears to configure.
const segmentFloorBytes int64 = 8 << 20

// sizer steers the segment size toward a target commit duration.
//
// Sizing by measured duration rather than by a block count is what keeps this stable
// across chains, where block payloads differ by orders of magnitude. Sizing by bytes
// rather than by rows is what keeps one dial meaningful in every write mode: the mode
// changes the cost per byte, and the loop absorbs that by converging somewhere else.
//
// It is read on the sinker's goroutine and written on the applier's, so it locks.
type sizer struct {
	target   time.Duration
	maxBytes int64

	mutex   sync.Mutex
	current int64
}

func newSizer(target time.Duration, maxBytes int64) *sizer {
	return &sizer{target: target, maxBytes: maxBytes, current: min(segmentFloorBytes, maxBytes)}
}

// size reports how large a segment should grow before it is committed.
func (s *sizer) size() int64 {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	return s.current
}

// observe folds one measured commit back into the target.
func (s *sizer) observe(bytes int64, elapsed time.Duration) {
	if elapsed <= 0 {
		return
	}

	ratio := s.target.Seconds() / elapsed.Seconds()
	// Clamp per step so the sizer converges instead of oscillating.
	ratio = min(max(ratio, 0.5), 2.0)

	s.mutex.Lock()
	defer s.mutex.Unlock()

	next := int64(float64(s.current) * ratio)
	s.current = min(max(next, segmentFloorBytes), s.maxBytes)
}

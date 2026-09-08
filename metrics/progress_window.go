package metrics

import (
	"time"
)

// Every rate and delta in the progress log covers a fixed trailing period, independent of how
// often the log is actually emitted. Tying the two together made the numbers change meaning
// whenever the emission interval was tuned, and made two consecutive lines incomparable when
// the first one covered a minute and the second one five.
//
// The window is accumulated in fixed time buckets: each recorded event lands in the bucket
// covering its timestamp, and reading sums the buckets still inside the window. Buckets are
// addressed by absolute time, so one that fell out of the window is simply overwritten when
// its slot comes back around — there is no rolling to do and no cleanup goroutine.
const (
	// ProgressWindow is the period every `_5m`-suffixed value in the progress log covers.
	ProgressWindow = 5 * time.Minute

	windowBucketDuration = 30 * time.Second
	// One extra bucket so the covered span is always at least ProgressWindow, even though the
	// most recent bucket is only partially elapsed.
	windowBuckets = int(ProgressWindow/windowBucketDuration) + 1
)

// bucketFor returns the slot a timestamp belongs to, and that slot's start time. Slots are
// derived from absolute time rather than from a moving cursor, so concurrent writers and
// readers always agree on which slot holds which period.
func bucketFor(now time.Time) (slot int, start time.Time) {
	bucket := now.UnixNano() / int64(windowBucketDuration)
	return int(bucket % int64(windowBuckets)), time.Unix(0, bucket*int64(windowBucketDuration))
}

// inWindow reports whether a bucket stamped at `start` still counts at `now`.
func inWindow(start, now time.Time) bool {
	return !start.IsZero() && !start.Before(now.Add(-ProgressWindow))
}

// windowedCounter sums increments over the trailing window.
type windowedCounter struct {
	stamps [windowBuckets]time.Time
	values [windowBuckets]uint64
}

func (w *windowedCounter) add(now time.Time, n uint64) {
	slot, start := bucketFor(now)
	if !w.stamps[slot].Equal(start) {
		w.stamps[slot] = start
		w.values[slot] = 0
	}
	w.values[slot] += n
}

func (w *windowedCounter) sum(now time.Time) (out uint64) {
	for slot, start := range w.stamps {
		if inWindow(start, now) {
			out += w.values[slot]
		}
	}
	return out
}

// windowedDuration sums elapsed time over the trailing window.
type windowedDuration struct {
	stamps [windowBuckets]time.Time
	values [windowBuckets]time.Duration
}

func (w *windowedDuration) add(now time.Time, d time.Duration) {
	slot, start := bucketFor(now)
	if !w.stamps[slot].Equal(start) {
		w.stamps[slot] = start
		w.values[slot] = 0
	}
	w.values[slot] += d
}

func (w *windowedDuration) sum(now time.Time) (out time.Duration) {
	for slot, start := range w.stamps {
		if inWindow(start, now) {
			out += w.values[slot]
		}
	}
	return out
}

// windowedStage holds one stage's job accounting over the trailing window. The fields are
// grouped in a single bucket so a stage costs one small array instead of one per counter.
type windowedStage struct {
	stamps  [windowBuckets]time.Time
	buckets [windowBuckets]stageBucket
}

type stageBucket struct {
	completed   uint64
	failed      uint64
	cancelled   uint64
	retried     uint64
	delayed     uint64
	duration    time.Duration
	maxDuration time.Duration
}

func (w *windowedStage) bucket(now time.Time) *stageBucket {
	slot, start := bucketFor(now)
	if !w.stamps[slot].Equal(start) {
		w.stamps[slot] = start
		w.buckets[slot] = stageBucket{}
	}
	return &w.buckets[slot]
}

func (w *windowedStage) sum(now time.Time) (out stageBucket) {
	for slot, start := range w.stamps {
		if !inWindow(start, now) {
			continue
		}
		bucket := w.buckets[slot]
		out.completed += bucket.completed
		out.failed += bucket.failed
		out.cancelled += bucket.cancelled
		out.retried += bucket.retried
		out.delayed += bucket.delayed
		out.duration += bucket.duration
		if bucket.maxDuration > out.maxDuration {
			out.maxDuration = bucket.maxDuration
		}
	}
	return out
}

// windowedDurations accumulates timed events over the trailing window: count, total, min and
// max. No sample is retained: quantiles over SendMsg durations described the gRPC flow-control
// window as much as the consumer, and the total is what the reporting relies on.
type windowedDurations struct {
	stamps  [windowBuckets]time.Time
	buckets [windowBuckets]durationBucket
}

type durationBucket struct {
	count   uint64
	blocks  uint64
	total   time.Duration
	minimum time.Duration
	maximum time.Duration
}

func (w *windowedDurations) record(now time.Time, elapsed time.Duration, blocks uint64) {
	slot, start := bucketFor(now)
	if !w.stamps[slot].Equal(start) {
		w.stamps[slot] = start
		w.buckets[slot] = durationBucket{}
	}

	bucket := &w.buckets[slot]
	if bucket.count == 0 || elapsed < bucket.minimum {
		bucket.minimum = elapsed
	}
	if elapsed > bucket.maximum {
		bucket.maximum = elapsed
	}
	bucket.count++
	bucket.blocks += blocks
	bucket.total += elapsed
}

func (w *windowedDurations) snapshot(now time.Time) durationStats {
	var out durationStats

	for slot, start := range w.stamps {
		if !inWindow(start, now) {
			continue
		}
		bucket := w.buckets[slot]
		if bucket.count == 0 {
			continue
		}
		if out.count == 0 || bucket.minimum < out.minimum {
			out.minimum = bucket.minimum
		}
		if bucket.maximum > out.maximum {
			out.maximum = bucket.maximum
		}
		out.count += bucket.count
		out.blocks += bucket.blocks
		out.total += bucket.total
	}
	return out
}

// callCounters is the cumulative state of one (module, extension) pair.
type callCounters struct {
	count uint64
	time  time.Duration
}

// windowedCallCounters turns cumulative external-call totals into a windowed delta. Unlike
// everything else here it cannot accumulate increments: calls made inside tier2 jobs are
// reported to tier1 as running totals, never as events. So one snapshot of those totals is
// kept per bucket, and the delta is measured against the oldest one still in the window.
type windowedCallCounters struct {
	stamps [windowBuckets]time.Time
	values [windowBuckets]map[string]callCounters
}

// observe records the current cumulative totals, once per bucket: the first observation of a
// bucket is what later reads measure against.
func (w *windowedCallCounters) observe(now time.Time, current map[string]callCounters) {
	slot, start := bucketFor(now)
	if w.stamps[slot].Equal(start) {
		return
	}

	snapshot := make(map[string]callCounters, len(current))
	for key, counters := range current {
		snapshot[key] = counters
	}
	w.stamps[slot] = start
	w.values[slot] = snapshot
}

// baseline returns the oldest snapshot still inside the window, or nil when none was taken
// yet — in which case the caller reports the lifetime totals, which is correct for a request
// younger than the window.
func (w *windowedCallCounters) baseline(now time.Time) map[string]callCounters {
	var oldest time.Time
	var out map[string]callCounters

	for slot, start := range w.stamps {
		if !inWindow(start, now) {
			continue
		}
		if oldest.IsZero() || start.Before(oldest) {
			oldest = start
			out = w.values[slot]
		}
	}
	return out
}

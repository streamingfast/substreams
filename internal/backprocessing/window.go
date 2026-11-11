package backprocessing

import "time"

// The server throttles how often it emits progress messages over the life of a request
// (500ms, then 1s after a minute, 3s after three, 5s after six). Windows are therefore
// anchored on time rather than on a number of samples, so that a value always covers the
// same trailing duration no matter how old the request is.
const RateWindowDuration = 30 * time.Second

// MinWindowForRate is the minimum span a window must cover before its derivative is
// considered meaningful. Below it, a single lucky (or unlucky) sample pair dominates.
const MinWindowForRate = 10 * time.Second

type rateSample struct {
	at   time.Time
	done uint64
}

// RateWindow computes the derivative of a monotonically increasing counter over a trailing
// time window.
type RateWindow struct {
	window  time.Duration
	samples []rateSample
}

func NewRateWindow(window time.Duration) *RateWindow {
	return &RateWindow{window: window}
}

func (w *RateWindow) Reset() { w.samples = nil }

func (w *RateWindow) Add(at time.Time, done uint64) {
	w.samples = append(w.samples, rateSample{at: at, done: done})
	w.trim(at)
}

func (w *RateWindow) trim(now time.Time) {
	cutoff := now.Add(-w.window)
	// Keep one sample older than the cutoff: it is the left-hand side of the derivative for
	// the oldest in-window sample, dropping it would shorten the effective span.
	keepFrom := 0
	for i, s := range w.samples {
		if s.at.After(cutoff) {
			break
		}
		keepFrom = i
	}
	w.samples = w.samples[keepFrom:]
}

// PerSecond returns the rate of change per second over the window, and the span the samples
// actually cover. ok is false when there is not enough data to say anything.
func (w *RateWindow) PerSecond() (rate float64, span time.Duration, ok bool) {
	if len(w.samples) < 2 {
		return 0, 0, false
	}

	first, last := w.samples[0], w.samples[len(w.samples)-1]
	span = last.at.Sub(first.at)
	if span <= 0 || last.done < first.done {
		return 0, span, false
	}

	return float64(last.done-first.done) / span.Seconds(), span, true
}

package tui

import "time"

// The server throttles how often it emits progress messages over the life of a request
// (500ms, then 1s after a minute, 3s after three, 5s after six). Windows are therefore
// anchored on time rather than on a number of samples, so that a value always covers the
// same trailing duration no matter how old the request is.
const rateWindowDuration = 30 * time.Second

// minWindowForRate is the minimum span a window must cover before its derivative is
// considered meaningful. Below it, a single lucky (or unlucky) sample pair dominates.
const minWindowForRate = 10 * time.Second

type rateSample struct {
	at   time.Time
	done uint64
}

// rateWindow computes the derivative of a monotonically increasing counter over a trailing
// time window.
type rateWindow struct {
	window  time.Duration
	samples []rateSample
}

func newRateWindow(window time.Duration) *rateWindow {
	return &rateWindow{window: window}
}

func (w *rateWindow) reset() { w.samples = nil }

func (w *rateWindow) add(at time.Time, done uint64) {
	w.samples = append(w.samples, rateSample{at: at, done: done})
	w.trim(at)
}

func (w *rateWindow) trim(now time.Time) {
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

// perSecond returns the rate of change per second over the window, and the span the samples
// actually cover. ok is false when there is not enough data to say anything.
func (w *rateWindow) perSecond() (rate float64, span time.Duration, ok bool) {
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

type moduleSample struct {
	at     time.Time
	ms     uint64
	blocks uint64
}

// moduleWindow tracks, per module, the cumulative processing time and block count so that a
// recent cost per block can be derived from their deltas.
type moduleWindow struct {
	window  time.Duration
	samples map[string][]moduleSample
}

func newModuleWindow(window time.Duration) *moduleWindow {
	return &moduleWindow{window: window, samples: map[string][]moduleSample{}}
}

func (w *moduleWindow) reset() { w.samples = map[string][]moduleSample{} }

func (w *moduleWindow) add(at time.Time, name string, ms, blocks uint64) {
	samples := append(w.samples[name], moduleSample{at: at, ms: ms, blocks: blocks})

	cutoff := at.Add(-w.window)
	keepFrom := 0
	for i, s := range samples {
		if s.at.After(cutoff) {
			break
		}
		keepFrom = i
	}

	w.samples[name] = samples[keepFrom:]
}

// msPerBlock returns the module cost over the window. ok is false until the window holds two
// samples that actually differ in block count, which is also the case for a module that has
// not executed a single block recently.
func (w *moduleWindow) msPerBlock(name string) (cost float64, ok bool) {
	samples := w.samples[name]
	if len(samples) < 2 {
		return 0, false
	}

	first, last := samples[0], samples[len(samples)-1]
	if last.blocks <= first.blocks || last.ms < first.ms {
		return 0, false
	}

	return float64(last.ms-first.ms) / float64(last.blocks-first.blocks), true
}

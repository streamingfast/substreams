package tui

import "time"

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

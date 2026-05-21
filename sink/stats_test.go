package sink

import (
	"runtime"
	"testing"
	"time"
)

// TestStatsCloseStopsAllRateJanitors verifies that Stats.Close() stops all
// three dmetrics rate objects (dataMsgRate, undoMsgRate, progressBlockRate).
//
// Each AvgRate* object spawns an internal janitor goroutine on construction.
// Before this fix, progressBlockRate.Stop() was missing, leaking one goroutine
// per Stats lifecycle. In sink.Sinker's continuous-mode usage this produces a
// linear goroutine slope over time.
func TestStatsCloseStopsAllRateJanitors(t *testing.T) {
	runtime.GC()
	time.Sleep(50 * time.Millisecond)
	before := runtime.NumGoroutine()

	const iterations = 50
	for range iterations {
		s := newStats(zlog)
		s.Close()
	}

	runtime.GC()
	time.Sleep(100 * time.Millisecond)
	after := runtime.NumGoroutine()

	delta := after - before
	if delta > 2 {
		t.Errorf("goroutine leak: started=%d ended=%d delta=%d (want ≤2); "+
			"likely progressBlockRate.Stop() is not called in Stats.Close()",
			before, after, delta)
	}
}

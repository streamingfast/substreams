package pipeline

// TEMPORARY local-testing helper: force a GC every 2s so heap/RSS drops show up
// promptly in pprof while chasing memory retention. DELETE before merging.

import (
	"runtime"
	"time"
)

func init() {
	go func() {
		for range time.Tick(2 * time.Second) {
			runtime.GC()
		}
	}()
}

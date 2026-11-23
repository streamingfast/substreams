package service

import (
	"context"
	"sync/atomic"
	"time"
)

// idleHeartbeatInterval is how long a linear stream may stay silent before a progress message
// is sent. It starts slower than the progress cadence used while back-processing, since a live
// stream is not waiting on its first data, and widens as the stream ages to limit egress. Every
// value is under the 300-350s idle timeouts of common NATs/firewalls. A non-zero requested
// interval, from Request.progress_messages_interval_ms, pins it.
func idleHeartbeatInterval(sinceLinearStart, requested time.Duration) time.Duration {
	if requested > 0 {
		return requested
	}

	switch {
	case sinceLinearStart < time.Minute:
		return 20 * time.Second
	case sinceLinearStart < 5*time.Minute:
		return 30 * time.Second
	default:
		return time.Minute
	}
}

// lastSentTracker records when the last message was written on a client stream. It is
// written by the response func wrapper and read by the heartbeat goroutine.
type lastSentTracker struct {
	unixNano atomic.Int64
}

func (t *lastSentTracker) touch() {
	t.unixNano.Store(time.Now().UnixNano())
}

// idle returns how long the stream has been silent, and false if nothing was ever sent.
func (t *lastSentTracker) idle() (time.Duration, bool) {
	last := t.unixNano.Load()
	if last == 0 {
		return 0, false
	}
	return time.Since(time.Unix(0, last)), true
}

// runIdleHeartbeat calls send whenever nothing was written on the stream for the duration
// returned by intervalFunc (re-evaluated at every check, so it can widen over time), until ctx
// is done or send fails.
// Any write, not only the heartbeat, resets the idle time. Nothing is sent before the first
// message (the session init) went out, so the heartbeat never precedes it.
func runIdleHeartbeat(ctx context.Context, intervalFunc func() time.Duration, last *lastSentTracker, send func() error) {
	timer := time.NewTimer(intervalFunc())
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}

		// Nothing sent yet: check again in a full interval.
		interval := intervalFunc()
		wait := interval
		if idle, sent := last.idle(); sent {
			if idle >= interval {
				if ctx.Err() != nil {
					return
				}
				if err := send(); err != nil {
					return
				}
			} else {
				wait = interval - idle
			}
		}
		timer.Reset(wait)
	}
}

package service

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/streamingfast/substreams"
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

// withIdleTracking wraps respFunc so that every response successfully sent on the stream, not
// only the heartbeat, resets the idle time reported by the returned tracker.
func withIdleTracking(respFunc substreams.ResponseFunc) (substreams.ResponseFunc, *lastSentTracker) {
	last := &lastSentTracker{}
	return func(resp substreams.ResponseFromAnyTier) error {
		err := respFunc(resp)
		if err == nil {
			last.touch()
		}
		return err
	}, last
}

// startIdleHeartbeat runs the heartbeat from now, the start of linear streaming, using the
// idleHeartbeatInterval ramp. The returned stop func ends it and waits for it to return.
func startIdleHeartbeat(ctx context.Context, requested time.Duration, last *lastSentTracker, send func() error) (stop func()) {
	ctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	startedAt := time.Now()
	go func() {
		defer close(done)
		runIdleHeartbeat(ctx, func() time.Duration {
			return idleHeartbeatInterval(time.Since(startedAt), requested)
		}, last, send)
	}()
	return func() {
		cancel()
		<-done
	}
}

// runIdleHeartbeat calls send whenever nothing was written on the stream for the duration
// returned by intervalFunc (re-evaluated at every check, so it can widen over time), until ctx
// is done. A failed send is retried at the next check: the client is either gone, in which case the
// request is ending anyway, or the failure was transient.
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
				// An error means the client is gone or ctx is done, both end the request
				// elsewhere, and the response handler already logs it.
				_ = send()
			} else {
				wait = interval - idle
			}
		}
		timer.Reset(wait)
	}
}

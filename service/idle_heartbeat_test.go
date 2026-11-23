package service

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testHeartbeatInterval = 20 * time.Second

// startHeartbeat runs the heartbeat in the current synctest bubble, stop cancels it and
// waits for the goroutine to return.
func startHeartbeat(t *testing.T, last *lastSentTracker, send func() error) (stop func(), done <-chan struct{}) {
	ctx, cancel := context.WithCancel(t.Context())
	doneCh := make(chan struct{})
	go func() {
		defer close(doneCh)
		runIdleHeartbeat(ctx, func() time.Duration { return testHeartbeatInterval }, last, send)
	}()
	return func() {
		cancel()
		<-doneCh
	}, doneCh
}

func TestIdleHeartbeat_EmittedAfterIdleInterval(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var last lastSentTracker
		last.touch() // the session init went out

		var calls atomic.Int32
		stop, _ := startHeartbeat(t, &last, func() error {
			calls.Add(1)
			last.touch() // the real respFunc touches on every write
			return nil
		})
		defer stop()

		time.Sleep(testHeartbeatInterval - time.Second)
		synctest.Wait()
		assert.Zero(t, calls.Load(), "heartbeat sent before the idle interval")

		time.Sleep(time.Second)
		synctest.Wait()
		assert.Equal(t, int32(1), calls.Load())

		time.Sleep(testHeartbeatInterval)
		synctest.Wait()
		assert.Equal(t, int32(2), calls.Load())
	})
}

func TestIdleHeartbeat_NothingBeforeFirstMessage(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var last lastSentTracker

		var calls atomic.Int32
		stop, _ := startHeartbeat(t, &last, func() error {
			calls.Add(1)
			return nil
		})
		defer stop()

		time.Sleep(10 * testHeartbeatInterval)
		synctest.Wait()
		assert.Zero(t, calls.Load())
	})
}

func TestIdleHeartbeat_SendsResetTimer(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var last lastSentTracker
		last.touch()

		var calls atomic.Int32
		stop, _ := startHeartbeat(t, &last, func() error {
			calls.Add(1)
			last.touch()
			return nil
		})
		defer stop()

		// A block every half interval for 10 intervals: the stream is never idle long enough.
		for range 20 {
			time.Sleep(testHeartbeatInterval / 2)
			last.touch()
		}
		synctest.Wait()
		assert.Zero(t, calls.Load(), "heartbeat sent although the stream was active")

		// Stream goes quiet, heartbeat must kick in one interval after the last send.
		time.Sleep(testHeartbeatInterval)
		synctest.Wait()
		assert.Equal(t, int32(1), calls.Load())
	})
}

func TestIdleHeartbeat_StopsAtStreamEnd(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var last lastSentTracker
		last.touch()

		var calls atomic.Int32
		stop, done := startHeartbeat(t, &last, func() error {
			calls.Add(1)
			last.touch()
			return nil
		})

		time.Sleep(testHeartbeatInterval)
		synctest.Wait()
		require.Equal(t, int32(1), calls.Load())

		stop()
		select {
		case <-done:
		default:
			require.Fail(t, "heartbeat goroutine did not stop on context cancel")
		}

		time.Sleep(10 * testHeartbeatInterval)
		synctest.Wait()
		assert.Equal(t, int32(1), calls.Load(), "heartbeat sent after stream end")
	})
}

func TestIdleHeartbeat_StopsOnSendError(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var last lastSentTracker
		last.touch()

		stop, done := startHeartbeat(t, &last, func() error {
			return errors.New("client gone")
		})
		defer stop()

		time.Sleep(testHeartbeatInterval)
		synctest.Wait()
		select {
		case <-done:
		default:
			require.Fail(t, "heartbeat goroutine did not stop after send error")
		}
	})
}

func TestIdleHeartbeatInterval(t *testing.T) {
	tests := []struct {
		name      string
		elapsed   time.Duration
		requested time.Duration
		expected  time.Duration
	}{
		{"live start is slower than progress cadence", 0, 0, 20 * time.Second},
		{"first minute", 59 * time.Second, 0, 20 * time.Second},
		{"under 5min", time.Minute, 0, 30 * time.Second},
		{"just under 5min", 4*time.Minute + 59*time.Second, 0, 30 * time.Second},
		{"from 5min", 5 * time.Minute, 0, time.Minute},
		{"requested pins it", time.Hour, 2 * time.Second, 2 * time.Second},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, idleHeartbeatInterval(tt.elapsed, tt.requested))
		})
	}
}

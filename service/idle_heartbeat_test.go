package service

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/streamingfast/substreams"
	pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
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

func TestIdleHeartbeat_RetriesAfterSendError(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var last lastSentTracker
		last.touch()

		var calls atomic.Int32
		stop, _ := startHeartbeat(t, &last, func() error {
			calls.Add(1)
			return errors.New("transient")
		})
		defer stop()

		time.Sleep(3 * testHeartbeatInterval)
		synctest.Wait()
		assert.Equal(t, int32(3), calls.Load())
	})
}

// The tests below go through the same helpers blocks() uses, with the real ramp.

func TestWithIdleTracking_OnlySuccessfulSendsTouch(t *testing.T) {
	var fail bool
	wrapped, last := withIdleTracking(func(substreams.ResponseFromAnyTier) error {
		if fail {
			return errors.New("client gone")
		}
		return nil
	})

	_, sent := last.idle()
	require.False(t, sent)

	fail = true
	require.Error(t, wrapped(&pbsubstreamsrpc.Response{}))
	_, sent = last.idle()
	assert.False(t, sent, "a failed send must not reset the idle time")

	fail = false
	require.NoError(t, wrapped(&pbsubstreamsrpc.Response{}))
	_, sent = last.idle()
	assert.True(t, sent)
}

func TestStartIdleHeartbeat_BlockSendsResetTimerAndStopWaits(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var heartbeats, blocks atomic.Int32
		var wrapped substreams.ResponseFunc
		wrapped, last := withIdleTracking(func(substreams.ResponseFromAnyTier) error { return nil })

		// Session init went out before linear streaming started.
		require.NoError(t, wrapped(&pbsubstreamsrpc.Response{}))

		stop := startIdleHeartbeat(t.Context(), 0, last, func() error {
			heartbeats.Add(1)
			return wrapped(&pbsubstreamsrpc.Response{})
		})

		// A block 15s in pushes the first heartbeat from 20s to 35s.
		time.Sleep(15 * time.Second)
		require.NoError(t, wrapped(&pbsubstreamsrpc.Response{}))
		blocks.Add(1)

		time.Sleep(19 * time.Second) // t=34s
		synctest.Wait()
		assert.Zero(t, heartbeats.Load())

		time.Sleep(time.Second) // t=35s
		synctest.Wait()
		assert.Equal(t, int32(1), heartbeats.Load())

		// Still in the first minute: next one 20s later.
		time.Sleep(20 * time.Second)
		synctest.Wait()
		assert.Equal(t, int32(2), heartbeats.Load())

		stop()
		time.Sleep(10 * time.Minute)
		synctest.Wait()
		assert.Equal(t, int32(2), heartbeats.Load(), "heartbeat sent after stop")
		assert.Equal(t, int32(1), blocks.Load())
	})
}

func TestStartIdleHeartbeat_RequestedIntervalPinsIt(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		wrapped, last := withIdleTracking(func(substreams.ResponseFromAnyTier) error { return nil })
		require.NoError(t, wrapped(&pbsubstreamsrpc.Response{}))

		var heartbeats atomic.Int32
		stop := startIdleHeartbeat(t.Context(), 2*time.Second, last, func() error {
			heartbeats.Add(1)
			return wrapped(&pbsubstreamsrpc.Response{})
		})
		defer stop()

		time.Sleep(10 * time.Second)
		synctest.Wait()
		assert.Equal(t, int32(5), heartbeats.Load())
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

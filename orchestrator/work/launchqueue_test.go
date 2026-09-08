package work

import (
	"context"
	"testing"
	"time"

	"github.com/streamingfast/substreams/orchestrator/stage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testLaunchDelays(t *testing.T, delay, jitter time.Duration) {
	t.Helper()
	prevDelay, prevJitter := workerOverloadedRetryDelay, workerOverloadedRetryJitter
	workerOverloadedRetryDelay, workerOverloadedRetryJitter = delay, jitter
	t.Cleanup(func() { workerOverloadedRetryDelay, workerOverloadedRetryJitter = prevDelay, prevJitter })
}

func TestLaunchQueue_WindowSize(t *testing.T) {
	assert.Equal(t, 2, NewLaunchQueue(10).windowSize)
	assert.Equal(t, 10, NewLaunchQueue(50).windowSize)
	assert.Equal(t, 15, NewLaunchQueue(200).windowSize, "the window is capped, a wider one only spreads the fleet thinner")
	assert.Equal(t, 2, NewLaunchQueue(1).windowSize, "a small request never serializes onto one job")
	assert.Equal(t, 2, NewLaunchQueue(0).windowSize)
}

func TestLaunchesBefore(t *testing.T) {
	assert.True(t, launchesBefore(stage.Unit{Segment: 1, Stage: 0}, stage.Unit{Segment: 2, Stage: 3}))
	assert.True(t, launchesBefore(stage.Unit{Segment: 1, Stage: 2}, stage.Unit{Segment: 1, Stage: 1}), "the last stage carries the ones under it")
	assert.False(t, launchesBefore(stage.Unit{Segment: 2, Stage: 2}, stage.Unit{Segment: 1, Stage: 0}))
}

// TestLaunchQueue_NoQueueNoWait validates that jobs dial without delay while no tier2 is
// turning them away.
func TestLaunchQueue_NoQueueNoWait(t *testing.T) {
	testLaunchDelays(t, time.Second, 0)

	q := NewLaunchQueue(10)
	start := time.Now()
	for segment := 0; segment < 10; segment++ {
		require.NoError(t, q.WaitTurn(context.Background(), stage.Unit{Segment: segment}))
	}
	assert.Less(t, time.Since(start), 100*time.Millisecond)
}

// TestLaunchQueue_HeadRedialsOnTheRetryDelay validates that a refused job at the head of
// the queue keeps dialing on the short retry delay.
func TestLaunchQueue_HeadRedialsOnTheRetryDelay(t *testing.T) {
	testLaunchDelays(t, 50*time.Millisecond, 0)

	q := NewLaunchQueue(10)
	head := stage.Unit{Segment: 1}
	q.Retry(head, workerOverloadedRetryDelay)

	start := time.Now()
	require.NoError(t, q.WaitTurn(context.Background(), head))
	assert.GreaterOrEqual(t, time.Since(start), 40*time.Millisecond)
	assert.Less(t, time.Since(start), 200*time.Millisecond)
}

// TestLaunchQueue_OutsideWindowDoesNotDial validates the point of the window: with the
// first two jobs of a 10-worker request refused, the third sends nothing at all until one
// of them is through.
func TestLaunchQueue_OutsideWindowDoesNotDial(t *testing.T) {
	testLaunchDelays(t, 20*time.Millisecond, 0)

	q := NewLaunchQueue(10) // window of 2
	first, second := stage.Unit{Segment: 1}, stage.Unit{Segment: 2}
	q.Retry(first, workerOverloadedRetryDelay)
	q.Retry(second, workerOverloadedRetryDelay)

	third := stage.Unit{Segment: 3}
	done := make(chan struct{})
	go func() {
		require.NoError(t, q.WaitTurn(context.Background(), third))
		close(done)
	}()

	select {
	case <-done:
		t.Fatal("job outside the launch window dialed a tier2")
	case <-time.After(200 * time.Millisecond):
	}

	q.Leave(first) // it got in: the window moves up to the second and third jobs

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("job never dialed after the window moved up to it")
	}
}

// TestLaunchQueue_LowSegmentGoesFirst validates that a job the scheduler creates late for
// a low segment dials right away, ahead of the higher segments already queued.
func TestLaunchQueue_LowSegmentGoesFirst(t *testing.T) {
	testLaunchDelays(t, time.Second, 0)

	q := NewLaunchQueue(10)
	for segment := 10; segment < 20; segment++ {
		q.Retry(stage.Unit{Segment: segment}, workerOverloadedRetryDelay)
	}

	start := time.Now()
	require.NoError(t, q.WaitTurn(context.Background(), stage.Unit{Segment: 1}))
	assert.Less(t, time.Since(start), 100*time.Millisecond)
}

func TestLaunchQueue_ContextCanceled(t *testing.T) {
	testLaunchDelays(t, 10*time.Second, 0)

	q := NewLaunchQueue(10)
	q.Retry(stage.Unit{Segment: 1}, workerOverloadedRetryDelay)
	q.Retry(stage.Unit{Segment: 2}, workerOverloadedRetryDelay)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	assert.ErrorIs(t, q.WaitTurn(ctx, stage.Unit{Segment: 3}), context.Canceled)
}

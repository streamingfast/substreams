package service

import (
	"errors"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var watchdogStart = time.Date(2026, 7, 31, 19, 46, 57, 0, time.UTC)

// TestSegmentWatchdog_SlowButProgressingSurvives is the case this watchdog exists for: a segment
// making thousands of `eth_call` per block advances slowly but steadily, and used to be killed by
// the fixed 60 minute budget. Since a killed segment is never cached, the retry redid the work and
// hit the same wall, so the request could never complete.
func TestSegmentWatchdog_SlowButProgressingSurvives(t *testing.T) {
	w := newSegmentWatchdog(watchdogStart, 0, 10*time.Minute, 4*time.Hour)

	// One block every 5 minutes for 3 hours: far past the old 60 minute budget, but never stalled.
	var processed uint64
	for elapsed := 5 * time.Minute; elapsed <= 3*time.Hour; elapsed += 5 * time.Minute {
		processed++
		require.NoError(t, w.check(watchdogStart.Add(elapsed), processed),
			"a segment still making progress must not be killed (elapsed: %s)", elapsed)
	}
}

func TestSegmentWatchdog_StalledIsKilled(t *testing.T) {
	w := newSegmentWatchdog(watchdogStart, 0, 10*time.Minute, 4*time.Hour)

	require.NoError(t, w.check(watchdogStart.Add(5*time.Minute), 12))
	// No progress since, but still within the stall budget.
	require.NoError(t, w.check(watchdogStart.Add(14*time.Minute), 12))

	err := w.check(watchdogStart.Add(15*time.Minute+1*time.Second), 12)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrRequestStalled)
	assert.Equal(t, connect.CodeDeadlineExceeded, connect.CodeOf(err))
}

// TestSegmentWatchdog_ProgressResetsStallDeadline verifies the deadline is measured from the last
// progress and not from the segment start.
func TestSegmentWatchdog_ProgressResetsStallDeadline(t *testing.T) {
	w := newSegmentWatchdog(watchdogStart, 0, 10*time.Minute, 4*time.Hour)

	// Nine minutes of silence, then a block: the stall clock restarts.
	require.NoError(t, w.check(watchdogStart.Add(9*time.Minute), 0))
	require.NoError(t, w.check(watchdogStart.Add(9*time.Minute+30*time.Second), 1))

	// Another nine minutes of silence is 18m30s into the segment, but only 9m since progress.
	require.NoError(t, w.check(watchdogStart.Add(18*time.Minute+30*time.Second), 1))
	assert.Equal(t, 9*time.Minute, w.sinceLastProgress(watchdogStart.Add(18*time.Minute+30*time.Second)))

	err := w.check(watchdogStart.Add(20*time.Minute), 1)
	assert.ErrorIs(t, err, ErrRequestStalled)
}

// TestSegmentWatchdog_AbsoluteBackstop covers a segment that keeps inching forward forever: the
// stall deadline never fires, so the absolute deadline must.
func TestSegmentWatchdog_AbsoluteBackstop(t *testing.T) {
	w := newSegmentWatchdog(watchdogStart, 0, 10*time.Minute, 1*time.Hour)

	var processed uint64
	var lastErr error
	for elapsed := 5 * time.Minute; elapsed <= 2*time.Hour; elapsed += 5 * time.Minute {
		processed++
		if err := w.check(watchdogStart.Add(elapsed), processed); err != nil {
			lastErr = err
			assert.Greater(t, elapsed, 1*time.Hour, "backstop fired before the absolute deadline")
			break
		}
	}

	require.Error(t, lastErr)
	assert.ErrorIs(t, lastErr, ErrRequestActiveForTooLong)
	assert.Equal(t, connect.CodeDeadlineExceeded, connect.CodeOf(lastErr))
}

// TestSegmentWatchdog_StallTakesPrecedence documents that when both deadlines are blown, the stall
// error is reported: it is the more actionable diagnosis.
func TestSegmentWatchdog_StallTakesPrecedence(t *testing.T) {
	w := newSegmentWatchdog(watchdogStart, 0, 10*time.Minute, 1*time.Hour)

	err := w.check(watchdogStart.Add(2*time.Hour), 0)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrRequestStalled)
}

func TestSegmentWatchdog_ZeroTimeoutsDisableDeadlines(t *testing.T) {
	w := newSegmentWatchdog(watchdogStart, 0, 0, 0)

	assert.NoError(t, w.check(watchdogStart.Add(72*time.Hour), 0),
		"both deadlines disabled means the segment is never killed by the watchdog")
}

// TestSegmentWatchdog_ErrorsAreDistinguishable guards the two abort reasons staying separable,
// since they call for different operator responses.
func TestSegmentWatchdog_ErrorsAreDistinguishable(t *testing.T) {
	assert.False(t, errors.Is(ErrRequestStalled, ErrRequestActiveForTooLong))
	assert.False(t, errors.Is(ErrRequestActiveForTooLong, ErrRequestStalled))
}

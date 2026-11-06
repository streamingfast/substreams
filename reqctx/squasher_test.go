package reqctx

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

type manualClock struct {
	mu sync.Mutex
	at time.Time
}

func (c *manualClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.at
}

func (c *manualClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.at = c.at.Add(d)
}

func TestRemoteAvailabilityStaysLocalUntilProbe(t *testing.T) {
	t.Parallel()

	clock := &manualClock{at: time.Unix(1_000, 0)}
	quiet := 30 * time.Second
	gate := NewRemoteAvailability(quiet, clock.Now)

	assert.False(t, gate.PreferLocal())

	gate.MarkDown()
	assert.True(t, gate.PreferLocal())

	clock.Advance(quiet - time.Second)
	assert.True(t, gate.PreferLocal())

	clock.Advance(time.Second)
	assert.False(t, gate.PreferLocal(), "the quiet period elapsed, one caller probes")
	assert.True(t, gate.PreferLocal(), "everyone else stays local while that probe is out")

	gate.MarkDown()
	assert.True(t, gate.PreferLocal(), "a failed probe starts another quiet period")

	clock.Advance(quiet)
	assert.False(t, gate.PreferLocal())
	assert.True(t, gate.MarkUp())
	assert.False(t, gate.PreferLocal(), "a successful probe resumes the remote")
	assert.False(t, gate.MarkUp())
}

func TestRemoteSquasherSharesAvailability(t *testing.T) {
	t.Parallel()

	gate := NewRemoteAvailability(time.Minute, func() time.Time { return time.Unix(0, 0) })
	a := &RemoteSquasher{Availability: gate}
	b := &RemoteSquasher{Availability: gate}

	assert.False(t, a.PreferLocal())
	a.MarkRemoteDown()
	assert.True(t, b.PreferLocal())

	assert.True(t, b.MarkRemoteUp())
	assert.False(t, a.PreferLocal())
}

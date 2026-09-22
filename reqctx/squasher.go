package reqctx

import (
	"context"
	"sync"
	"time"

	"github.com/streamingfast/substreams/squash"
)

var remoteSquasherKey = contextKeyType(17)

// RemoteSquasher is the per-request handle tier1 uses to call a remote squash
// implementation. Nil (or a context without one) means squash in-process.
// When set, each squash run is one Squash RPC per store module, covering every
// claimed segment, so tier1 does not read store files for merging.
//
// Availability is shared by every request on the process. Nil means each run
// tries the remote; a non-nil gate sticks to local squashing after the remote
// stops answering, until a later attempt succeeds.
type RemoteSquasher struct {
	Client         squash.Client
	StateStoreURL  string
	CacheTag       string
	StoreSizeLimit uint64
	Availability   *RemoteAvailability
}

// PreferLocal reports whether squash should skip the remote and run in-process.
// One caller whose quiet period has elapsed is let through to probe; the rest
// stay local until that attempt reports back.
func (r *RemoteSquasher) PreferLocal() bool {
	if r == nil || r.Availability == nil {
		return false
	}
	return r.Availability.PreferLocal()
}

// MarkRemoteDown starts a quiet period during which PreferLocal stays true.
func (r *RemoteSquasher) MarkRemoteDown() {
	if r == nil || r.Availability == nil {
		return
	}
	r.Availability.MarkDown()
}

// MarkRemoteUp clears a quiet period. recovered is true when one was in effect,
// including the lease taken by the probe that just succeeded.
func (r *RemoteSquasher) MarkRemoteUp() (recovered bool) {
	if r == nil || r.Availability == nil {
		return false
	}
	return r.Availability.MarkUp()
}

// RemoteAvailability is the process-wide gate for a remote squasher. A remote
// that stops answering is left alone for quietFor; squash stays local until a
// later attempt gets through and succeeds.
type RemoteAvailability struct {
	mu        sync.Mutex
	downUntil time.Time
	quietFor  time.Duration
	now       func() time.Time
}

func NewRemoteAvailability(quietFor time.Duration, now func() time.Time) *RemoteAvailability {
	if now == nil {
		now = time.Now
	}
	return &RemoteAvailability{quietFor: quietFor, now: now}
}

func (a *RemoteAvailability) PreferLocal() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.downUntil.IsZero() {
		return false
	}
	now := a.now()
	if now.Before(a.downUntil) {
		return true
	}
	// Hold the gate for everyone else while this caller probes. MarkDown
	// extends it if the probe fails; MarkUp clears it if the remote answers.
	a.downUntil = now.Add(a.quietFor)
	return false
}

func (a *RemoteAvailability) MarkDown() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.downUntil = a.now().Add(a.quietFor)
}

func (a *RemoteAvailability) MarkUp() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	recovered := !a.downUntil.IsZero()
	a.downUntil = time.Time{}
	return recovered
}

func WithRemoteSquasher(ctx context.Context, squasher *RemoteSquasher) context.Context {
	return context.WithValue(ctx, remoteSquasherKey, squasher)
}

func GetRemoteSquasher(ctx context.Context) *RemoteSquasher {
	val := ctx.Value(remoteSquasherKey)
	if s, ok := val.(*RemoteSquasher); ok {
		return s
	}
	return nil
}

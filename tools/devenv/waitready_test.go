package devenv

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/streamingfast/shutter"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// fakeApp has the shape of both tiers: Run launches in the background and returns nil, and
// anything that fails later is reported through the shutter.
type fakeApp struct {
	*shutter.Shutter
	runErr error
}

func newFakeApp(runErr error) *fakeApp {
	return &fakeApp{Shutter: shutter.New(), runErr: runErr}
}

func (a *fakeApp) Run() error { return a.runErr }

func never(context.Context) bool { return false }

// Without watching for termination this would sit for the whole timeout and then report only
// "timeout after 1m0s", with the real cause buried in a log line printed a minute earlier.
func TestWaitReady_SurfacesStartupFailure(t *testing.T) {
	boom := errors.New("listen tcp :9000: bind: address already in use")

	err := waitReady(t.Context(), time.Minute, never, runApp(newFakeApp(boom), "tier1", zap.NewNop()))

	require.Error(t, err)
	assert.ErrorIs(t, err, boom)
	assert.NotContains(t, err.Error(), "timeout")
}

// A tier that starts and then dies — a store it cannot read, a listener that goes away — never
// reports ready either, and the shutter is the only place that says why.
func TestWaitReady_SurfacesFailureAfterStartup(t *testing.T) {
	boom := errors.New("cannot open state store")

	app := newFakeApp(nil)
	terminated := runApp(app, "tier1", zap.NewNop())
	app.Shutdown(boom)

	err := waitReady(t.Context(), time.Minute, never, terminated)

	require.Error(t, err)
	assert.ErrorIs(t, err, boom)
}

func TestWaitReady_TerminatedCleanlyBeforeReady(t *testing.T) {
	app := newFakeApp(nil)
	terminated := runApp(app, "tier1", zap.NewNop())
	app.Shutdown(nil)

	err := waitReady(t.Context(), time.Minute, never, terminated)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "terminated before becoming ready")
}

// The normal path: Run returns nil immediately because the listener is in a goroutine, which
// must not be mistaken for the app having finished.
func TestWaitReady_NilRunReturnIsNotAFailure(t *testing.T) {
	var ready atomic.Bool
	app := newFakeApp(nil)
	terminated := runApp(app, "tier1", zap.NewNop())

	time.AfterFunc(50*time.Millisecond, func() { ready.Store(true) })

	assert.NoError(t, waitReady(t.Context(), time.Minute, func(context.Context) bool { return ready.Load() }, terminated))
}

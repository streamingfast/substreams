package work

import (
	"context"
	"testing"

	"github.com/streamingfast/dsession/local"
	"github.com/streamingfast/substreams/reqctx"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// Workers still borrowed when a request ends must be returned before its session is
// released: the local session pool cannot decrement its counters for a worker whose
// session is gone, and the leaked workers eventually starve every later request.
func TestSessionWorkerPoolReleaseAllBeforeSessionRelease(t *testing.T) {
	sessionPool, err := local.NewLocalSessionPool("local://?max_workers=4&max_workers_per_organization=4", zap.NewNop())
	require.NoError(t, err)

	borrowAll := func() (*SessionWorkerPool, string, context.CancelFunc) {
		ctx, cancel := context.WithCancel(reqctx.WithRequest(context.Background(), &reqctx.RequestDetails{MaxParallelJobs: 4}))
		sessionID, err := sessionPool.Get(ctx, "t1r", "org", "key", "trace", nil)
		require.NoError(t, err)
		pool := NewSessionWorkerPool(ctx, sessionID, sessionPool, nil)
		pool.rampingUp.Store(false)
		for i := 0; i < 4; i++ {
			_, err := pool.Borrow(ctx)
			require.NoError(t, err)
		}
		_, err = pool.Borrow(ctx)
		require.ErrorIs(t, err, ErrorResourceExhausted)
		return pool, sessionID, cancel
	}

	pool, sessionID, cancel := borrowAll()
	pool.ReleaseAll()
	sessionPool.Release(sessionID)
	cancel()

	// every worker is back: a new request can borrow them all again
	pool, sessionID, cancel = borrowAll()
	pool.ReleaseAll()
	sessionPool.Release(sessionID)
	cancel()
}

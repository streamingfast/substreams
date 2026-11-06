package exec

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/streamingfast/substreams/metrics"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/reqctx"
	"github.com/streamingfast/substreams/wasm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// failingModule simulates a wasm.Module whose ExecuteNewCall fails before an
// instance is created (e.g. wazero instantiation or writeToHeap failure),
// returning a nil instance alongside the error.
type failingModule struct {
	err error
}

func (f *failingModule) NewInstance(ctx context.Context) (wasm.Instance, error) {
	return nil, errors.New("not implemented")
}

func (f *failingModule) ExecuteNewCall(ctx context.Context, call *wasm.Call, cachedInstance wasm.Instance, arguments []wasm.Argument, argValues map[string][]byte) (wasm.Instance, error) {
	return nil, f.err
}

func (f *failingModule) Close(ctx context.Context) error {
	return nil
}

func TestSharedCacheExecute_ExecuteNewCallFailureReturnsErrorWithoutPanic(t *testing.T) {
	stats := metrics.NewReqStats(&metrics.Config{}, nil, nil, zap.NewNop())
	ctx := reqctx.WithReqStats(context.Background(), stats)
	ctx = reqctx.WithRequest(ctx, &reqctx.RequestDetails{UniqueID: 1})

	clock := &pbsubstreams.Clock{Id: "block-1", Number: 1}
	call := wasm.NewCall(ctx, clock, "test_module", "test_entrypoint", stats, nil, false, nil)

	sharedCache := NewSharedCache(10)
	execErr := errors.New("could not instantiate wasm module: boom")

	var err error
	require.NotPanics(t, func() {
		err = sharedCache.Execute(ctx, &failingModule{err: execErr}, "modulehash", call, nil, nil, nil)
	})
	require.ErrorIs(t, err, execErr)
}

// scriptedModule returns errs[i] on its i-th ExecuteNewCall, and no error once errs is exhausted.
// When started is set, the first call closes it and waits for release before returning.
type scriptedModule struct {
	failingModule
	mu      sync.Mutex
	errs    []error
	calls   int
	started chan struct{}
	release chan struct{}
}

func (m *scriptedModule) ExecuteNewCall(ctx context.Context, call *wasm.Call, cachedInstance wasm.Instance, arguments []wasm.Argument, argValues map[string][]byte) (wasm.Instance, error) {
	m.mu.Lock()
	idx := m.calls
	m.calls++
	m.mu.Unlock()

	if idx == 0 && m.started != nil {
		close(m.started)
		<-m.release
	}
	if idx < len(m.errs) {
		return nil, m.errs[idx]
	}
	return nil, nil
}

func (m *scriptedModule) callCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.calls
}

func newSharedCacheTestCall(t *testing.T) (context.Context, *wasm.Call) {
	t.Helper()
	stats := metrics.NewReqStats(&metrics.Config{}, nil, nil, zap.NewNop())
	ctx := reqctx.WithReqStats(context.Background(), stats)
	ctx = reqctx.WithRequest(ctx, &reqctx.RequestDetails{UniqueID: 1})
	clock := &pbsubstreams.Clock{Id: "block-1", Number: 1}
	return ctx, wasm.NewCall(ctx, clock, "test_module", "test_entrypoint", stats, nil, false, nil)
}

func TestSharedCacheExecute_DeterministicErrorIsReplayed(t *testing.T) {
	sharedCache := NewSharedCache(10)
	execErr := errors.New("calling entrypoint: unreachable")
	module := &scriptedModule{errs: []error{execErr}}

	for range 2 {
		ctx, call := newSharedCacheTestCall(t)
		require.ErrorIs(t, sharedCache.Execute(ctx, module, "modulehash", call, nil, nil, nil), execErr)
	}
	assert.Equal(t, 1, module.callCount())
}

func TestSharedCacheExecute_TransientErrorIsNotReplayed(t *testing.T) {
	sharedCache := NewSharedCache(10)
	execErr := errors.New("creating new instance: cannot create a memfd: Too many open files (os error 24)")
	module := &scriptedModule{errs: []error{execErr}}

	ctx, call := newSharedCacheTestCall(t)
	require.ErrorIs(t, sharedCache.Execute(ctx, module, "modulehash", call, nil, nil, nil), execErr)

	ctx, call = newSharedCacheTestCall(t)
	require.NoError(t, sharedCache.Execute(ctx, module, "modulehash", call, nil, nil, nil))
	assert.Equal(t, 2, module.callCount())
}

func TestSharedCacheExecute_WaiterRetriesAfterTransientError(t *testing.T) {
	sharedCache := NewSharedCache(10)
	execErr := errors.New("creating new instance: cannot create a memfd: Too many open files (os error 24)")
	module := &scriptedModule{
		errs:    []error{execErr},
		started: make(chan struct{}),
		release: make(chan struct{}),
	}

	firstErr := make(chan error, 1)
	go func() {
		ctx, call := newSharedCacheTestCall(t)
		firstErr <- sharedCache.Execute(ctx, module, "modulehash", call, nil, nil, nil)
	}()
	<-module.started

	waiterErr := make(chan error, 1)
	go func() {
		ctx, call := newSharedCacheTestCall(t)
		waiterErr <- sharedCache.Execute(ctx, module, "modulehash", call, nil, nil, nil)
	}()
	// give the waiter time to block on the entry held by the first execution
	time.Sleep(50 * time.Millisecond)
	close(module.release)

	require.ErrorIs(t, <-firstErr, execErr)
	require.NoError(t, <-waiterErr)
	assert.Equal(t, 2, module.callCount())
}

package pipeline

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/streamingfast/bstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRecoverExecutionPanic_DeadlineExceeded is the regression test for the
// silent-swallow bug: when a WASM host-function panic coincides with the
// per-block execution deadline, recoverExecutionPanic must return a
// CodeDeadlineExceeded connect error — not nil — so the stream terminates
// instead of skipping the block.
func TestRecoverExecutionPanic_DeadlineExceeded(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()
	// Wait for the deadline to fire.
	<-ctx.Done()
	require.ErrorIs(t, ctx.Err(), context.DeadlineExceeded)

	blockRef := bstream.NewBlockRef("blk-id", 42)
	recovered := fmt.Errorf("running wasm extension %q: %w", "eth::eth_call",
		fmt.Errorf("timeout while doing eth_call: %w", context.DeadlineExceeded))

	got := recoverExecutionPanic(ctx, nil, recovered, blockRef)

	require.NotNil(t, got, "deadline-exceeded panic must not be silently swallowed")

	var ce *connect.Error
	require.ErrorAs(t, got, &ce, "result must be a connect.Error")
	assert.Equal(t, connect.CodeDeadlineExceeded, ce.Code())

	// The original error chain must be preserved so callers can introspect.
	assert.ErrorIs(t, got, context.DeadlineExceeded)
	assert.Contains(t, got.Error(), "blk-id")
}

// TestRecoverExecutionPanic_ContextCanceled covers the non-deadline cancel
// path: the recover must return the caller's executionError as-is (we don't
// invent a new error when the request was cancelled).
func TestRecoverExecutionPanic_ContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.ErrorIs(t, ctx.Err(), context.Canceled)

	blockRef := bstream.NewBlockRef("blk-id", 42)
	executionErr := errors.New("some prior error")

	got := recoverExecutionPanic(ctx, executionErr, errors.New("panicked"), blockRef)
	assert.Same(t, executionErr, got, "canceled ctx should return executionError unchanged")
}

// TestRecoverExecutionPanic_RecoveredIsCanceled covers the case where ctx
// itself is alive but the panic value contains context.Canceled (e.g.
// propagated through wrapping).
func TestRecoverExecutionPanic_RecoveredIsCanceled(t *testing.T) {
	ctx := context.Background()
	blockRef := bstream.NewBlockRef("blk-id", 42)
	executionErr := errors.New("some prior error")

	recovered := fmt.Errorf("wrapped: %w", context.Canceled)
	got := recoverExecutionPanic(ctx, executionErr, recovered, blockRef)
	assert.Same(t, executionErr, got)
}

// TestRecoverExecutionPanic_GenericPanic verifies the fallback path: a real
// runtime panic with a live ctx should yield a "panic at block ..." error
// that wraps the recovered value.
func TestRecoverExecutionPanic_GenericPanic(t *testing.T) {
	ctx := context.Background()
	blockRef := bstream.NewBlockRef("blk-id", 42)

	recovered := errors.New("nil pointer dereference")
	got := recoverExecutionPanic(ctx, nil, recovered, blockRef)

	require.NotNil(t, got)
	assert.ErrorIs(t, got, recovered)
	assert.Contains(t, got.Error(), "blk-id")
}

// TestRecoverExecutionPanic_NonErrorRecovered handles the case where the
// panic value isn't an `error` (e.g. a plain string panic from somewhere).
func TestRecoverExecutionPanic_NonErrorRecovered(t *testing.T) {
	ctx := context.Background()
	blockRef := bstream.NewBlockRef("blk-id", 42)

	got := recoverExecutionPanic(ctx, nil, "something broke", blockRef)

	require.NotNil(t, got)
	assert.Contains(t, got.Error(), "something broke")
	assert.Contains(t, got.Error(), "blk-id")
}

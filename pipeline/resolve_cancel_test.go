package pipeline

import (
	"context"
	"errors"
	"testing"

	"github.com/streamingfast/bstream"
	"github.com/streamingfast/bstream/hub"
	"github.com/streamingfast/dstore"
	"github.com/stretchr/testify/require"
)

// TestCursorResolver_CancellationDoesNotEmitUndo covers the residual race of
// issue #399: when the context is cancelled, the source shutdown goroutine may
// terminate the source before the ctx.Done() select case is picked. If the
// src.Terminated() case wins, the resolver used to fall through to the error
// fallback and emit an undo-to-LIB signal (junction = cursor.LIB, err = nil)
// instead of propagating the cancellation.
//
// The select choice between the two ready channels is randomized by the
// runtime, so we run several iterations: every single one must surface the
// context error, never an undo signal.
func TestCursorResolver_CancellationDoesNotEmitUndo(t *testing.T) {
	mergedBlocksStore := dstore.NewMockStore(nil)
	forkedBlocksStore := dstore.NewMockStore(nil)

	resolver := NewCursorResolver((*hub.ForkableHub)(nil), mergedBlocksStore, forkedBlocksStore)

	cursor := &bstream.Cursor{
		Step:      bstream.StepNew,
		Block:     bstream.NewBlockRef("bb", 105),
		HeadBlock: bstream.NewBlockRef("bb", 105),
		LIB:       bstream.NewBlockRef("aa", 100),
	}

	for i := 0; i < 30; i++ {
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // cancelled before resolution starts

		junction, head, err := resolver(ctx, cursor)
		require.Error(t, err, "iteration %d: cancellation was swallowed, got junction=%v head=%v (undo-to-LIB signal)", i, junction, head)
		require.True(t, errors.Is(err, context.Canceled), "iteration %d: expected context.Canceled, got: %v", i, err)
	}
}

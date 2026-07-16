package service

import (
	"context"
	"fmt"
	"testing"

	"github.com/streamingfast/substreams/wasm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// TestToGRPCError_FoundationalStoreFatalIsInternal documents how a foundational
// store fatal error (auth failure, org id mismatch, prolonged unreachability)
// crosses the tier2 -> tier1 boundary.
//
// Unlike a deterministic wasm error (codes.InvalidArgument, which tier1 treats
// as non-retryable), a foundational store fatal error is non-deterministic and
// is emitted as codes.Internal. tier1's RemoteWorker.work treats codes.Internal
// as retryable, so the segment is retried up to workerMaxRetries (5) times
// before the error bubbles up to the user, uncached.
func TestToGRPCError_FoundationalStoreFatalIsInternal(t *testing.T) {
	ctx := context.Background()

	// Mirrors how the error reaches tier2's toGRPCError: wrapped a few times as
	// it unwinds from the wasm host call through process_block.
	fatal := fmt.Errorf("running executor %q: %w", "mymodule",
		fmt.Errorf("execute module: %w",
			fmt.Errorf("module %q: %w", "mymodule",
				fmt.Errorf("%w: store unreachable: %s", wasm.ErrFoundationalStoreFatal, "connection refused"))))

	grpcErr := toGRPCError(ctx, fatal)
	require.Error(t, grpcErr)

	st, ok := status.FromError(grpcErr)
	require.True(t, ok, "expected a gRPC status error")
	assert.Equal(t, codes.Internal, st.Code(),
		"foundational store fatal errors must be Internal (non-deterministic, retried by tier1 then bubbled up), not InvalidArgument")
}

// TestToGRPCError_DeterministicIsInvalidArgument is the contrast case: a
// deterministic wasm error is emitted as InvalidArgument and is NOT retried by
// tier1 (and is cached on tier2).
func TestToGRPCError_DeterministicIsInvalidArgument(t *testing.T) {
	ctx := context.Background()

	det := fmt.Errorf("module %q: %w", "mymodule", wasm.ErrWasmDeterministicExec)

	grpcErr := toGRPCError(ctx, det)
	require.Error(t, grpcErr)

	st, ok := status.FromError(grpcErr)
	require.True(t, ok, "expected a gRPC status error")
	assert.Equal(t, codes.InvalidArgument, st.Code())
}

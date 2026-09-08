package exec

import (
	"context"
	"errors"
	"testing"

	"github.com/streamingfast/substreams/metrics"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/reqctx"
	"github.com/streamingfast/substreams/wasm"
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

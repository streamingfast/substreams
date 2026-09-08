package sink

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
	pbsubstreamsrpcv3 "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v3"
)

// The sinker reaches the optional callbacks through a type assertion on the `SinkerHandler` value,
// so a handler whose signature drifted from the interface is silently never called. Assert on the
// value actually returned by the constructors, the compile-time assertions in types.go only cover
// the concrete type.
func TestNewSinkerFullHandlers_SatisfiesOptionalInterfaces(t *testing.T) {
	noopData := func(ctx context.Context, data *pbsubstreamsrpc.BlockScopedData, isLive *bool, cursor *Cursor) error {
		return nil
	}
	noopUndo := func(ctx context.Context, undoSignal *pbsubstreamsrpc.BlockUndoSignal, cursor *Cursor) error {
		return nil
	}

	for name, handler := range map[string]SinkerHandler{
		"NewSinkerFullHandlers":            NewSinkerFullHandlers(noopData, noopUndo, nil, nil, nil, nil, nil),
		"NewSinkerFullHandlersWithPartial": NewSinkerFullHandlersWithPartial(noopData, noopUndo, nil, nil, nil, nil, nil),
	} {
		t.Run(name, func(t *testing.T) {
			_, ok := handler.(SinkerSessionInitHandler)
			assert.True(t, ok, "must implement SinkerSessionInitHandler, otherwise the session (and its trace ID) never reaches the caller")

			_, ok = handler.(SinkerErrorHandler)
			assert.True(t, ok, "must implement SinkerErrorHandler, otherwise retried stream errors never reach the caller")

			_, ok = handler.(SinkerProgressHandler)
			assert.True(t, ok, "must implement SinkerProgressHandler")

			_, ok = handler.(SinkerSnapshotHandler)
			assert.True(t, ok, "must implement SinkerSnapshotHandler")
		})
	}
}

// The callbacks must effectively be forwarded, not just declared with the right signature.
func TestFullSinkerHandlers_ForwardsSessionInitAndError(t *testing.T) {
	var gotSession *pbsubstreamsrpc.SessionInit
	var gotErr error

	handler := NewSinkerFullHandlers(
		func(ctx context.Context, data *pbsubstreamsrpc.BlockScopedData, isLive *bool, cursor *Cursor) error {
			return nil
		},
		func(ctx context.Context, undoSignal *pbsubstreamsrpc.BlockUndoSignal, cursor *Cursor) error {
			return nil
		},
		func(ctx context.Context, req *pbsubstreamsrpcv3.Request, session *pbsubstreamsrpc.SessionInit) error {
			gotSession = session
			return nil
		},
		nil,
		nil,
		nil,
		func(ctx context.Context, err error) {
			gotErr = err
		},
	)

	session := &pbsubstreamsrpc.SessionInit{TraceId: "0123456789abcdef"}

	sessionHandler, ok := handler.(SinkerSessionInitHandler)
	assert.True(t, ok)
	assert.NoError(t, sessionHandler.HandleSessionInit(context.Background(), &pbsubstreamsrpcv3.Request{}, session))
	assert.Same(t, session, gotSession)

	errHandler, ok := handler.(SinkerErrorHandler)
	assert.True(t, ok)
	errHandler.HandleError(context.Background(), assert.AnError)
	assert.Same(t, assert.AnError, gotErr)
}

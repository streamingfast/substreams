package sink

import (
	"context"
	"errors"
	"testing"

	pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
	pbsubstreamsrpcv3 "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// noopHandler implements SinkerHandler but NOT SinkerSessionInitHandler.
type noopHandler struct{}

func (noopHandler) HandleBlockScopedData(ctx context.Context, data *pbsubstreamsrpc.BlockScopedData, isLive *bool, cursor *Cursor) error {
	return nil
}

func (noopHandler) HandleBlockUndoSignal(ctx context.Context, undoSignal *pbsubstreamsrpc.BlockUndoSignal, cursor *Cursor) error {
	return nil
}

// sessionInitRecordingHandler implements both SinkerHandler and SinkerSessionInitHandler, recording
// the arguments it was invoked with so the test can assert dispatch happened correctly.
type sessionInitRecordingHandler struct {
	noopHandler

	calls       int
	gotReq      *pbsubstreamsrpcv3.Request
	gotSession  *pbsubstreamsrpc.SessionInit
	returnError error
}

func (h *sessionInitRecordingHandler) HandleSessionInit(ctx context.Context, req *pbsubstreamsrpcv3.Request, sessionInit *pbsubstreamsrpc.SessionInit) error {
	h.calls++
	h.gotReq = req
	h.gotSession = sessionInit
	return h.returnError
}

func newTestSinker() *Sinker {
	return &Sinker{
		SinkerConfig: &SinkerConfig{Logger: zlog},
		request:      &pbsubstreamsrpcv3.Request{StartBlockNum: 100},
	}
}

func TestSinker_handleSessionInit(t *testing.T) {
	const resolvedStartBlock = uint64(12345)

	t.Run("no session-init handler still sets requestActiveStartBlock", func(t *testing.T) {
		s := newTestSinker()
		session := &pbsubstreamsrpc.SessionInit{ResolvedStartBlock: resolvedStartBlock}

		err := s.handleSessionInit(context.Background(), noopHandler{}, session)
		require.NoError(t, err)
		assert.Equal(t, resolvedStartBlock, s.requestActiveStartBlock)
	})

	t.Run("session-init handler invoked and requestActiveStartBlock still set", func(t *testing.T) {
		s := newTestSinker()
		handler := &sessionInitRecordingHandler{}
		session := &pbsubstreamsrpc.SessionInit{ResolvedStartBlock: resolvedStartBlock}

		err := s.handleSessionInit(context.Background(), handler, session)
		require.NoError(t, err)

		// Handler was invoked exactly once with the sinker's request and the received session.
		assert.Equal(t, 1, handler.calls)
		assert.Same(t, s.request, handler.gotReq)
		assert.Same(t, session, handler.gotSession)

		// Regression assertion for the bug: the internal bookkeeping must run even when a custom
		// session-init handler is installed (previously a `break` short-circuited this assignment).
		assert.Equal(t, resolvedStartBlock, s.requestActiveStartBlock)
	})

	t.Run("session-init handler error is propagated and wrapped", func(t *testing.T) {
		s := newTestSinker()
		sentinel := errors.New("boom")
		handler := &sessionInitRecordingHandler{returnError: sentinel}
		session := &pbsubstreamsrpc.SessionInit{ResolvedStartBlock: resolvedStartBlock}

		err := s.handleSessionInit(context.Background(), handler, session)
		require.Error(t, err)
		assert.ErrorIs(t, err, sentinel)

		// On handler failure we never reach the bookkeeping, so the field stays at its zero value.
		assert.Zero(t, s.requestActiveStartBlock)
	})
}

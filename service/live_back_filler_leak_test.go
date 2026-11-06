package service

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	pbssinternal "github.com/streamingfast/substreams/pb/sf/substreams/intern/v2"
	"github.com/streamingfast/substreams/client"
	"github.com/streamingfast/substreams/reqctx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

type fakeProcessRangeClientStream struct {
	recvErr error
}

func (s *fakeProcessRangeClientStream) Recv() (*pbssinternal.ProcessRangeResponse, error) {
	return nil, s.recvErr
}
func (s *fakeProcessRangeClientStream) Header() (metadata.MD, error) { return nil, nil }
func (s *fakeProcessRangeClientStream) Trailer() metadata.MD         { return nil }
func (s *fakeProcessRangeClientStream) CloseSend() error             { return nil }
func (s *fakeProcessRangeClientStream) Context() context.Context     { return context.Background() }
func (s *fakeProcessRangeClientStream) SendMsg(any) error            { return nil }
func (s *fakeProcessRangeClientStream) RecvMsg(any) error            { return nil }

type fakeSubstreamsClient struct {
	processRangeErr error
	recvErr         error
}

func (c *fakeSubstreamsClient) ProcessRange(ctx context.Context, in *pbssinternal.ProcessRangeRequest, opts ...grpc.CallOption) (grpc.ServerStreamingClient[pbssinternal.ProcessRangeResponse], error) {
	if c.processRangeErr != nil {
		return nil, c.processRangeErr
	}
	return &fakeProcessRangeClientStream{recvErr: c.recvErr}, nil
}

func newCountingClientFactory(fakeClient *fakeSubstreamsClient, closeCount *int) client.InternalClientFactory {
	return func() (pbssinternal.SubstreamsClient, func() error, []grpc.CallOption, client.Headers, error) {
		closeFunc := func() error {
			*closeCount++
			return nil
		}
		return fakeClient, closeFunc, nil, nil, nil
	}
}

// TestRequestBackProcessing_ConnectionClosed verifies that the grpc client
// connection created by the clientFactory is closed exactly once on every
// code path of requestBackProcessing: previously, error returns leaked the
// connection and the success path closed it twice.
func TestRequestBackProcessing_ConnectionClosed(t *testing.T) {
	cases := []struct {
		name        string
		fakeClient  *fakeSubstreamsClient
		expectError bool
	}{
		{
			name:        "error getting stream",
			fakeClient:  &fakeSubstreamsClient{processRangeErr: errors.New("connection refused")},
			expectError: true,
		},
		{
			name:        "error receiving from stream",
			fakeClient:  &fakeSubstreamsClient{recvErr: errors.New("stream broken")},
			expectError: true,
		},
		{
			name:        "success",
			fakeClient:  &fakeSubstreamsClient{recvErr: io.EOF},
			expectError: false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			closeCount := 0
			factory := newCountingClientFactory(c.fakeClient, &closeCount)

			request := &pbssinternal.ProcessRangeRequest{SegmentSize: 10}
			err := requestBackProcessing(context.Background(), zap.NewNop(), request, factory)
			if c.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, 1, closeCount, "clientFactory closeFunc must be called exactly once")
		})
	}
}

// TestRequestBackProcessing_NoGoroutineLeakOnCancel verifies that
// RequestBackProcessing does not block forever on its result channel when the
// context is cancelled and nobody is reading the channel anymore (which is
// what happens when LiveBackFiller.Start returns on ctx.Done()).
func TestRequestBackProcessing_NoGoroutineLeakOnCancel(t *testing.T) {
	ctx := reqctx.WithRequest(context.Background(), &reqctx.RequestDetails{})
	ctx = reqctx.WithTier2RequestParameters(ctx, reqctx.Tier2RequestParameters{StateBundleSize: 10})
	ctx, cancel := context.WithCancel(ctx)
	cancel()

	closeCount := 0
	factory := newCountingClientFactory(&fakeSubstreamsClient{processRangeErr: errors.New("unreachable")}, &closeCount)

	jobResult := make(chan error) // unbuffered and never read, like after Start() has returned
	done := make(chan struct{})
	go func() {
		RequestBackProcessing(ctx, zap.NewNop(), 0, 0, factory, jobResult)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("RequestBackProcessing leaked: still blocked sending its result after context cancellation")
	}
}

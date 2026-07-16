package work

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/streamingfast/substreams"
	"github.com/streamingfast/substreams/client"
	"github.com/streamingfast/substreams/metrics"
	"github.com/streamingfast/substreams/orchestrator/response"
	"github.com/streamingfast/substreams/orchestrator/stage"
	pbssinternal "github.com/streamingfast/substreams/pb/sf/substreams/intern/v2"
	pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/reqctx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func TestMain(m *testing.M) {
	// work() touches tier1 worker metrics (e.g. Tier1ActiveWorkerRequest), which
	// are nil until declared.
	metrics.DeclareTier1Metrics(zap.NewNop())
	os.Exit(m.Run())
}

// fakeProcessRangeStream is a grpc.ServerStreamingClient that yields a single
// error on the first Recv (mimicking a tier2 worker that fails immediately, e.g.
// because a foundational store call returned an auth / org-mismatch error).
type fakeProcessRangeStream struct {
	err error
}

func (s *fakeProcessRangeStream) Recv() (*pbssinternal.ProcessRangeResponse, error) {
	return nil, s.err
}
func (s *fakeProcessRangeStream) Header() (metadata.MD, error) { return metadata.MD{}, nil }
func (s *fakeProcessRangeStream) Trailer() metadata.MD         { return nil }
func (s *fakeProcessRangeStream) CloseSend() error             { return nil }
func (s *fakeProcessRangeStream) Context() context.Context     { return context.Background() }
func (s *fakeProcessRangeStream) SendMsg(any) error            { return nil }
func (s *fakeProcessRangeStream) RecvMsg(any) error            { return nil }

// fakeSubstreamsClient counts ProcessRange calls and returns a stream that fails
// with the configured error.
type fakeSubstreamsClient struct {
	err   error
	calls int
}

func (c *fakeSubstreamsClient) ProcessRange(ctx context.Context, in *pbssinternal.ProcessRangeRequest, opts ...grpc.CallOption) (grpc.ServerStreamingClient[pbssinternal.ProcessRangeResponse], error) {
	c.calls++
	return &fakeProcessRangeStream{err: c.err}, nil
}

func testWorkerCtx() context.Context {
	ctx := context.Background()
	ctx = reqctx.WithRequest(ctx, &reqctx.RequestDetails{OutputModule: "m", Modules: &pbsubstreams.Modules{}})
	ctx = reqctx.WithTier2RequestParameters(ctx, reqctx.Tier2RequestParameters{StateBundleSize: 10})

	stats := metrics.NewReqStats(&metrics.Config{}, nil, nil, zap.NewNop())
	stats.RecordStages([]*pbsubstreamsrpc.Stage{{Modules: []string{"m"}}})
	ctx = reqctx.WithReqStats(ctx, stats)
	return ctx
}

func newFakeWorker(streamErr error) (*RemoteWorker, *fakeSubstreamsClient) {
	fake := &fakeSubstreamsClient{err: streamErr}
	factory := func() (pbssinternal.SubstreamsClient, func() error, []grpc.CallOption, client.Headers, error) {
		return fake, func() error { return nil }, nil, client.Headers{}, nil
	}
	return NewRemoteWorker(factory, "test-worker", zap.NewNop()), fake
}

// TestWork_FoundationalStoreFatalIsRetryable validates how tier1 classifies the
// gRPC error that a foundational store fatal failure arrives as. Tier2 emits it
// as codes.Internal (see service.toGRPCError), and tier1 treats codes.Internal
// as retryable, while a deterministic codes.InvalidArgument is non-retryable.
func TestWork_FoundationalStoreFatalIsRetryable(t *testing.T) {
	upstream := response.New(func(substreams.ResponseFromAnyTier) error { return nil })
	request := &pbssinternal.ProcessRangeRequest{SegmentNumber: 0, SegmentSize: 10}

	t.Run("internal (foundational store fatal) is retryable", func(t *testing.T) {
		w, _ := newFakeWorker(status.Error(codes.Internal, "module \"m\": foundational store request failed: store unreachable: connection refused"))
		res := w.work(testWorkerCtx(), request, nil, upstream, 0)

		var retryable *RetryableErr
		require.True(t, errors.As(res.Error, &retryable), "codes.Internal must be retryable, got: %v", res.Error)
	})

	t.Run("invalid argument (deterministic) is not retryable", func(t *testing.T) {
		w, _ := newFakeWorker(status.Error(codes.InvalidArgument, "module \"m\": boom (deterministic error)"))
		res := w.work(testWorkerCtx(), request, nil, upstream, 0)

		var retryable *RetryableErr
		require.False(t, errors.As(res.Error, &retryable), "codes.InvalidArgument must NOT be retryable, got: %v", res.Error)
		require.Error(t, res.Error)
	})
}

// TestWork_RetriesThenBubblesUp validates that a foundational store fatal error
// (arriving as codes.Internal) is retried exactly workerMaxRetries times before
// it gives up and bubbles up to the user as a failed job. workerMaxRetries is
// lowered here to keep the test fast (the real default is 5).
func TestWork_RetriesThenBubblesUp(t *testing.T) {
	prev := workerMaxRetries
	workerMaxRetries = 2
	defer func() { workerMaxRetries = prev }()

	w, fake := newFakeWorker(status.Error(codes.Internal, "module \"m\": foundational store request failed: organization id mismatch"))

	cmd := w.Work(testWorkerCtx(), stage.Unit{Stage: 0, Segment: 0}, 0, []string{"m"}, response.New(func(substreams.ResponseFromAnyTier) error { return nil }), false)
	msg := cmd()

	failed, ok := msg.(MsgJobFailed)
	require.True(t, ok, "expected MsgJobFailed, got %T", msg)
	require.Error(t, failed.Error)
	assert.Contains(t, failed.Error.Error(), "giving up", "error should indicate retries were exhausted")
	assert.Equal(t, workerMaxRetries, fake.calls, "segment must be attempted exactly workerMaxRetries times")
}

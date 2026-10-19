package work

import (
	"context"
	"io"
	"testing"

	"github.com/streamingfast/substreams"
	"github.com/streamingfast/substreams/client"
	"github.com/streamingfast/substreams/orchestrator/response"
	pbssinternal "github.com/streamingfast/substreams/pb/sf/substreams/intern/v2"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// failingDialClient fails ProcessRange itself, the way a dial failure does, for its first
// `failures` calls, then returns a stream that ends right away, which work() reads as a success.
type failingDialClient struct {
	failures int
	calls    int
}

func (c *failingDialClient) ProcessRange(ctx context.Context, in *pbssinternal.ProcessRangeRequest, opts ...grpc.CallOption) (grpc.ServerStreamingClient[pbssinternal.ProcessRangeResponse], error) {
	c.calls++
	if c.calls <= c.failures {
		return nil, status.Error(codes.Unavailable, "connection error: desc = \"transport: Error while dialing: dial tcp 10.0.0.1:9000: connect: connection refused\"")
	}
	return &fakeProcessRangeStream{err: io.EOF}, nil
}

// runFailingDialWork runs a single work() attempt against a failingDialClient and reports
// how many times the client it created was closed.
func runFailingDialWork(failures int) (res *Result, closed int) {
	fake := &failingDialClient{failures: failures}
	factory := func() (pbssinternal.SubstreamsClient, func() error, []grpc.CallOption, client.Headers, error) {
		return fake, func() error { closed++; return nil }, nil, client.Headers{}, nil
	}
	w := NewRemoteWorker(factory, "test-worker", zap.NewNop(), NewLaunchQueue(1))
	res = w.work(testWorkerCtx(), &pbssinternal.ProcessRangeRequest{}, nil, response.New(func(substreams.ResponseFromAnyTier) error { return nil }), 0, func() {})
	return res, closed
}

// TestWork_ClosesClient validates that work() closes the client it created exactly once,
// whether it could not reach a tier2 or the job ran.
func TestWork_ClosesClient(t *testing.T) {
	testCases := []struct {
		name     string
		failures int
	}{
		{name: "failed dial", failures: 1},
		{name: "job ran", failures: 0},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, closed := runFailingDialWork(tc.failures)
			assert.Equal(t, 1, closed)
		})
	}
}

// TestWork_ConnectionFailureKeepsCause validates that a failed dial reports the error that
// caused it, while still being retried as a job that never got into a tier2.
func TestWork_ConnectionFailureKeepsCause(t *testing.T) {
	res, _ := runFailingDialWork(1)

	assert.True(t, isJobRejectedErr(res.Error), "expected a rejected job, got %v", res.Error)
	assert.ErrorContains(t, res.Error, "connect: connection refused")
}

package work

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/streamingfast/substreams"
	"github.com/streamingfast/substreams/client"
	"github.com/streamingfast/substreams/orchestrator/response"
	"github.com/streamingfast/substreams/orchestrator/stage"
	pbssinternal "github.com/streamingfast/substreams/pb/sf/substreams/intern/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// scriptedSubstreamsClient returns one error per ProcessRange call, in order. Once the
// script is exhausted every stream ends with io.EOF, which work() reads as a success.
type scriptedSubstreamsClient struct {
	errs  []error
	calls int
}

func (c *scriptedSubstreamsClient) ProcessRange(ctx context.Context, in *pbssinternal.ProcessRangeRequest, opts ...grpc.CallOption) (grpc.ServerStreamingClient[pbssinternal.ProcessRangeResponse], error) {
	err := io.EOF
	if c.calls < len(c.errs) {
		err = c.errs[c.calls]
	}
	c.calls++
	return &fakeProcessRangeStream{err: err}, nil
}

func newScriptedWorker(errs ...error) (*RemoteWorker, *scriptedSubstreamsClient) {
	fake := &scriptedSubstreamsClient{errs: errs}
	factory := func() (pbssinternal.SubstreamsClient, func() error, []grpc.CallOption, client.Headers, error) {
		return fake, func() error { return nil }, nil, client.Headers{}, nil
	}
	return NewRemoteWorker(factory, "test-worker", zap.NewNop(), NewLaunchQueue(1)), fake
}

// TestWork_OverloadedRetriesFastAndDoesNotCount validates that a tier2 refusing the job at
// its concurrent-request limit is redialed on the launch queue's short delay rather than the
// growing Fibonacci backoff, and that those attempts are not charged against workerMaxRetries.
func TestWork_OverloadedRetriesFastAndDoesNotCount(t *testing.T) {
	prevRetries, prevDelay, prevJitter := workerMaxRetries, workerOverloadedRetryDelay, workerOverloadedRetryJitter
	workerMaxRetries, workerOverloadedRetryDelay, workerOverloadedRetryJitter = 2, 10*time.Millisecond, 5*time.Millisecond
	defer func() {
		workerMaxRetries, workerOverloadedRetryDelay, workerOverloadedRetryJitter = prevRetries, prevDelay, prevJitter
	}()

	overloaded := status.Error(codes.ResourceExhausted, "service currently overloaded")
	w, fake := newScriptedWorker(overloaded, overloaded, overloaded)

	start := time.Now()
	cmd := w.Work(testWorkerCtx(), stage.Unit{Stage: 0, Segment: 0}, 0, []string{"m"}, response.New(func(substreams.ResponseFromAnyTier) error { return nil }), false)
	msg := cmd()

	_, ok := msg.(MsgJobSucceeded)
	require.True(t, ok, "expected MsgJobSucceeded, got %T (%v)", msg, msg)
	assert.Equal(t, 4, fake.calls, "3 refusals then a successful attempt, none charged to workerMaxRetries")
	assert.Less(t, time.Since(start), time.Second, "refusals must not fall back to the Fibonacci backoff")
}

func TestIsInstanceOverloadedErr(t *testing.T) {
	assert.True(t, isInstanceOverloadedErr(status.Error(codes.ResourceExhausted, "service currently overloaded")))
	assert.True(t, isInstanceOverloadedErr(status.Error(codes.Unknown, "service currently overloaded")), "legacy tier2 message without the code")
	assert.False(t, isInstanceOverloadedErr(status.Error(codes.Unavailable, "no healthy upstream")), "whole fleet unreachable, keep the growing backoff")
	assert.False(t, isInstanceOverloadedErr(ErrConnectionRefused))
	assert.False(t, isInstanceOverloadedErr(status.Error(codes.Internal, "boom")))
}

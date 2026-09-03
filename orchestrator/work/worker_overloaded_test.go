package work

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/streamingfast/substreams"
	"github.com/streamingfast/substreams/client"
	"github.com/streamingfast/substreams/orchestrator/loop"
	"github.com/streamingfast/substreams/orchestrator/response"
	"github.com/streamingfast/substreams/orchestrator/stage"
	pbssinternal "github.com/streamingfast/substreams/pb/sf/substreams/intern/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
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

// blockingProcessRangeStream answers the first Recv, then holds the stream open until it
// is released, standing in for a tier2 that took the job and is running it.
type blockingProcessRangeStream struct {
	started  chan struct{}
	release  chan struct{}
	answered bool
}

func (s *blockingProcessRangeStream) Recv() (*pbssinternal.ProcessRangeResponse, error) {
	if !s.answered {
		s.answered = true
		close(s.started)
		return &pbssinternal.ProcessRangeResponse{}, nil
	}
	<-s.release
	return nil, io.EOF
}
func (s *blockingProcessRangeStream) Header() (metadata.MD, error) { return metadata.MD{}, nil }
func (s *blockingProcessRangeStream) Trailer() metadata.MD         { return nil }
func (s *blockingProcessRangeStream) CloseSend() error             { return nil }
func (s *blockingProcessRangeStream) Context() context.Context     { return context.Background() }
func (s *blockingProcessRangeStream) SendMsg(any) error            { return nil }
func (s *blockingProcessRangeStream) RecvMsg(any) error            { return nil }

// refusedThenBlockingClient refuses the first job the way a tier2 at its concurrent-request
// limit does, then takes the second one and keeps running it.
type refusedThenBlockingClient struct {
	blocking *blockingProcessRangeStream
	calls    int
}

func (c *refusedThenBlockingClient) ProcessRange(ctx context.Context, in *pbssinternal.ProcessRangeRequest, opts ...grpc.CallOption) (grpc.ServerStreamingClient[pbssinternal.ProcessRangeResponse], error) {
	c.calls++
	if c.calls == 1 {
		return &fakeProcessRangeStream{err: status.Error(codes.ResourceExhausted, "service currently overloaded")}, nil
	}
	return c.blocking, nil
}

// TestWork_LeavesLaunchQueueOnceAdmitted validates that a job stops holding its place in
// the launch queue as soon as a tier2 takes it, rather than when it finishes: the jobs
// behind it must be free to dial while it runs.
func TestWork_LeavesLaunchQueueOnceAdmitted(t *testing.T) {
	prevDelay, prevJitter := workerOverloadedRetryDelay, workerOverloadedRetryJitter
	workerOverloadedRetryDelay, workerOverloadedRetryJitter = 10*time.Millisecond, 0
	defer func() { workerOverloadedRetryDelay, workerOverloadedRetryJitter = prevDelay, prevJitter }()

	blocking := &blockingProcessRangeStream{started: make(chan struct{}), release: make(chan struct{})}
	fake := &refusedThenBlockingClient{blocking: blocking}
	factory := func() (pbssinternal.SubstreamsClient, func() error, []grpc.CallOption, client.Headers, error) {
		return fake, func() error { return nil }, nil, client.Headers{}, nil
	}

	queue := NewLaunchQueue(10)
	w := NewRemoteWorker(factory, "test-worker", zap.NewNop(), queue)

	unit := stage.Unit{Stage: 0, Segment: 0}
	done := make(chan loop.Msg, 1)
	go func() {
		cmd := w.Work(testWorkerCtx(), unit, 0, []string{"m"}, response.New(func(substreams.ResponseFromAnyTier) error { return nil }), false)
		done <- cmd()
	}()

	select {
	case <-blocking.started:
	case <-time.After(2 * time.Second):
		t.Fatal("worker never got into the fake tier2")
	}

	queue.mu.Lock()
	queued := len(queue.waiting)
	queue.mu.Unlock()
	assert.Equal(t, 0, queued, "a running job must not hold its place in the launch queue")

	close(blocking.release)
	select {
	case msg := <-done:
		_, ok := msg.(MsgJobSucceeded)
		assert.True(t, ok, "expected MsgJobSucceeded, got %T (%v)", msg, msg)
	case <-time.After(2 * time.Second):
		t.Fatal("job never completed")
	}
}

// TestWork_FailedJobWaitsOnceThenRetriesFast validates that the longer wait belongs to the
// failure itself: a tier2 refusing the job on the way back in puts it on the short delay
// again rather than on another failure wait.
func TestWork_FailedJobWaitsOnceThenRetriesFast(t *testing.T) {
	prevFailed, prevDelay, prevJitter := workerFailedRetryDelay, workerOverloadedRetryDelay, workerOverloadedRetryJitter
	workerFailedRetryDelay, workerOverloadedRetryDelay, workerOverloadedRetryJitter = 300*time.Millisecond, 10*time.Millisecond, 0
	defer func() {
		workerFailedRetryDelay, workerOverloadedRetryDelay, workerOverloadedRetryJitter = prevFailed, prevDelay, prevJitter
	}()

	overloaded := status.Error(codes.ResourceExhausted, "service currently overloaded")
	failed := status.Error(codes.Internal, "module \"m\": boom")
	w, fake := newScriptedWorker(failed, overloaded, overloaded)

	start := time.Now()
	cmd := w.Work(testWorkerCtx(), stage.Unit{Stage: 0, Segment: 0}, 0, []string{"m"}, response.New(func(substreams.ResponseFromAnyTier) error { return nil }), false)
	msg := cmd()
	took := time.Since(start)

	_, ok := msg.(MsgJobSucceeded)
	require.True(t, ok, "expected MsgJobSucceeded, got %T (%v)", msg, msg)
	assert.Equal(t, 4, fake.calls)
	assert.GreaterOrEqual(t, took, 300*time.Millisecond, "the failure waits once")
	assert.Less(t, took, 600*time.Millisecond, "the refusals that follow it do not")
}

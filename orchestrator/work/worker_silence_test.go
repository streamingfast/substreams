package work

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/streamingfast/substreams"
	"github.com/streamingfast/substreams/client"
	"github.com/streamingfast/substreams/orchestrator/loop"
	"github.com/streamingfast/substreams/orchestrator/response"
	"github.com/streamingfast/substreams/orchestrator/stage"
	pbssinternal "github.com/streamingfast/substreams/pb/sf/substreams/intern/v2"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/reqctx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// TestWork_SilenceTimeout validates that a job gives up on a tier2 only when that tier2 sent
// nothing for the silence timeout, and that neither a tier2 that keeps reporting nor a
// consumer that is slow to take a block is mistaken for one.
func TestWork_SilenceTimeout(t *testing.T) {
	const timeout = 150 * time.Millisecond

	update := &pbssinternal.ProcessRangeResponse{Type: &pbssinternal.ProcessRangeResponse_Update{Update: &pbssinternal.Update{}}}
	block := &pbssinternal.ProcessRangeResponse{Type: &pbssinternal.ProcessRangeResponse_BlockScopedData{
		BlockScopedData: &pbssinternal.BlockScopedData{Clock: &pbsubstreams.Clock{Id: "a", Number: 1}},
	}}

	testCases := []struct {
		name string
		// steps are what the fake tier2 does, in order, before it ends the job.
		steps          []tier2Step
		silenceTimeout time.Duration
		// consumerDelay is how long tier1 takes to hand a block to its own consumer.
		consumerDelay time.Duration
		expectSilent  bool
	}{
		{
			name:           "tier2 never answers",
			steps:          []tier2Step{{hang: true}},
			silenceTimeout: timeout,
			expectSilent:   true,
		},
		{
			name:           "tier2 stops answering after a few updates",
			steps:          []tier2Step{{msg: update}, {msg: update}, {hang: true}},
			silenceTimeout: timeout,
			expectSilent:   true,
		},
		{
			name: "tier2 keeps reporting for longer than the timeout",
			steps: []tier2Step{
				{delay: timeout / 2, msg: update},
				{delay: timeout / 2, msg: update},
				{delay: timeout / 2, msg: update},
				{delay: timeout / 2, msg: update},
			},
			silenceTimeout: timeout,
		},
		{
			name:           "consumer takes longer than the timeout to read a block",
			steps:          []tier2Step{{msg: block}, {msg: update}},
			silenceTimeout: timeout,
			consumerDelay:  2 * timeout,
		},
		{
			name:           "no timeout waits for a slow tier2",
			steps:          []tier2Step{{delay: 2 * timeout, msg: update}},
			silenceTimeout: 0,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			w := newSteppedWorker(tc.silenceTimeout, tc.steps)
			upstream := response.New(func(substreams.ResponseFromAnyTier) error {
				time.Sleep(tc.consumerDelay)
				return nil
			})

			res := runWorkWithDeadline(t, w, testWorkerCtx(), upstream)

			if !tc.expectSilent {
				require.NoError(t, res.Error)
				return
			}
			require.ErrorIs(t, res.Error, ErrSilentWorker)
			_, retryable := res.Error.(*RetryableErr)
			assert.True(t, retryable, "a silent tier2 must be retried, got %T", res.Error)
		})
	}
}

// TestWork_SilenceTimeoutLeavesCanceledRequestAlone validates that a request canceled by its
// own consumer while the tier2 is quiet ends as a cancellation, not as a silent tier2.
func TestWork_SilenceTimeoutLeavesCanceledRequestAlone(t *testing.T) {
	w := newSteppedWorker(time.Minute, []tier2Step{{hang: true}})
	ctx, cancel := context.WithCancel(testWorkerCtx())
	time.AfterFunc(50*time.Millisecond, cancel)

	res := runWorkWithDeadline(t, w, ctx, response.New(func(substreams.ResponseFromAnyTier) error { return nil }))

	assert.NotErrorIs(t, res.Error, ErrSilentWorker)
	_, retryable := res.Error.(*RetryableErr)
	assert.False(t, retryable, "a canceled request must not be retried")
}

// TestWork_SilentTier2IsSentAgain validates that the job whose tier2 went silent is sent to
// a tier2 again and completes there.
func TestWork_SilentTier2IsSentAgain(t *testing.T) {
	prevFailed := workerFailedRetryDelay
	workerFailedRetryDelay = 10 * time.Millisecond
	defer func() { workerFailedRetryDelay = prevFailed }()

	fake := &steppedSubstreamsClient{scripts: [][]tier2Step{{{hang: true}}, {}}}
	w := NewRemoteWorker(fake.factory(), "test-worker", zap.NewNop(), NewLaunchQueue(1), 100*time.Millisecond)

	done := make(chan loop.Msg, 1)
	go func() {
		cmd := w.Work(testWorkerCtx(), stage.Unit{Stage: 0, Segment: 0}, 0, []string{"m"}, response.New(func(substreams.ResponseFromAnyTier) error { return nil }), false)
		done <- cmd()
	}()

	select {
	case msg := <-done:
		_, ok := msg.(MsgJobSucceeded)
		assert.True(t, ok, "expected MsgJobSucceeded, got %T (%v)", msg, msg)
		assert.Equal(t, 2, fake.calls)
	case <-time.After(5 * time.Second):
		t.Fatal("job never completed")
	}
}

// tier2Step is one thing a fake tier2 does on a job stream: wait for delay, then send msg,
// or hang until tier1 gives up on the stream.
type tier2Step struct {
	delay time.Duration
	msg   *pbssinternal.ProcessRangeResponse
	hang  bool
}

// steppedProcessRangeStream plays its steps, then ends the job with io.EOF. Like a real
// gRPC stream, it fails with the context's error once tier1 cancels it.
type steppedProcessRangeStream struct {
	ctx   context.Context
	steps []tier2Step
}

func (s *steppedProcessRangeStream) Recv() (*pbssinternal.ProcessRangeResponse, error) {
	if len(s.steps) == 0 {
		return nil, io.EOF
	}
	step := s.steps[0]
	s.steps = s.steps[1:]

	if step.hang {
		<-s.ctx.Done()
		return nil, s.ctx.Err()
	}

	select {
	case <-s.ctx.Done():
		return nil, s.ctx.Err()
	case <-time.After(step.delay):
		return step.msg, nil
	}
}
func (s *steppedProcessRangeStream) Header() (metadata.MD, error) { return metadata.MD{}, nil }
func (s *steppedProcessRangeStream) Trailer() metadata.MD         { return nil }
func (s *steppedProcessRangeStream) CloseSend() error             { return nil }
func (s *steppedProcessRangeStream) Context() context.Context     { return s.ctx }
func (s *steppedProcessRangeStream) SendMsg(any) error            { return nil }
func (s *steppedProcessRangeStream) RecvMsg(any) error            { return nil }

// steppedSubstreamsClient gives each ProcessRange call the next script. Once the scripts
// run out, every job ends right away with io.EOF.
type steppedSubstreamsClient struct {
	scripts [][]tier2Step
	calls   int
}

func (c *steppedSubstreamsClient) ProcessRange(ctx context.Context, in *pbssinternal.ProcessRangeRequest, opts ...grpc.CallOption) (grpc.ServerStreamingClient[pbssinternal.ProcessRangeResponse], error) {
	var steps []tier2Step
	if c.calls < len(c.scripts) {
		steps = c.scripts[c.calls]
	}
	c.calls++
	return &steppedProcessRangeStream{ctx: ctx, steps: steps}, nil
}

func (c *steppedSubstreamsClient) factory() client.InternalClientFactory {
	return func() (pbssinternal.SubstreamsClient, func() error, []grpc.CallOption, client.Headers, error) {
		return c, func() error { return nil }, nil, client.Headers{}, nil
	}
}

func newSteppedWorker(silenceTimeout time.Duration, steps []tier2Step) *RemoteWorker {
	fake := &steppedSubstreamsClient{scripts: [][]tier2Step{steps}}
	return NewRemoteWorker(fake.factory(), "test-worker", zap.NewNop(), NewLaunchQueue(1), silenceTimeout)
}

// runWorkWithDeadline runs a single work() attempt and fails the test if it does not return.
func runWorkWithDeadline(t *testing.T, w *RemoteWorker, ctx context.Context, upstream *response.Stream) *Result {
	t.Helper()

	jobIdx := reqctx.ReqStats(ctx).RecordNewSubrequest(0, 0, 10)
	done := make(chan *Result, 1)
	go func() {
		done <- w.work(ctx, &pbssinternal.ProcessRangeRequest{}, nil, upstream, jobIdx, func() {})
	}()

	select {
	case res := <-done:
		if res.Error != nil && errors.Is(res.Error, context.DeadlineExceeded) {
			t.Fatalf("unexpected deadline: %v", res.Error)
		}
		return res
	case <-time.After(5 * time.Second):
		t.Fatal("work never returned")
		return nil
	}
}

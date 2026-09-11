package stage

import (
	"context"
	"fmt"
	"testing"

	"github.com/abourget/llerrgroup"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/streamingfast/substreams/block"
	"github.com/streamingfast/substreams/reqctx"
	"github.com/streamingfast/substreams/squash"
	"github.com/streamingfast/substreams/storage/store"
)

func TestShouldFallbackToLocal(t *testing.T) {
	t.Parallel()

	live := context.Background()
	canceled, cancel := context.WithCancel(context.Background())
	cancel()

	timedOut, cancelTimeout := context.WithTimeout(context.Background(), 0)
	t.Cleanup(cancelTimeout)
	<-timedOut.Done()

	tests := []struct {
		name string
		ctx  context.Context
		err  error
		want bool
	}{
		{name: "nil error", ctx: live, err: nil, want: false},
		{name: "unavailable", ctx: live, err: status.Error(codes.Unavailable, "connection refused"), want: true},
		{name: "deadline exceeded grpc", ctx: live, err: status.Error(codes.DeadlineExceeded, "timeout"), want: true},
		{
			name: "wrapped unavailable",
			ctx:  live,
			err:  fmt.Errorf("remote squash foo [0-10]: %w", status.Error(codes.Unavailable, "no connection")),
			want: true,
		},
		{name: "invalid argument", ctx: live, err: status.Error(codes.InvalidArgument, "bad module"), want: false},
		{name: "failed precondition", ctx: live, err: status.Error(codes.FailedPrecondition, "missing partial"), want: false},
		{name: "not found", ctx: live, err: status.Error(codes.NotFound, "missing store"), want: false},
		{name: "internal", ctx: live, err: status.Error(codes.Internal, "merge panicked"), want: false},
		{name: "net timeout", ctx: live, err: timeoutNetErr{}, want: true},
		{name: "context deadline on error", ctx: live, err: context.DeadlineExceeded, want: true},
		{name: "request canceled", ctx: canceled, err: status.Error(codes.Unavailable, "connection refused"), want: false},
		{name: "request timed out", ctx: timedOut, err: status.Error(codes.Unavailable, "connection refused"), want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, test.want, shouldFallbackToLocal(test.ctx, test.err))
		})
	}
}

type timeoutNetErr struct{}

func (timeoutNetErr) Error() string   { return "i/o timeout" }
func (timeoutNetErr) Timeout() bool   { return true }
func (timeoutNetErr) Temporary() bool { return true }

type stubSquashClient struct {
	err      error
	calls    int
	requests []squash.Request
}

func (c *stubSquashClient) Squash(_ context.Context, req squash.Request) (*squash.Result, error) {
	c.calls++
	c.requests = append(c.requests, req)
	if c.err != nil {
		return nil, c.err
	}
	return &squash.Result{}, nil
}

func TestRemoteSquashRunSendsAllRangesInOneCall(t *testing.T) {
	t.Parallel()

	client := &stubSquashClient{}
	stages, stage := remoteSquashFixture(client)

	run := []Unit{{Segment: 0}, {Segment: 1}, {Segment: 2}}
	err := stages.remoteSquashRun(stage, run)
	require.NoError(t, err)
	require.Equal(t, 1, client.calls)
	assert.Equal(t, []squash.Range{
		{StartBlock: 0, ExclusiveEndBlock: 10},
		{StartBlock: 10, ExclusiveEndBlock: 20},
		{StartBlock: 20, ExclusiveEndBlock: 30},
	}, client.requests[0].Ranges)
}

func TestSquashStageRemoteSuccessSkipsLocal(t *testing.T) {
	t.Parallel()

	client := &stubSquashClient{}
	stages, stage := remoteSquashFixture(client)

	merged, unmerged, err := stages.squashStage(stage, []Unit{{Segment: 0}, {Segment: 1}})
	require.NoError(t, err)
	assert.Equal(t, []Unit{{Segment: 0}, {Segment: 1}}, merged)
	assert.Empty(t, unmerged)
	assert.Equal(t, 1, client.calls)
}

func TestSquashStageRemoteApplicationErrorDoesNotFallback(t *testing.T) {
	t.Parallel()

	client := &stubSquashClient{err: status.Error(codes.InvalidArgument, "bad module")}
	stages, stage := remoteSquashFixture(client)

	_, _, err := stages.squashStage(stage, []Unit{{Segment: 0}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "remote squash")
	assert.Equal(t, 1, client.calls)
}

func remoteSquashFixture(client squash.Client) (*Stages, *Stage) {
	segmenter := block.NewSegmenter(10, 0, 100)
	ctx := reqctx.WithRemoteSquasher(context.Background(), &reqctx.RemoteSquasher{Client: client})
	modState := &StoreModuleState{
		name:        "mod",
		segmenter:   segmenter,
		storeConfig: &store.Config{},
	}
	stage := &Stage{
		kind:              KindStore,
		segmenter:         segmenter,
		storeModuleStates: []*StoreModuleState{modState},
		syncWork:          llerrgroup.New(10),
	}
	return &Stages{
		ctx:    ctx,
		logger: zap.NewNop(),
	}, stage
}

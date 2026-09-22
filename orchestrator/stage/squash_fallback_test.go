package stage

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/abourget/llerrgroup"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/streamingfast/dstore"
	"github.com/streamingfast/substreams/block"
	"github.com/streamingfast/substreams/metrics"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
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

func TestRemoteFailureDoesNotPoisonLocalMergeGroup(t *testing.T) {
	t.Parallel()

	client := &stubSquashClient{err: status.Error(codes.Unavailable, "connection refused")}
	stages, stage := remoteSquashFixture(client)

	err := stages.remoteSquashRun(stage, []Unit{{Segment: 0}})
	require.Error(t, err)
	require.NoError(t, stage.syncWork.Wait())
}

func TestRemoteSquashDoesNotRetry(t *testing.T) {
	t.Parallel()

	client := &stubSquashClient{err: status.Error(codes.Unavailable, "connection refused")}
	stages, stage := remoteSquashFixture(client)

	err := stages.remoteSquash(stage.storeModuleStates[0], []squash.Range{{StartBlock: 0, ExclusiveEndBlock: 10}})
	require.Error(t, err)
	assert.Equal(t, 1, client.calls)
}

func TestSquashStageApplicationErrorDoesNotStickToLocal(t *testing.T) {
	t.Parallel()

	client := &stubSquashClient{err: status.Error(codes.InvalidArgument, "bad module")}
	stages, stage := remoteSquashFixture(client)
	remote := reqctx.GetRemoteSquasher(stages.ctx)
	remote.Availability = reqctx.NewRemoteAvailability(time.Minute, time.Now)

	_, _, err := stages.squashStage(stage, []Unit{{Segment: 0}})
	require.Error(t, err)
	assert.False(t, remote.PreferLocal())
	assert.Equal(t, 1, client.calls)

	_, _, err = stages.squashStage(stage, []Unit{{Segment: 0}})
	require.Error(t, err)
	assert.Equal(t, 2, client.calls)
	assert.False(t, remote.PreferLocal())
}

func TestSquashStageUnreachableStaysLocalUntilRemoteAnswers(t *testing.T) {
	t.Parallel()

	client := &stubSquashClient{err: status.Error(codes.Unavailable, "connection refused")}
	clock := &manualClock{at: time.Unix(1_000, 0)}
	quiet := 30 * time.Second
	stages, stage := remoteSquashFixture(client)
	remote := reqctx.GetRemoteSquasher(stages.ctx)
	remote.Availability = reqctx.NewRemoteAvailability(quiet, clock.Now)
	prepareLocalFallback(t, stages, stage)

	run := []Unit{{Segment: 0}}
	_, _, err := stages.squashStage(stage, run)
	require.Error(t, err)
	assert.Equal(t, 1, client.calls)
	assert.True(t, remote.PreferLocal())

	_, _, err = stages.squashStage(stage, run)
	require.Error(t, err)
	assert.Equal(t, 1, client.calls, "later runs stay local while the remote is down")

	clock.Advance(quiet)
	client.err = nil
	merged, unmerged, err := stages.squashStage(stage, run)
	require.NoError(t, err)
	assert.Equal(t, run, merged)
	assert.Empty(t, unmerged)
	assert.Equal(t, 2, client.calls)
	assert.False(t, remote.PreferLocal())
}

func TestSquashStageCanceledProbeDoesNotFallBack(t *testing.T) {
	t.Parallel()

	client := &stubSquashClient{err: status.Error(codes.Unavailable, "connection refused")}
	clock := &manualClock{at: time.Unix(1_000, 0)}
	quiet := 30 * time.Second
	gate := reqctx.NewRemoteAvailability(quiet, clock.Now)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx = reqctx.WithRemoteSquasher(ctx, &reqctx.RemoteSquasher{Client: client, Availability: gate})

	stages, stage := remoteSquashFixture(client)
	stages.ctx = ctx
	gate.MarkDown()
	clock.Advance(quiet)
	cancel()

	_, _, err := stages.squashStage(stage, []Unit{{Segment: 0}})
	require.Error(t, err)
	assert.Equal(t, 1, client.calls)
	assert.True(t, gate.PreferLocal(), "a canceled probe keeps the quiet period and does not squash locally")
}

func prepareLocalFallback(t *testing.T, stages *Stages, stage *Stage) {
	t.Helper()
	obj, err := dstore.NewStore("memory://squash", "", "", false)
	require.NoError(t, err)
	cfg, err := store.NewConfig(
		"mod",
		0,
		"hash",
		pbsubstreams.Module_KindStore_UPDATE_POLICY_SET,
		"string",
		obj,
		nil,
		0,
		t.TempDir(),
		"memory",
	)
	require.NoError(t, err)
	modState := stage.storeModuleStates[0]
	modState.storeConfig = cfg
	modState.logger = zap.NewNop()
	stages.ctx = reqctx.WithReqStats(stages.ctx, metrics.NewReqStats(&metrics.Config{}, nil, nil, zap.NewNop()))
}

type manualClock struct {
	mu sync.Mutex
	at time.Time
}

func (c *manualClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.at
}

func (c *manualClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.at = c.at.Add(d)
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

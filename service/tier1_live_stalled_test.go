package service

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/streamingfast/shutter"
	"github.com/streamingfast/substreams/metrics"
	pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/atomic"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestTier1Service_RefusesLiveRequestsWhenLiveStalled(t *testing.T) {
	metrics.DeclareTier1Metrics(zap.NewNop())
	s := &Tier1Service{
		Shutter:      shutter.New(),
		logger:       zap.NewNop(),
		liveStalled:  func() bool { return true },
		getHeadBlock: func() (uint64, error) { return 1000, nil },
	}

	_, err := s.BlocksAny(context.Background(), &pbsubstreamsrpc.Request{StartBlockNum: 900}, nil, "grpc", nil, nil, false, zap.NewNop())
	require.Error(t, err)
	assert.Equal(t, connect.CodeUnavailable, connect.CodeOf(err))
	assert.ErrorIs(t, err, errLiveSourceStalled)
}

func TestTier1Service_NeedsLiveBlocks(t *testing.T) {
	tests := []struct {
		name       string
		stopBlock  uint64
		finalsOnly bool
		headErr    error
		expected   bool
	}{
		{"no stop block", 0, false, nil, true},
		{"stop below head", 500, false, nil, false},
		{"stop at head", 1000, false, nil, false},
		{"stop past head", 1001, false, nil, true},
		{"head unknown", 500, false, errors.New("not ready"), true},
		{"final blocks only, stop below final head", 970, true, nil, false},
		{"final blocks only, stop between final head and head", 990, true, nil, true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			s := &Tier1Service{
				getHeadBlock:        func() (uint64, error) { return 1000, test.headErr },
				getRecentFinalBlock: func() (uint64, error) { return 980, test.headErr },
			}
			assert.Equal(t, test.expected, s.needsLiveBlocks(&pbsubstreamsrpc.Request{StopBlockNum: test.stopBlock, FinalBlocksOnly: test.finalsOnly}))
		})
	}
}

func TestTier1Service_CancelWhenLiveStalls(t *testing.T) {
	defer func(previous time.Duration) { liveStalledCheckInterval = previous }(liveStalledCheckInterval)
	liveStalledCheckInterval = 10 * time.Millisecond

	stalled := atomic.NewBool(false)
	s := &Tier1Service{liveStalled: stalled.Load}

	ctx, cancel := context.WithCancelCause(context.Background())
	defer cancel(nil)
	go s.cancelWhenLiveStalls(ctx, cancel)

	time.Sleep(50 * time.Millisecond)
	require.NoError(t, ctx.Err(), "a live source that has not stalled must not cancel the request")

	stalled.Store(true)
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("request not canceled once the live source stalled")
	}
	assert.ErrorIs(t, context.Cause(ctx), errLiveSourceStalled)

	// Whatever error the cancellation surfaces as, the client is told to reconnect: cursor
	// resolution, for one, turns it into an InvalidArgument a sink would not retry.
	for _, streamErr := range []error{
		fmt.Errorf("running pipeline: %w", context.Canceled),
		connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("resolving cursor: %s", context.Canceled)),
		status.Error(codes.Canceled, "registry lookup canceled"),
	} {
		assert.Equal(t, codes.Unavailable, status.Code(toGrpcTier1Error(ctx, streamErr)), streamErr.Error())
		assert.Equal(t, connect.CodeUnavailable, connect.CodeOf(toConnectError(ctx, streamErr)), streamErr.Error())
	}
}

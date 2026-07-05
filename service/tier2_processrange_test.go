package service

import (
	"context"
	"testing"

	"github.com/streamingfast/substreams/metrics"
	pbssinternal "github.com/streamingfast/substreams/pb/sf/substreams/intern/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type fakeProcessRangeServer struct {
	ctx context.Context
}

func (s *fakeProcessRangeServer) Send(*pbssinternal.ProcessRangeResponse) error { return nil }
func (s *fakeProcessRangeServer) SetHeader(metadata.MD) error                   { return nil }
func (s *fakeProcessRangeServer) SendHeader(metadata.MD) error                  { return nil }
func (s *fakeProcessRangeServer) SetTrailer(metadata.MD)                        {}
func (s *fakeProcessRangeServer) Context() context.Context                      { return s.ctx }
func (s *fakeProcessRangeServer) SendMsg(any) error                             { return nil }
func (s *fakeProcessRangeServer) RecvMsg(any) error                             { return nil }

// TestProcessRange_NilModules ensures that a malformed ProcessRangeRequest
// with no Modules is refused with InvalidArgument instead of panicking on a
// nil pointer dereference when building the module names list.
func TestProcessRange_NilModules(t *testing.T) {
	metrics.DeclareTier2Metrics(zap.NewNop()) // tier2 metrics are lazily declared by the app, ProcessRange needs them

	svc, err := NewTier2(
		context.Background(),
		zap.NewNop(),
		func() bool { return false },
		WithReadinessFunc(func(bool) {}),
	)
	require.NoError(t, err)

	streamSrv := &fakeProcessRangeServer{ctx: context.Background()}

	err = svc.ProcessRange(&pbssinternal.ProcessRangeRequest{Modules: nil}, streamSrv)
	require.Error(t, err)

	st, ok := status.FromError(err)
	require.True(t, ok, "expected a gRPC status error, got: %v", err)
	assert.Equal(t, codes.InvalidArgument, st.Code())
}

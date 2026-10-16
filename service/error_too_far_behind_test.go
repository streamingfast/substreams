package service

import (
	"context"
	"fmt"
	"testing"

	"connectrpc.com/connect"
	"github.com/streamingfast/substreams/pipeline"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// A request that trails the last final block is disconnected so that it back-processes the gap
// on reconnection. It must be Unavailable so clients reconnect, and must say why instead of
// reporting a server shutdown.
func TestTooFarBehindIsUnavailable(t *testing.T) {
	ctx := context.Background()

	cases := []struct {
		name string
		err  error
		want error
	}{
		{"back-processing ended too far behind", connect.NewError(connect.CodeUnavailable, pipeline.ErrBackprocessEndedTooFarBehind), pipeline.ErrBackprocessEndedTooFarBehind},
		{"stream fell too far behind", pipeline.ErrStreamFellTooFarBehind, pipeline.ErrStreamFellTooFarBehind},
		{"stream fell too far behind, wrapped", fmt.Errorf("process block: %w", pipeline.ErrStreamFellTooFarBehind), pipeline.ErrStreamFellTooFarBehind},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			connectErr := toConnectError(ctx, tt.err)
			require.Error(t, connectErr)
			assert.Equal(t, connect.CodeUnavailable, connect.CodeOf(connectErr))
			assert.ErrorContains(t, connectErr, tt.want.Error())
			assert.NotContains(t, connectErr.Error(), "shutting down")

			grpcErr := toGrpcTier1Error(ctx, tt.err)
			require.Error(t, grpcErr)
			assert.Equal(t, codes.Unavailable, status.Code(grpcErr))
			assert.Contains(t, status.Convert(grpcErr).Message(), tt.want.Error())
			assert.NotContains(t, grpcErr.Error(), "shutting down")
		})
	}
}

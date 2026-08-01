package service

import (
	"context"
	"fmt"
	"testing"

	"connectrpc.com/connect"
	"github.com/streamingfast/substreams/pipeline"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestToConnectError_ShuttingDownIsUnavailable documents how a tier1 shutdown
// reaches the client while a parallel backprocess is in flight.
//
// When the tier1 service starts terminating, the scheduler quits with
// `errShuttingDown`, and that error is wrapped four times as it unwinds:
//
//	scheduler run -> parallel processing run -> run_parallel_process failed ->
//	error during init_stores_and_backprocess
//
// It must still be reported as `CodeUnavailable` so that sinks treat it as a
// "reconnect me" signal rather than a server-side fault.
func TestToConnectError_ShuttingDownIsUnavailable(t *testing.T) {
	ctx := context.Background()

	cases := []struct {
		name string
		err  error
	}{
		{"unwrapped", errShuttingDown},
		{
			"wrapped by the backprocess call chain",
			fmt.Errorf("error during init_stores_and_backprocess: %w",
				fmt.Errorf("run_parallel_process failed: %w",
					fmt.Errorf("parallel processing run: %w",
						fmt.Errorf("scheduler run: %w", errShuttingDown)))),
		},
		{"quicksave on shutdown, unwrapped", pipeline.ErrShuttingDown},
		{
			"quicksave on shutdown, wrapped",
			fmt.Errorf("stream terminated: %w", pipeline.ErrShuttingDown),
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			connectErr := toConnectError(ctx, tt.err)
			require.Error(t, connectErr)

			assert.Equal(t, connect.CodeUnavailable, connect.CodeOf(connectErr),
				"shutdown errors must be Unavailable so clients reconnect, got: %s", connectErr)
		})
	}
}

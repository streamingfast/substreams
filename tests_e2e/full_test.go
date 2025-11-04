package tests_e2e

import (
	"context"
	"os"
	"testing"

	"github.com/streamingfast/logging"
	"github.com/streamingfast/substreams/manifest"
	pbsubstreamsrpcv2 "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDummyBlockchainContainer(t *testing.T) {
	ctx := context.Background()
	zlog := logging.MustCreateLoggerWithServiceName("dummy-blockchain-test")

	// Create temporary directory for volume mount
	tmpDir, err := os.MkdirTemp("", "firehose-data-")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// launch dummy blockchain container
	container, err := newDummyBlockchainContainer(ctx, tmpDir)
	require.NoError(t, err)
	defer container.Terminate(ctx)

	_, substreamsEndpoint := startTier1App(t, ctx, tmpDir, container, zlog)
	_ = startTier2App(t, ctx, tmpDir, zlog)

	testCases := []struct {
		name           string
		startBlock     int64
		stopBlock      uint64
		productionMode bool
		spkgFile       string
		outputModule   string
		expectedLen    int
		expectedClock  uint64
	}{
		{
			name:           "simple events dev",
			startBlock:     100,
			stopBlock:      160,
			productionMode: false,
			spkgFile:       "./dummy/e2e-v0.1.0.spkg",
			outputModule:   "map_events",
			expectedLen:    60,
			expectedClock:  159,
		},
		{
			name:           "stats that depend on store (dev)",
			startBlock:     150,
			stopBlock:      210,
			productionMode: false,
			spkgFile:       "./dummy/e2e-v0.1.0.spkg",
			outputModule:   "map_stats",
			expectedLen:    60,
			expectedClock:  209,
		},
		{
			name:           "stats that depend on store (prod)",
			startBlock:     150,
			stopBlock:      500,
			productionMode: true,
			spkgFile:       "./dummy/e2e-v0.1.0.spkg",
			outputModule:   "map_stats",
			expectedLen:    350,
			expectedClock:  499,
		},
		{
			name:           "stats that depend on double store (prod)",
			startBlock:     150,
			stopBlock:      500,
			productionMode: true,
			spkgFile:       "./dummy/e2e-v0.1.0.spkg",
			outputModule:   "map_stats2",
			expectedLen:    350,
			expectedClock:  499,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			pkg, err := manifest.MustNewReader(tc.spkgFile).Read()
			require.NoError(t, err)

			request := &pbsubstreamsrpcv2.Request{
				StartBlockNum:   tc.startBlock,
				StopBlockNum:    tc.stopBlock,
				FinalBlocksOnly: false,
				ProductionMode:  tc.productionMode,
				OutputModule:    tc.outputModule,
				Modules:         pkg.Package.Modules,
			}

			// Run the request
			blockScopedDataSlice, session, err := RunRequest(t, request, substreamsEndpoint)
			if err != nil {
				// Let's try to get container logs to debug
				logs, logErr := container.Logs(ctx)
				if logErr == nil {
					defer logs.Close()
					buf := make([]byte, 4096)
					n, _ := logs.Read(buf)
					t.Logf("Container logs: %s", string(buf[:n]))
				}

				t.Errorf("Error running request: %v", err)
			}

			require.NotNil(t, session, "Should have received at least one session")
			assert.Equal(t, tc.expectedLen, len(blockScopedDataSlice), "Should have received all expected blocks")
			assert.Equal(t, tc.expectedClock, uint64(blockScopedDataSlice[len(blockScopedDataSlice)-1].Clock.Number), "Should end on expected block")
		})
	}

}

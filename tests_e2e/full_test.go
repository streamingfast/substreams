package tests_e2e

import (
	"context"
	"io"
	"os"
	"testing"

	"github.com/streamingfast/logging"
	"github.com/streamingfast/substreams/manifest"
	pbsubstreamsrpcv3 "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"go.uber.org/zap"
)

var zlog, _ = logging.PackageLogger("tests_e2e", "github.com/streamingfast/substreams/tests_e2e")

func init() {
	logging.InstantiateLoggers(logging.WithDefaultLevel(zap.DPanicLevel))
}

func TestDummyBlockchainContainer(t *testing.T) {
	ctx := context.Background()

	// Create temporary directory for volume mount
	tmpDir, err := os.MkdirTemp("", "firehose-data-")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// launch dummy blockchain container
	container, err := newDummyBlockchainContainer(ctx, tmpDir, latestDummyBlockchainImage, "", 1000)
	require.NoError(t, err)
	defer container.Terminate(ctx, testcontainers.StopTimeout(0))

	zlog.Info("dummy blockchain container started", zap.String("tmp_dir", tmpDir))

	app2, t2Endpoint := startTier2App(t, ctx, tmpDir, zlog)
	app, substreamsEndpoint := startTier1App(t, ctx, tmpDir, container, t2Endpoint, zlog)

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
			name:           "stats that depend on store in future (dev)",
			startBlock:     150,
			stopBlock:      210,
			productionMode: false,
			spkgFile:       "./dummy/e2e-v0.1.0.spkg",
			outputModule:   "map_stats",
			expectedLen:    60,
			expectedClock:  209,
		},
		{
			name:           "stats further away that depend on store (dev)",
			startBlock:     500,
			stopBlock:      510,
			productionMode: false,
			spkgFile:       "./dummy/e2e-v0.1.0.spkg",
			outputModule:   "map_stats",
			expectedLen:    10,
			expectedClock:  509,
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
		{
			name:           "map that depend on store in future (prod)",
			startBlock:     0,
			stopBlock:      400,
			productionMode: true,
			spkgFile:       "./dummy/e2e-v0.1.0.spkg",
			outputModule:   "map_stats",
			expectedLen:    399,
			expectedClock:  399,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			pkg, err := manifest.MustNewReader(tc.spkgFile).Read()
			require.NoError(t, err)

			request := &pbsubstreamsrpcv3.Request{
				StartBlockNum:   tc.startBlock,
				StopBlockNum:    tc.stopBlock,
				FinalBlocksOnly: false,
				ProductionMode:  tc.productionMode,
				OutputModule:    tc.outputModule,
				Package:         pkg.Package,
			}

			// Run the request
			blockScopedDataSlice, session, err := RunRequest(t, request, substreamsEndpoint)
			if err != nil && err != io.EOF {
				// Let's try to get container logs to debug
				logs, logErr := container.Logs(ctx)
				if logErr == nil {
					defer logs.Close()
					buf := make([]byte, 4096)
					n, _ := logs.Read(buf)
					t.Logf("Container logs: %s", string(buf[:n]))
				}
				t.Fatalf("Unexpected error: %s", err.Error())
			}

			require.NotNil(t, session, "Should have received at least one session")
			assert.Equal(t, tc.expectedLen, len(blockScopedDataSlice), "Should have received all expected blocks")
			require.Greater(t, len(blockScopedDataSlice), 0)
			assert.Equal(t, tc.expectedClock, uint64(blockScopedDataSlice[len(blockScopedDataSlice)-1].Clock.Number), "Should end on expected block")
		})
	}

	// ensure we close this well for next tests
	app.Shutdown(nil)
	app2.Shutdown(nil)
	<-app.Terminated()
	<-app2.Terminated()
}

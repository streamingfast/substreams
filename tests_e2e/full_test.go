package tests_e2e

import (
	"context"
	"io"
	"testing"

	"github.com/streamingfast/logging"
	"github.com/streamingfast/substreams/manifest"
	pbsubstreamsrpcv3 "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

var zlog, _ = logging.PackageLogger("tests_e2e", "github.com/streamingfast/substreams/tests_e2e")

func init() {
	logging.InstantiateLoggers(logging.WithDefaultLevel(zap.DPanicLevel))
}

func TestDummyBlockchainContainer(t *testing.T) {
	ctx := context.Background()

	// Create temporary directory for volume mount
	tmpDir := t.TempDir()

	// launch dummy blockchain container
	container, err := newDummyBlockchainContainer(t, ctx, tmpDir, latestDummyBlockchainImage, "", 1000)
	require.NoError(t, err)

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

// TestLiveBackfillerWithTier2SecretAuth validates that the live backfiller
// forwards the subrequest secret key when calling tier2.
// Tier2 uses a secret:// authenticator and will reject unauthenticated requests.
func TestLiveBackfillerWithTier2SecretAuth(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	// Burst 500 blocks so merged-block files are ready for historical segments.
	// StopBlockNum=550 puts the stop past the burst LIB so the live source
	// engages and the backfiller fires at least one authenticated job to tier2.
	// LiveBackFillerFinalBlockDelay=2 (set on Tier1Config) makes the backfiller
	// fire quickly instead of waiting for the default 120-block delay.
	container, err := newDummyBlockchainContainerWithBlockRate(t, ctx, tmpDir, latestDummyBlockchainImage, "", 500, 480)
	require.NoError(t, err)

	const secret = "super-secret-key-e2e"

	app2, t2Endpoint := startTier2AppWithSecret(t, ctx, tmpDir, secret, zlog)
	app1, substreamsEndpoint := startTier1AppWithSecret(t, ctx, tmpDir, container, t2Endpoint, secret, zlog)
	defer func() {
		app1.Shutdown(nil)
		app2.Shutdown(nil)
		<-app1.Terminated()
		<-app2.Terminated()
	}()

	pkg, err := manifest.MustNewReader("./dummy/e2e-v0.1.0.spkg").Read()
	require.NoError(t, err)

	// Production mode with a store module ensures tier2 back-processes
	// historical segments, then the live backfiller handles the remainder.
	request := &pbsubstreamsrpcv3.Request{
		StartBlockNum:   0,
		StopBlockNum:    550,
		FinalBlocksOnly: false,
		ProductionMode:  true,
		OutputModule:    "map_stats",
		Package:         pkg.Package,
	}

	blockScopedDataSlice, session, err := RunRequest(t, request, substreamsEndpoint)
	if err != nil && err != io.EOF {
		logs, logErr := container.Logs(ctx)
		if logErr == nil {
			defer logs.Close()
			buf := make([]byte, 4096)
			n, _ := logs.Read(buf)
			t.Logf("Container logs: %s", string(buf[:n]))
		}
		t.Fatalf("Unexpected error: %s", err.Error())
	}

	require.NotNil(t, session, "should have received at least one session init message")
	assert.Equal(t, 549, len(blockScopedDataSlice), "should have received all expected blocks")
	require.Greater(t, len(blockScopedDataSlice), 0)
	assert.Equal(t, uint64(549), uint64(blockScopedDataSlice[len(blockScopedDataSlice)-1].Clock.Number), "should end on block 549")
}

// TestErrNoInputTypeUrlNotEmpty replicates the issue where a module that has no input
// for a block (ErrNoInput) produced a BlockScopedData with an empty anypb.Any payload
// (no TypeUrl, no Value). The fix ensures TypeUrl is always set.
//
// The scenario uses two modules:
//   - map_filtered: takes map_events as input, calls skip_empty_output() for even blocks
//   - map_downstream: takes ONLY map_filtered as input (no block source)
//
// When map_filtered skips its output for an even block, map_downstream's only input
// is absent from the cache and it hits the ErrNoInput code path. Before the fix,
// this produced an empty anypb.Any{}. After the fix, TypeUrl is always set.
func TestErrNoInputTypeUrlNotEmpty(t *testing.T) {
	ctx := context.Background()

	tmpDir := t.TempDir()

	container, err := newDummyBlockchainContainer(t, ctx, tmpDir, latestDummyBlockchainImage, "", 1000)
	require.NoError(t, err)

	app2, t2Endpoint := startTier2App(t, ctx, tmpDir, zlog)
	app, substreamsEndpoint := startTier1App(t, ctx, tmpDir, container, t2Endpoint, zlog)
	defer func() {
		app.Shutdown(nil)
		app2.Shutdown(nil)
		<-app.Terminated()
		<-app2.Terminated()
	}()

	pkg, err := manifest.MustNewReader("./dummy/e2e-v0.2.0.spkg").Read()
	require.NoError(t, err)

	// Stream map_downstream over 40 blocks in dev mode.
	// Roughly half of these blocks will have map_filtered skip its output (even block numbers),
	// causing map_downstream to hit the ErrNoInput path.
	request := &pbsubstreamsrpcv3.Request{
		StartBlockNum:   100,
		StopBlockNum:    140,
		FinalBlocksOnly: false,
		ProductionMode:  false,
		OutputModule:    "map_downstream",
		Package:         pkg.Package,
	}

	blockScopedDataSlice, session, err := RunRequest(t, request, substreamsEndpoint)
	if err != nil && err != io.EOF {
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
	require.Equal(t, 40, len(blockScopedDataSlice), "Should have received all 40 blocks")

	var blocksWithData, blocksWithEmptyOutput int
	for _, bsd := range blockScopedDataSlice {
		output := bsd.GetOutput()
		require.NotNil(t, output, "output field must not be nil for block %d", bsd.Clock.Number)

		mapOutput := output.GetMapOutput()
		require.NotNil(t, mapOutput, "MapOutput must not be nil for block %d", bsd.Clock.Number)

		// This is the key assertion: TypeUrl must always be set, never empty.
		// Before the fix, blocks where map_downstream hit ErrNoInput would produce
		// an empty anypb.Any{} with no TypeUrl.
		assert.NotEmpty(t, mapOutput.TypeUrl,
			"MapOutput.TypeUrl must be non-empty for block %d (ErrNoInput fix validation)", bsd.Clock.Number)

		if len(mapOutput.Value) > 0 {
			blocksWithData++
		} else {
			blocksWithEmptyOutput++
		}
	}

	// Sanity check: roughly half of blocks should have empty output (even blocks filtered out),
	// and the other half should have data (odd blocks passed through).
	t.Logf("Blocks with data: %d, blocks with empty output (ErrNoInput path): %d", blocksWithData, blocksWithEmptyOutput)
	assert.Greater(t, blocksWithEmptyOutput, 0, "At least some blocks should have triggered the ErrNoInput path")
	assert.Greater(t, blocksWithData, 0, "At least some blocks should have data")
}

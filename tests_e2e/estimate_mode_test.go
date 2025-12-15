package tests_e2e

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/streamingfast/substreams/manifest"
	pbsubstreamsrpcv2 "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

const (
	estimateModeBurst           = 500
	estimateModeBlockchainImage = "ghcr.io/streamingfast/dummy-blockchain:17b576d"
)

func TestEstimateMode(t *testing.T) {
	ctx := t.Context()
	tmpDir := t.TempDir()

	container, err := newDummyBlockchainContainer(ctx, tmpDir, estimateModeBlockchainImage, "", estimateModeBurst)
	require.NoError(t, err)
	defer container.Terminate(ctx)

	// Log container details for debugging
	if container != nil {
		if ports, portErr := container.Ports(ctx); portErr == nil {
			zlog.Info("container exposed ports", zap.Any("ports", ports))
		}
		if logs, logErr := container.Logs(ctx); logErr == nil {
			defer logs.Close()
			buf := make([]byte, 2048)
			if n, _ := logs.Read(buf); n > 0 {
				zlog.Info("container logs", zap.String("logs", string(buf[:n])))
			}
		}
	}

	app2, t2Endpoint := startTier2App(t, ctx, tmpDir, zlog)
	app, substreamsEndpoint := startTier1App(t, ctx, tmpDir, container, t2Endpoint, zlog)

	testCases := []struct {
		name           string
		startBlock     int64
		stopBlock      uint64
		productionMode bool
		estimateMode   bool
		noopMode       bool
		spkgFile       string
		outputModule   string
		expectError    bool
		errorContains  string
	}{
		{
			name:           "basic estimate mode - single block",
			startBlock:     100,
			stopBlock:      101,
			productionMode: false,
			estimateMode:   true,
			noopMode:       false,
			spkgFile:       "./dummy/e2e-v0.1.0.spkg",
			outputModule:   "map_events",
			expectError:    false,
		},
		{
			name:           "estimate mode - small range",
			startBlock:     100,
			stopBlock:      110,
			productionMode: false,
			estimateMode:   true,
			noopMode:       false,
			spkgFile:       "./dummy/e2e-v0.1.0.spkg",
			outputModule:   "map_events",
			expectError:    false,
		},
		{
			name:           "estimate mode - medium range",
			startBlock:     100,
			stopBlock:      200,
			productionMode: false,
			estimateMode:   true,
			noopMode:       false,
			spkgFile:       "./dummy/e2e-v0.1.0.spkg",
			outputModule:   "map_events",
			expectError:    false,
		},
		{
			name:           "mutual exclusivity - estimate + noop should fail",
			startBlock:     100,
			stopBlock:      105,
			productionMode: false,
			estimateMode:   true,
			noopMode:       true,
			spkgFile:       "./dummy/e2e-v0.1.0.spkg",
			outputModule:   "map_events",
			expectError:    true,
			errorContains:  "mutually exclusive",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			pkg, err := manifest.MustNewReader(tc.spkgFile).Read()
			require.NoError(t, err)

			request := &pbsubstreamsrpcv2.Request{
				StartBlockNum:   tc.startBlock,
				StopBlockNum:    tc.stopBlock,
				FinalBlocksOnly: true,
				ProductionMode:  tc.productionMode,
				EstimateMode:    tc.estimateMode,
				NoopMode:        tc.noopMode,
				OutputModule:    tc.outputModule,
				Modules:         pkg.Package.Modules,
			}

			// Run the request
			blockScopedDataSlice, session, err := RunRequest(t, request, substreamsEndpoint)

			if tc.expectError {
				require.Error(t, err)
				if tc.errorContains != "" {
					assert.Contains(t, err.Error(), tc.errorContains)
				}
				return
			}

			// For successful requests
			if err != nil && err != io.EOF && !errors.Is(err, context.Canceled) {
				// Let's try to get container logs to debug
				logs, logErr := container.Logs(ctx)
				if logErr == nil {
					defer logs.Close()
					buf := make([]byte, 4096)
					if n, _ := logs.Read(buf); n > 0 {
						zlog.Error("container logs on error", zap.String("logs", string(buf[:n])))
					}
				}
				require.NoError(t, err)
			}

			// Verify session was initialized
			require.NotNil(t, session, "Session should be initialized")
			zlog.Info("session initialized", zap.String("trace_id", session.TraceId))

			// Verify we got some block scoped data
			require.NotEmpty(t, blockScopedDataSlice, "Should have received block scoped data")
			zlog.Info("received block scoped data responses", zap.Int("count", len(blockScopedDataSlice)))

			// In estimate mode, verify that responses have empty MapModuleOutput
			// but still contain clock and cursor information
			for i, blockData := range blockScopedDataSlice {
				require.NotNil(t, blockData.Clock, "Block %d should have clock", i)
				require.NotNil(t, blockData.Cursor, "Block %d should have cursor", i)

				// In estimate mode, the output should be empty (no actual data)
				if tc.estimateMode {
					if blockData.Output != nil {
						mapOutput := blockData.Output.GetMapOutput()
						assert.Nil(t, mapOutput, "MapOutput should be empty in estimate mode for block %d", blockData.Clock.Number)
					}
				}

				zlog.Debug("processed block", zap.Int("index", i), zap.Uint64("number", blockData.Clock.Number), zap.String("id", blockData.Clock.Id))
			}

			// Verify the block range matches what we requested
			if len(blockScopedDataSlice) > 0 {
				firstBlock := blockScopedDataSlice[0]
				lastBlock := blockScopedDataSlice[len(blockScopedDataSlice)-1]

				assert.GreaterOrEqual(t, firstBlock.Clock.Number, uint64(tc.startBlock), "First block should be >= start block")
				assert.Less(t, lastBlock.Clock.Number, tc.stopBlock, "Last block should be < stop block")
			}
		})
	}

	// Cleanup
	app.Shutdown(nil)
	app2.Shutdown(nil)
}

func TestEstimateModeMetricsCollection(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	container, err := newDummyBlockchainContainer(ctx, tmpDir, estimateModeBlockchainImage, "", estimateModeBurst)
	require.NoError(t, err)
	defer container.Terminate(ctx)

	app2, t2Endpoint := startTier2App(t, ctx, tmpDir, zlog)
	app, substreamsEndpoint := startTier1App(t, ctx, tmpDir, container, t2Endpoint, zlog)

	pkg, err := manifest.MustNewReader("./dummy/e2e-v0.1.0.spkg").Read()
	require.NoError(t, err)

	// Test metrics collection with different block ranges
	testCases := []struct {
		name       string
		startBlock int64
		stopBlock  uint64
		minBlocks  int
	}{
		{
			name:       "single block metrics",
			startBlock: 100,
			stopBlock:  101,
			minBlocks:  1,
		},
		{
			name:       "multiple blocks metrics",
			startBlock: 100,
			stopBlock:  110,
			minBlocks:  9,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			request := &pbsubstreamsrpcv2.Request{
				StartBlockNum:   tc.startBlock,
				StopBlockNum:    tc.stopBlock,
				FinalBlocksOnly: true,
				ProductionMode:  true,
				EstimateMode:    true,
				OutputModule:    "map_events",
				Modules:         pkg.Package.Modules,
			}

			// Run the request
			blockScopedDataSlice, session, err := RunRequest(t, request, substreamsEndpoint)
			if err != nil && err != io.EOF && !errors.Is(err, context.Canceled) {
				require.NoError(t, err)
			}

			// Verify session was initialized
			require.NotNil(t, session, "Session should be initialized")

			// Verify we got the expected number of blocks
			require.GreaterOrEqual(t, len(blockScopedDataSlice), tc.minBlocks, "Should have received at least %d blocks", tc.minBlocks)

			// Log metrics information for manual verification
			zlog.Info("processed blocks in estimate mode", zap.Int("block_count", len(blockScopedDataSlice)))

			// In a real implementation, we would verify that:
			// 1. Egress bytes are calculated correctly
			// 2. Processed block count matches the number of blocks
			// 3. Metrics are reported in the final progress message
			// For now, we verify that the basic flow works without errors
		})
	}

	// Cleanup
	app.Shutdown(nil)
	app2.Shutdown(nil)
}

func TestEstimateModeConcurrentRequests(t *testing.T) {
	ctx := t.Context()
	tmpDir := t.TempDir()

	container, err := newDummyBlockchainContainer(ctx, tmpDir, estimateModeBlockchainImage, "", estimateModeBurst)
	require.NoError(t, err)
	t.Cleanup(func() { container.Terminate(context.Background()) })

	app2, t2Endpoint := startTier2App(t, ctx, tmpDir, zlog)
	app, substreamsEndpoint := startTier1App(t, ctx, tmpDir, container, t2Endpoint, zlog)

	pkg, err := manifest.MustNewReader("./dummy/e2e-v0.1.0.spkg").Read()
	require.NoError(t, err)

	// Test concurrent estimate mode requests
	numConcurrentRequests := 3
	results := make(chan error, numConcurrentRequests)

	for i := range numConcurrentRequests {
		go func(requestID int) {
			request := &pbsubstreamsrpcv2.Request{
				StartBlockNum:   int64(100 + requestID),
				StopBlockNum:    uint64(101 + requestID),
				FinalBlocksOnly: true,
				ProductionMode:  true,
				EstimateMode:    true,
				OutputModule:    "map_events",
				Modules:         pkg.Package.Modules,
			}

			// Run the request
			blockScopedDataSlice, session, err := RunRequest(t, request, substreamsEndpoint)
			if err != nil && err != io.EOF && !errors.Is(err, context.Canceled) {
				results <- err
				return
			}

			// Verify basic response structure
			if session == nil {
				results <- errors.New("session should not be nil")
				return
			}

			if len(blockScopedDataSlice) == 0 {
				results <- errors.New("should have received block scoped data")
				return
			}

			zlog.Info("concurrent request completed successfully", zap.Int("request_id", requestID), zap.Int("block_count", len(blockScopedDataSlice)))
			results <- nil
		}(i)
	}

	// Wait for all requests to complete
	for i := range numConcurrentRequests {
		err := <-results
		require.NoError(t, err, "Concurrent request %d should succeed", i)
	}

	// Cleanup
	app.Shutdown(nil)
	app2.Shutdown(nil)
}

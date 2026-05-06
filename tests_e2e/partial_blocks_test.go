package tests_e2e

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/streamingfast/substreams/manifest"
	pbsubstreamsrpcv2 "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
	pbsubstreamsrpcv3 "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"go.uber.org/zap"
)

const latestDummyBlockchainImage = "ghcr.io/streamingfast/dummy-blockchain:1cea671"

func TestPartialBlocksSimple(t *testing.T) {
	ctx := context.Background()
	// takes zlog from init()
	//zlog := logging.MustCreateLoggerWithServiceName("partial-blocks-test")
	//zlog := zap.NewNop()

	// Create temporary directory for volume mount
	tmpDir := t.TempDir()

	// launch dummy blockchain container with flash blocks enabled
	image := latestDummyBlockchainImage

	burst := 120

	t.Logf("Starting container with image: %s and burst %d", image, burst)
	container, err := newDummyBlockchainContainer(ctx, tmpDir, image, "--with-flash-blocks", burst)
	require.NoError(t, err)
	defer container.Terminate(ctx, testcontainers.StopTimeout(0))

	// Log container details for debugging
	if container != nil {
		if ports, portErr := container.Ports(ctx); portErr == nil {
			t.Logf("Container exposed ports: %v", ports)
		}
		if logs, logErr := container.Logs(ctx); logErr == nil {
			defer logs.Close()
			buf := make([]byte, 2048)
			if n, _ := logs.Read(buf); n > 0 {
				t.Logf("Container logs: %s", string(buf[:n]))
			}
		}
	}

	app2, t2Endpoint := startTier2App(t, ctx, tmpDir, zlog)
	app, substreamsEndpoint := startTier1App(t, ctx, tmpDir, container, t2Endpoint, zlog)

	defer func() {
		container.Terminate(ctx, testcontainers.StopTimeout(0))
		// ensure we close this well, for next tests
		app.Shutdown(nil)
		app2.Shutdown(nil)
		<-app.Terminated()
		<-app2.Terminated()
	}()

	testCases := []struct {
		name           string
		startBlock     int64
		stopBlock      uint64
		productionMode bool
		spkgFile       string
		outputModule   string
	}{
		{
			name:           "simple events head",
			startBlock:     -1,
			stopBlock:      0, // Will halt on first partial block
			productionMode: false,
			spkgFile:       "./dummy/e2e-v0.1.0.spkg",
			outputModule:   "map_events",
		},
		{
			name:           "simple events prod",
			startBlock:     -1,
			stopBlock:      0, // Will halt on first partial block
			productionMode: true,
			spkgFile:       "./dummy/e2e-v0.1.0.spkg",
			outputModule:   "map_events",
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
				PartialBlocks:   true,
			}

			// Run the request with custom handling for partial blocks
			resps, err := RunRequestWithPartialBlocks(t, request, substreamsEndpoint, 5)

			if err != nil && err != io.EOF && !errors.Is(err, context.Canceled) {
				// Let's try to get container logs to debug
				logs, logErr := container.Logs(ctx)
				if logErr == nil {
					defer logs.Close()
					buf := make([]byte, 4096)
					n, _ := logs.Read(buf)
					t.Logf("Container logs: %s", string(buf[:n]))
				}
				t.Fatalf("Unexpected error: %s", err)
			}

			blockScopedDataSlice := resps.BlockScopedData()
			session := resps.SessionInit()
			var fullBlocks int
			var partialBlocks int
			for _, blkdata := range blockScopedDataSlice {
				if blkdata.IsPartial {
					partialBlocks++
				} else {
					fullBlocks++
				}
			}

			require.NotNil(t, session, "Should have received at least one session")
			assert.True(t, partialBlocks > 0, "Should have received partial block data")
			t.Logf("Received %d block scoped data items and %d partial responses", fullBlocks, partialBlocks)
		})
	}

	// ensure we close this well, for next tests
	app.Shutdown(nil)
	app2.Shutdown(nil)
	<-app.Terminated()
	<-app2.Terminated()

}

func TestPartialBlocksWithStores(t *testing.T) {
	testCases := []struct {
		name           string
		startBlock     int64
		stopBlock      uint64
		productionMode bool
		spkgFile       string
		outputModule   string
	}{
		{
			name:           "dev with stores",
			startBlock:     300,
			stopBlock:      330,
			productionMode: false,
			spkgFile:       "./partial_blocks_store/partial-blocks-store-v0.1.0.spkg",
			outputModule:   "map_tx_counter_summary",
		},
		{
			name:           "prod with stores",
			startBlock:     300,
			stopBlock:      330,
			productionMode: true,
			spkgFile:       "./partial_blocks_store/partial-blocks-store-v0.1.0.spkg",
			outputModule:   "map_tx_counter_summary",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {

			ctx := context.Background()
			zlog := zap.NewNop()
			// zlog := MustCreateLoggerWithServiceName("partial-blocks-test") // use this to include verbose substreams service logs

			// Create temporary directory for volume mount
			tmpDir := t.TempDir()

			// launch dummy blockchain container with flash blocks enabled
			image := latestDummyBlockchainImage
			burst := int(tc.startBlock)

			t.Logf("Starting container with image: %s and burst %d", image, burst)
			container, err := newDummyBlockchainContainer(ctx, tmpDir, image, "--with-flash-blocks --with-reorgs", burst)
			require.NoError(t, err)
			defer container.Terminate(ctx, testcontainers.StopTimeout(0))

			// Log container details for debugging
			if container != nil {
				if ports, portErr := container.Ports(ctx); portErr == nil {
					t.Logf("Container exposed ports: %v", ports)
				}
				if logs, logErr := container.Logs(ctx); logErr == nil {
					defer logs.Close()
					buf := make([]byte, 2048)
					if n, _ := logs.Read(buf); n > 0 {
						t.Logf("Container logs: %s", string(buf[:n]))
					}
				}
			}

			app2, t2Endpoint := startTier2App(t, ctx, tmpDir, zlog)
			app, substreamsEndpoint := startTier1App(t, ctx, tmpDir, container, t2Endpoint, zlog)

			defer func() {
				fmt.Println("Terminating container...")
				container.Terminate(ctx, testcontainers.StopTimeout(0))
				// ensure we close this well, for next tests
				app.Shutdown(nil)
				app2.Shutdown(nil)
				<-app.Terminated()
				<-app2.Terminated()
			}()

			pkg, err := manifest.MustNewReader(tc.spkgFile).Read()
			require.NoError(t, err)

			requestPartials := &pbsubstreamsrpcv3.Request{
				StartBlockNum:   tc.startBlock,
				StopBlockNum:    tc.stopBlock,
				FinalBlocksOnly: false,
				ProductionMode:  tc.productionMode,
				OutputModule:    tc.outputModule,
				Package:         pkg.Package,
				PartialBlocks:   true,
			}

			requestNoPartials := &pbsubstreamsrpcv3.Request{
				StartBlockNum:   tc.startBlock,
				StopBlockNum:    tc.stopBlock,
				FinalBlocksOnly: false,
				ProductionMode:  tc.productionMode,
				OutputModule:    tc.outputModule,
				Package:         pkg.Package,
			}

			respsNoPartialsCh := make(chan responses)
			go func() {
				data, init, err := RunRequest(t, requestNoPartials, substreamsEndpoint)
				if err != nil && err != io.EOF && !errors.Is(err, context.Canceled) {
					// Let's try to get container logs to debug
					logs, logErr := container.Logs(ctx)
					if logErr == nil {
						defer logs.Close()
						buf := make([]byte, 4096)
						n, _ := logs.Read(buf)
						t.Logf("Container logs: %s", string(buf[:n]))
					}
					panic(err)
				}
				result := responses{init}
				for _, d := range data {
					result = append(result, d)
				}
				respsNoPartialsCh <- result

			}()

			// Run the request with custom handling for partial blocks
			respsPartials, err := RunRequestWithPartialBlocks(t, requestPartials, substreamsEndpoint, 0)
			if err != nil && err != io.EOF && !errors.Is(err, context.Canceled) {
				// Let's try to get container logs to debug
				logs, logErr := container.Logs(ctx)
				if logErr == nil {
					defer logs.Close()
					buf := make([]byte, 4096)
					n, _ := logs.Read(buf)
					t.Logf("Container logs: %s", string(buf[:n]))
				}
				t.Fatalf("Unexpected error: %s", err)
			}
			respsNoPartials := <-respsNoPartialsCh

			ref := make(map[uint64]*pbsubstreamsrpcv2.BlockScopedData)
			for _, data := range respsNoPartials.BlockScopedData() {
				ref[data.Clock.Number] = data
			}

			finalPartialCount := 0
			for _, data := range respsPartials.BlockScopedData() {
				if data.IsLastPartial != nil && *data.IsLastPartial {
					require.NotNil(t, ref[data.Clock.Number], "Partial block should have a corresponding full block")

					blockNumber, currentTxCount, storedPrevBlockTxCount, storedTotalTxCount, ok := ParseTxCounterSummary(data.Output)
					require.True(t, ok, "Failed to parse tx counter summary")

					refBlockNumber, refCurrentTxCount, refStoredPrevBlockTxCount, refStoredTotalTxCount, ok := ParseTxCounterSummary(ref[data.Clock.Number].Output)
					require.True(t, ok, "Failed to parse tx counter summary")

					assert.Equal(t, refBlockNumber, blockNumber, "Partial block number should match full block number")
					assert.GreaterOrEqual(t, refCurrentTxCount, currentTxCount, "Partial block current tx count should be less or equal to full block current tx count -- this one is not identical")
					assert.Equal(t, storedPrevBlockTxCount, refStoredPrevBlockTxCount, "Partial block stored prev block tx count should match full block stored prev block tx count")
					assert.Equalf(t, int(refStoredTotalTxCount), int(storedTotalTxCount), "Partial block stored total tx count should match full block stored total tx count in block %d", blockNumber)
					finalPartialCount++
				}
			}
			assert.GreaterOrEqual(t, finalPartialCount, 20)        // should validate against at least 20 partial blocks
			assert.GreaterOrEqual(t, len(respsPartials.Undo()), 2) // should validate against at least 2 undo

		})
	}
}

func TestPartialBlocksReorgs(t *testing.T) {
	testCases := []struct {
		name           string
		startBlock     int64
		stopBlock      uint64
		productionMode bool
		spkgFile       string
		outputModule   string
		partialBlocks  bool
		assertFunc     func(t *testing.T, allResponses string)
	}{
		{
			name:           "with undo, only partial blocks",
			startBlock:     0,
			stopBlock:      40,
			partialBlocks:  true,
			productionMode: false,
			spkgFile:       "./dummy/e2e-v0.1.0.spkg",
			outputModule:   "map_events_0",
			assertFunc: func(t *testing.T, allResponses string) {
				assert.Contains(t, allResponses,
					"P:32:1,P:32:2,P:32:3,P:32:4*,"+ // partials for block 32 (normal)
						"P:33:1,P:33:2,P:33:3,U:32,F:33,"+ // partials for block 33 (got no p4 from dummy-blockchain: multiples of 11 are missing the lastFinal)
						"P:34:1,P:34:2,P:34:3,P:34:4*,"+ // partials for block 34
						"F:35,"+ // sudden "full" block 35
						"U:33,"+ // undo up to block 33
						"F:34,"+"P:35:4*,"+ // get the right block 34 and 35 (from "full")
						"P:36:1,P:36:2,P:36:3,P:36:4*,", // partials for block 36 (back on track)
					"did not contain expected partial blocks + undo sequence")
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {

			ctx := context.Background()
			// zlog := logging.MustCreateLoggerWithServiceName("partial-blocks-test")
			// zlog := zap.NewNop()

			// Create temporary directory for volume mount
			tmpDir := t.TempDir()

			// launch dummy blockchain container with flash blocks enabled
			image := latestDummyBlockchainImage

			burst := 0

			t.Logf("Starting container with image: %s and burst %d", image, burst)
			container, err := newDummyBlockchainContainer(ctx, tmpDir, image, "--with-flash-blocks --with-skipped-blocks=false --with-reorgs --block-rate=220", burst)
			require.NoError(t, err)

			// Log container details for debugging
			if container != nil {
				if ports, portErr := container.Ports(ctx); portErr == nil {
					t.Logf("Container exposed ports: %v", ports)
				}
				if logs, logErr := container.Logs(ctx); logErr == nil {
					defer logs.Close()
					buf := make([]byte, 2048)
					if n, _ := logs.Read(buf); n > 0 {
						t.Logf("Container logs: %s", string(buf[:n]))
					}
				}
			}

			app2, t2Endpoint := startTier2App(t, ctx, tmpDir, zlog)
			app, substreamsEndpoint := startTier1App(t, ctx, tmpDir, container, t2Endpoint, zlog)

			defer func() {
				container.Terminate(ctx, testcontainers.StopTimeout(0))
				// ensure we close this well, for next tests
				app.Shutdown(nil)
				app2.Shutdown(nil)
				<-app.Terminated()
				<-app2.Terminated()
			}()

			pkg, err := manifest.MustNewReader(tc.spkgFile).Read()
			require.NoError(t, err)

			request := &pbsubstreamsrpcv3.Request{

				StopBlockNum:    tc.stopBlock,
				FinalBlocksOnly: false,
				ProductionMode:  tc.productionMode,
				OutputModule:    tc.outputModule,
				Package:         pkg.Package,
				PartialBlocks:   tc.partialBlocks,
			}

			// Run the request with custom handling for partial blocks
			resps, err := RunRequestWithPartialBlocks(t, request, substreamsEndpoint, 300)
			if err != nil && err != io.EOF && !errors.Is(err, context.Canceled) {
				// Let's try to get container logs to debug
				logs, logErr := container.Logs(ctx)
				if logErr == nil {
					defer logs.Close()
					buf := make([]byte, 4096)
					n, _ := logs.Read(buf)
					t.Logf("Container logs: %s", string(buf[:n]))
				}
				t.Fatalf("Unexpected error: %s", err)
			}

			allResponses := strings.Join(resps.Strings(), ",")
			tc.assertFunc(t, allResponses)
			require.NotNil(t, resps.SessionInit(), "Should have received at least one session")

		})

	}

}

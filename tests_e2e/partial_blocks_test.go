package tests_e2e

import (
	"context"
	"errors"
	"io"
	"os"
	"testing"

	"github.com/streamingfast/substreams/manifest"
	pbsubstreamsrpcv2 "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestPartialBlocksSimple(t *testing.T) {
	ctx := context.Background()
	//zlog := logging.MustCreateLoggerWithServiceName("partial-blocks-test")
	zlog := zap.NewNop()

	// Create temporary directory for volume mount
	tmpDir, err := os.MkdirTemp("", "firehose-data-")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// launch dummy blockchain container with flash blocks enabled
	image := "ghcr.io/streamingfast/dummy-blockchain:17b576d"
	burst := 120

	t.Logf("Starting container with image: %s and burst %d", image, burst)
	container, err := newDummyBlockchainContainer(ctx, tmpDir, image, "--with-flash-blocks", burst)
	require.NoError(t, err)
	defer container.Terminate(ctx)

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

			request := &pbsubstreamsrpcv2.Request{
				StartBlockNum:   tc.startBlock,
				StopBlockNum:    tc.stopBlock,
				FinalBlocksOnly: false,
				ProductionMode:  tc.productionMode,
				OutputModule:    tc.outputModule,
				Modules:         pkg.Package.Modules,
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
					fullBlocks++
				} else {
					partialBlocks++
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
	ctx := context.Background()
	zlog := zap.NewNop()
	// zlog := MustCreateLoggerWithServiceName("partial-blocks-test") // use this to include verbose substreams service logs

	// Create temporary directory for volume mount
	tmpDir, err := os.MkdirTemp("", "firehose-data-")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// launch dummy blockchain container with flash blocks enabled
	image := "ghcr.io/streamingfast/dummy-blockchain:17b576d"
	burst := 300

	t.Logf("Starting container with image: %s and burst %d", image, burst)
	container, err := newDummyBlockchainContainer(ctx, tmpDir, image, "--with-flash-blocks", burst)
	require.NoError(t, err)
	defer container.Terminate(ctx)

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

	testCases := []struct {
		name                   string
		startBlock             int64
		stopBlock              uint64
		productionMode         bool
		spkgFile               string
		outputModule           string
		expectPartialResponses int
	}{
		{
			name:                   "dev with stores",
			startBlock:             100,
			stopBlock:              0, // Will halt after the requested number of partial responses
			productionMode:         false,
			spkgFile:               "./partial_blocks_store/partial-blocks-store-v0.1.0.spkg",
			outputModule:           "map_tx_counter_summary",
			expectPartialResponses: 100,
		},
		{
			name:                   "prod with stores",
			startBlock:             200,
			stopBlock:              0, // Will halt after the requested number of partial responses
			productionMode:         false,
			spkgFile:               "./partial_blocks_store/partial-blocks-store-v0.1.0.spkg",
			outputModule:           "map_tx_counter_summary",
			expectPartialResponses: 50,
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
				PartialBlocks:   true,
			}

			// Run the request with custom handling for partial blocks
			resps, err := RunRequestWithPartialBlocks(t, request, substreamsEndpoint, tc.expectPartialResponses)
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
			blockScopedData := resps.BlockScopedData()
			session := resps.SessionInit()

			require.NotNil(t, session, "Should have received at least one session")

			type fullBlockResponse struct {
				currentTxCount uint64
				storedTxCount  uint64
				totalTxCount   uint64
			}

			//seenBlocks := make(map[uint64]fullBlockResponse)

			//var partialResponses []*pbsubstreamsrpcv2.BlockScopedData

			prevBlock := uint64(0)
			prevBlockTxCount := uint64(0)
			var prevBlockWasFull bool
			var prevBlockWasLastPartial bool
			totalTxCount := uint64(0)

			for _, fullResponse := range blockScopedData {

				blockNumber, currentTxCount, storedPrevBlockTxCount, storedTotalTxCount, ok := ParseTxCounterSummary(fullResponse.Output)
				if !ok {
					t.Fatal("Failed to parse TxCounterSummary from MapOutput")
				}

				// heavy print-debugging
				// partialIndex := 9999
				// if fullResponse.PartialIndex != nil {
				// 	partialIndex = int(*fullResponse.PartialIndex)
				// }
				// fmt.Printf("Processing response block %d (currentTxCount %d prevBlockStoredTxCount %d totalTxCount %d partialIndex %d)\n", blockNumber, currentTxCount, storedPrevBlockTxCount, totalTxCount, partialIndex)

				// first block ever
				if prevBlock == 0 {
					prevBlock = blockNumber
					prevBlockTxCount = currentTxCount
					totalTxCount = storedTotalTxCount
					continue
				}

				totalTxCount += currentTxCount // happens on every block

				if fullResponse.IsPartial {
					assert.Equal(t, int(totalTxCount), int(storedTotalTxCount))

					if blockNumber == prevBlock {
						assert.False(t, prevBlockWasFull, "prev block was full, receiving a partial %d", blockNumber)
						assert.False(t, prevBlockWasLastPartial, "prev block was last partial, receiving another partial %d", blockNumber)

						prevBlockTxCount += currentTxCount // append to block
					} else {
						assert.Equal(t, int(prevBlockTxCount), int(storedPrevBlockTxCount), "at block %d", blockNumber)
						assert.True(t, prevBlockWasFull || prevBlockWasLastPartial, "prev block was not full NOR last partial")

						prevBlock = blockNumber
						prevBlockTxCount = currentTxCount // replace
					}

					prevBlockWasFull = false
					prevBlockWasLastPartial = *fullResponse.IsLastPartial

				} else {
					require.True(t, blockNumber == prevBlock+1, "non-consecutive block numbers, prev %d, current %d", prevBlock, blockNumber)

					// full block always replaces
					prevBlock = blockNumber
					prevBlockTxCount = currentTxCount
					prevBlockWasFull = true
					prevBlockWasLastPartial = false
				}
			}

		})
	}
	// ensure we close this well, for next tests
	app.Shutdown(nil)
	app2.Shutdown(nil)
	<-app.Terminated()
	<-app2.Terminated()

}

//
//func TestPartialBlocksReorgs(t *testing.T) {
//	testCases := []struct {
//		name                 string
//		startBlock           int64
//		stopBlock            uint64
//		productionMode       bool
//		spkgFile             string
//		outputModule         string
//		partialBlocksOnly    bool
//		includePartialBlocks bool
//		assertFunc           func(t *testing.T, allResponses string)
//	}{
//		{
//			name:              "with undo, only partial blocks",
//			startBlock:        0,
//			stopBlock:         40,
//			partialBlocksOnly: true,
//			productionMode:    false,
//			spkgFile:          "./dummy/e2e-v0.1.0.spkg",
//			outputModule:      "map_events_0",
//			assertFunc: func(t *testing.T, allResponses string) {
//				assert.True(t, strings.Contains(allResponses,
//					"P:33:1,P:33:2,P:33:3,P:33:4,P:33:10,"+ // partials for block 33
//						"P:34:1,P:34:2,P:34:3,P:34:4,"+ // partials for block 34
//						"P:35:10,"+ // sudden "full" block 35
//						"U:33,"+ // undo block 33
//						"P:34:10,"+"P:35:10,"+ // get the right block 34 and 35 (from "full")
//						"P:36:1,P:36:2,P:36:3,P:36:4,", // partials for block 36 (back on track)
//				), "did not contain expected partial blocks + undo sequence")
//			},
//		},
//		{
//			name:                 "with undo, include partial blocks",
//			startBlock:           0,
//			stopBlock:            40,
//			productionMode:       false,
//			includePartialBlocks: true,
//			spkgFile:             "./dummy/e2e-v0.1.0.spkg",
//			outputModule:         "map_events_0",
//			assertFunc: func(t *testing.T, allResponses string) {
//				assert.True(t, strings.Contains(allResponses,
//					"P:33:1,P:33:2,P:33:3,P:33:4,P:33:10,"+ // partials for block 33
//						"F:33,"+ // full block 33
//						"P:34:1,P:34:2,P:34:3,P:34:4,"+ // partials for block 34
//						"F:34,"+ // full block 34
//						"P:35:10,"+ // sudden "full" block 35 causing partial:10
//						"F:35,"+ // full block 35
//						"U:33,"+ // undo block 33
//						"P:34:10,"+ // get the right block 34 (from "full")
//						"F:34,"+ // full block 34
//						"P:35:10,"+ // get the right block 35 (from "full")
//						"F:35,"+ // full block 35
//						"P:36:1,P:36:2,P:36:3,P:36:4,", // partials for block 36 (back on track)
//				), "did not contain expected full/partial blocks + undo sequence. Allresponses:%s", allResponses)
//			},
//		},
//	}
//
//	for _, tc := range testCases {
//		t.Run(tc.name, func(t *testing.T) {
//
//			ctx := context.Background()
//			// zlog := logging.MustCreateLoggerWithServiceName("partial-blocks-test")
//			zlog := zap.NewNop()
//
//			// Create temporary directory for volume mount
//			tmpDir, err := os.MkdirTemp("", "firehose-data-")
//			require.NoError(t, err)
//			defer os.RemoveAll(tmpDir)
//
//			// launch dummy blockchain container with flash blocks enabled
//			image := "ghcr.io/streamingfast/dummy-blockchain:17b576d"
//			burst := 0
//
//			t.Logf("Starting container with image: %s and burst %d", image, burst)
//			container, err := newDummyBlockchainContainer(ctx, tmpDir, image, "--with-flash-blocks --with-skipped-blocks=false --with-reorgs --block-rate=220", burst)
//			require.NoError(t, err)
//			defer container.Terminate(ctx)
//
//			// Log container details for debugging
//			if container != nil {
//				if ports, portErr := container.Ports(ctx); portErr == nil {
//					t.Logf("Container exposed ports: %v", ports)
//				}
//				if logs, logErr := container.Logs(ctx); logErr == nil {
//					defer logs.Close()
//					buf := make([]byte, 2048)
//					if n, _ := logs.Read(buf); n > 0 {
//						t.Logf("Container logs: %s", string(buf[:n]))
//					}
//				}
//			}
//
//			app2, t2Endpoint := startTier2App(t, ctx, tmpDir, zlog)
//			app, substreamsEndpoint := startTier1App(t, ctx, tmpDir, container, t2Endpoint, zlog)
//
//			pkg, err := manifest.MustNewReader(tc.spkgFile).Read()
//			require.NoError(t, err)
//
//			request := &pbsubstreamsrpcv2.Request{
//				StartBlockNum:        tc.startBlock,
//				StopBlockNum:         tc.stopBlock,
//				FinalBlocksOnly:      false,
//				ProductionMode:       tc.productionMode,
//				OutputModule:         tc.outputModule,
//				Modules:              pkg.Package.Modules,
//				PartialBlocksOnly:    tc.partialBlocksOnly,
//				IncludePartialBlocks: tc.includePartialBlocks,
//			}
//
//			// Run the request with custom handling for partial blocks
//			resps, err := RunRequestWithPartialBlocks(t, request, substreamsEndpoint, 300)
//			if err != nil && err != io.EOF && !errors.Is(err, context.Canceled) {
//				// Let's try to get container logs to debug
//				logs, logErr := container.Logs(ctx)
//				if logErr == nil {
//					defer logs.Close()
//					buf := make([]byte, 4096)
//					n, _ := logs.Read(buf)
//					t.Logf("Container logs: %s", string(buf[:n]))
//				}
//				t.Fatalf("Unexpected error: %s", err)
//			}
//
//			allResponses := strings.Join(resps.Strings(), ",")
//			tc.assertFunc(t, allResponses)
//			require.NotNil(t, resps.SessionInit(), "Should have received at least one session")
//
//			// ensure we close this well, for next tests
//			app.Shutdown(nil)
//			app2.Shutdown(nil)
//			<-app.Terminated()
//			<-app2.Terminated()
//		})
//
//	}
//
//}
//

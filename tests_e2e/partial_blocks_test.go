package tests_e2e

import (
	"context"
	"errors"
	"io"
	"os"
	"testing"

	"github.com/streamingfast/logging"
	"github.com/streamingfast/substreams/client"
	"github.com/streamingfast/substreams/manifest"
	pbsubstreamsrpcv2 "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPartialBlocksContainer(t *testing.T) {
	ctx := context.Background()
	zlog := logging.MustCreateLoggerWithServiceName("partial-blocks-test")

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

	app, substreamsEndpoint := startTier1App(t, ctx, tmpDir, container, zlog)
	app2 := startTier2App(t, ctx, tmpDir, zlog)

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
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			pkg, err := manifest.MustNewReader(tc.spkgFile).Read()
			require.NoError(t, err)

			request := &pbsubstreamsrpcv2.Request{
				StartBlockNum:        tc.startBlock,
				StopBlockNum:         tc.stopBlock,
				FinalBlocksOnly:      false,
				ProductionMode:       tc.productionMode,
				OutputModule:         tc.outputModule,
				Modules:              pkg.Package.Modules,
				IncludePartialBlocks: true,
			}

			// Run the request with custom handling for partial blocks
			blockScopedDataSlice, session, partialBlockReceived, err := RunRequestWithPartialBlocks(t, request, substreamsEndpoint)
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

			require.NotNil(t, session, "Should have received at least one session")
			assert.True(t, partialBlockReceived, "Should have received partial block data")
			t.Logf("Received %d block scoped data items", len(blockScopedDataSlice))
		})
	}

	// ensure we close this well, for next tests
	app.Shutdown(nil)
	app2.Shutdown(nil)
	<-app.Terminated()
	<-app2.Terminated()

}

// RunRequestWithPartialBlocks is similar to RunRequest but specifically looks for partial block data
func RunRequestWithPartialBlocks(t *testing.T, req *pbsubstreamsrpcv2.Request, endpoint string) ([]*pbsubstreamsrpcv2.BlockScopedData, *pbsubstreamsrpcv2.SessionInit, bool, error) {

	ctx := t.Context()
	// substreams client
	//
	conn, closeFunc, callOpts, _, err := client.NewSubstreamsClientConn(client.NewSubstreamsClientConfig(
		client.SubstreamsClientConfigOptions{
			Endpoint:  endpoint,
			AuthType:  client.None,
			PlainText: true,
			Agent:     "test-client",
		},
	))
	require.NoError(t, err)
	defer func() {
		err := closeFunc()
		require.NoError(t, err)
	}()

	// Debug: Log the endpoint we're connecting to
	t.Logf("Connecting to endpoint: %s", endpoint)

	// Create client
	streamClient := pbsubstreamsrpcv2.NewStreamClient(conn)

	// Make the streaming call
	stream, err := streamClient.Blocks(ctx, req, callOpts...)
	if err != nil {
		t.Logf("Error making streaming call: %v", err)
		require.NoError(t, err)
	}

	// Read all responses and accumulate BlockScopedData
	var blockScopedDataSlice []*pbsubstreamsrpcv2.BlockScopedData
	var session *pbsubstreamsrpcv2.SessionInit
	var partialBlockReceived bool

	for {
		var response *pbsubstreamsrpcv2.Response
		response, err = stream.Recv()
		if err != nil {
			t.Logf("Stream ended or error: %v", err)
			break
		}
		require.NotNil(t, response)

		// Validate session if received
		if sess := response.GetSession(); sess != nil {
			session = sess
			t.Logf("Received session: %+v", session)
		}

		// Check for partial block data and halt when received
		if partialData := response.GetPartialBlockData(); partialData != nil {
			partialBlockReceived = true
			t.Logf("Received partial block data - halting request")
			break
		}

		// Accumulate BlockScopedData
		if blockData := response.GetBlockScopedData(); blockData != nil {
			blockScopedDataSlice = append(blockScopedDataSlice, blockData)
			t.Logf("Accumulated BlockScopedData clock %d, total count: %d", blockData.Clock.Number, len(blockScopedDataSlice))
		}
	}

	return blockScopedDataSlice, session, partialBlockReceived, err
}

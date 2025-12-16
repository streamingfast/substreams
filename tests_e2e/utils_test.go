package tests_e2e

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/streamingfast/dauth"
	dauthnull "github.com/streamingfast/dauth/null"
	dauthtrust "github.com/streamingfast/dauth/trust"
	"github.com/streamingfast/dmetering/logger"
	"github.com/streamingfast/dmetrics"
	"github.com/streamingfast/dsession"
	_ "github.com/streamingfast/dsession/local"
	"github.com/streamingfast/substreams/app"
	"github.com/streamingfast/substreams/client"
	pbsubstreamsrpcv2 "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
	pbtest "github.com/streamingfast/substreams/tests_e2e/partial_blocks_store/pb"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"go.uber.org/zap"
)

func init() {
	logger.Register()
}

// ParseTxCounterSummary unmarshals the MapOutput protobuf Any field into a TxCounterSummary
// and returns the key values: blockNumber, currentTxCount, storedTxCount, totalTxCount
// Returns ok=false if the unmarshaling fails
func ParseTxCounterSummary(output *pbsubstreamsrpcv2.MapModuleOutput) (blockNumber, currentTxCount, storedTxCount, totalTxCount uint64, ok bool) {
	if output == nil || output.MapOutput == nil {
		return 0, 0, 0, 0, false
	}

	var summary pbtest.TxCounterSummary
	if err := output.MapOutput.UnmarshalTo(&summary); err != nil {
		return 0, 0, 0, 0, false
	}

	// Convert int64 to uint64 for the counts (they should be positive values)
	currentTxCount = uint64(summary.CurrentBlockTxCount)
	totalTxCount = uint64(summary.TotalTxCount)

	// Calculate stored tx count from the block_counts slice
	storedTxCount = 0
	for _, blockCount := range summary.BlockCounts {
		storedTxCount += uint64(blockCount.TxCount)
	}

	return summary.BlockNumber, currentTxCount, storedTxCount, totalTxCount, true
}

func RunRequest(t *testing.T, req *pbsubstreamsrpcv2.Request, endpoint string) ([]*pbsubstreamsrpcv2.BlockScopedData, *pbsubstreamsrpcv2.SessionInit, error) {

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

		// Accumulate BlockScopedData
		if blockData := response.GetBlockScopedData(); blockData != nil {
			blockScopedDataSlice = append(blockScopedDataSlice, blockData)
			//t.Logf("Accumulated BlockScopedData clock %d, total count: %d", blockData.Clock.Number, len(blockScopedDataSlice))
		}
	}

	return blockScopedDataSlice, session, err

}

func startTier1App(t *testing.T, ctx context.Context, tmpDir string, container testcontainers.Container, t2Endpoint string, zlog *zap.Logger) (*app.Tier1App, string) {

	os.Setenv("SUBSTREAMS_WORKERS_RAMPUP_TIME", "0")

	relayerPort, err := container.MappedPort(ctx, "10014/tcp")
	require.NoError(t, err)

	relayerEndpoint := fmt.Sprintf("localhost:%d", relayerPort.Int())
	t.Logf("Tier1App relayer endpoint: %s", relayerEndpoint)

	listenPort := findFreePort(t)
	substreamsEndpoint := fmt.Sprintf("localhost:%d", listenPort)
	t1conf := &app.Tier1Config{
		GRPCListenAddr:               substreamsEndpoint,
		OneBlocksStoreURL:            filepath.Join(tmpDir, "one-blocks"),
		MergedBlocksStoreURL:         filepath.Join(tmpDir, "merged-blocks"),
		BlockStreamAddr:              relayerEndpoint,
		MeteringConfig:               "logger://",
		FoundationalStoresConfigPath: "",
		ForkedBlocksStoreURL:         "",
		GRPCShutdownGracePeriod:      0,
		ServiceDiscoveryURL:          nil,
		BlockExecutionTimeout:        5 * time.Second,
		TmpDir:                       filepath.Join(tmpDir, "tmp"),
		StateStoreURL:                filepath.Join(tmpDir, "states"),
		QuickSaveStoreURL:            "",
		StateStoreDefaultTag:         "",
		BlockType:                    "sf.acme.type.v1.Block",
		StateBundleSize:              100,
		SubrequestsEndpoint:          t2Endpoint,
		SubrequestsPlaintext:         true,
		MaxSubrequests:               10,
	}

	// unused components, but required for the tier1 to start
	dauthnull.Register()
	auth, err := dauth.New("null://", zlog)
	require.NoError(t, err)
	metricset := dmetrics.NewSet()
	headBlockNumMetric := metricset.NewHeadBlockNumber("test-firehose")
	headTimeDriftmetric := metricset.NewHeadTimeDrift("test-firehose")

	sessionPool, err := dsession.New("local://localhost?max_workers=10&max_workers_per_session=5", zlog)
	require.NoError(t, err)
	t1app := app.NewTier1(zlog, t1conf, &app.Tier1Modules{
		HeadTimeDriftMetric:   headTimeDriftmetric,
		HeadBlockNumberMetric: headBlockNumMetric,
		Authenticator:         auth,
		SessionPool:           sessionPool,
	})
	go func() {
		err := t1app.Run()
		require.NoError(t, err)
	}()

	// Wait for t1app to be ready with polling
	timeout := time.After(20 * time.Second)
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()

waitReady:
	for {
		select {
		case <-timeout:
			t.Fatal("timeout waiting for t1app to be ready")
		case <-ticker.C:
			if t1app.IsReady(ctx) {
				t.Logf("t1app is ready")
				break waitReady
			}
		}
	}

	return t1app, substreamsEndpoint
}

func startTier2App(t *testing.T, ctx context.Context, tmpDir string, zlog *zap.Logger) (out *app.Tier2App, endpoint string) {

	port := findFreePort(t)
	endpoint = fmt.Sprintf("localhost:%d", port)
	dauthtrust.Register()

	t2conf := &app.Tier2Config{
		GRPCListenAddr:        endpoint,
		ServiceDiscoveryURL:   nil,
		BlockExecutionTimeout: 5 * time.Second,
		TmpDir:                filepath.Join(tmpDir, "tmp"),
	}

	t2app := app.NewTier2(zlog, t2conf, &app.Tier2Modules{
		CheckPendingShutDown: func() bool {
			return false
		},
	})
	go func() {
		err := t2app.Run()
		require.NoError(t, err)
	}()

	// Wait for t2app to be ready with polling
	timeout := time.After(5 * time.Second)
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()

waitReady:
	for {
		select {
		case <-timeout:
			t.Fatal("timeout waiting for t2app to be ready")
		case <-ticker.C:
			if t2app.IsReady(ctx) {
				t.Logf("t1app is ready")
				break waitReady
			}
		}
	}

	return t2app, endpoint
}

func newDummyBlockchainContainer(ctx context.Context, tmpDir string, image string, additionalReaderArgs string, burst int) (testcontainers.Container, error) {
	baseReaderArgs := fmt.Sprintf("start --log-level=error --tracer=firehose --store-dir=/data --genesis-block-burst=%d --block-rate=120 --block-size=1230 --genesis-height=0 --server-addr=:9777 --with-reorgs=false --with-skipped-blocks=false", burst)
	readerArgs := baseReaderArgs
	if additionalReaderArgs != "" {
		readerArgs = baseReaderArgs + " " + additionalReaderArgs
	}

	req := testcontainers.ContainerRequest{
		Image: image,
		Cmd: []string{
			"start",
			"reader-node",
			"merger",
			"relayer",
			"-c",
			"",
			"--advertise-chain-name=acme-dummy-blockchain",
			"--reader-node-path=dummy-blockchain",
			"--reader-node-arguments=" + readerArgs,
			"--advertise-block-id-encoding=hex",
		},
		ExposedPorts: []string{"10014/tcp"},
		HostConfigModifier: func(hostConfig *container.HostConfig) {
			hostConfig.Binds = []string{tmpDir + ":/app/firehose-data/storage/"}
		},
		WaitingFor: wait.ForAll(
			wait.ForListeningPort("10014/tcp"),
			wait.ForLog("serving gRPC").WithStartupTimeout(30*time.Second),
		),
	}

	// Start container
	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})

	if err != nil {
		return nil, err
	}

	return container, nil
}

// RunRequestWithPartialBlocks is similar to RunRequest but specifically looks for partial block data
func RunRequestWithPartialBlocks(t *testing.T, req *pbsubstreamsrpcv2.Request, endpoint string, partialResponseCount int) ([]*pbsubstreamsrpcv2.BlockScopedData, *pbsubstreamsrpcv2.SessionInit, []*pbsubstreamsrpcv2.PartialBlockData, error) {

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

	// Read all responses and accumulate BlockScopedData and PartialBlockData
	var blockScopedDataSlice []*pbsubstreamsrpcv2.BlockScopedData
	var partialBlockDataSlice []*pbsubstreamsrpcv2.PartialBlockData
	var session *pbsubstreamsrpcv2.SessionInit

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

		// Check for partial block data and accumulate
		if partialData := response.GetPartialBlockData(); partialData != nil {
			partialBlockDataSlice = append(partialBlockDataSlice, partialData)
			//t.Logf("Received partial block data %d/%d", len(partialBlockDataSlice), partialResponseCount)

			// Break if we've received the desired number of partial responses
			if len(partialBlockDataSlice) >= partialResponseCount {
				t.Logf("Received %d partial responses - halting request", len(partialBlockDataSlice))
				break
			}
		}

		// Accumulate BlockScopedData
		if blockData := response.GetBlockScopedData(); blockData != nil {
			blockScopedDataSlice = append(blockScopedDataSlice, blockData)
			//t.Logf("Accumulated BlockScopedData clock %d, total count: %d", blockData.Clock.Number, len(blockScopedDataSlice))
		}
	}

	return blockScopedDataSlice, session, partialBlockDataSlice, err
}

func findFreePort(t *testing.T) int {
	// Listen on port 0, which tells the OS to pick any available port
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Error(err)
	}
	defer listener.Close()

	addr := listener.Addr().(*net.TCPAddr)
	return addr.Port
}

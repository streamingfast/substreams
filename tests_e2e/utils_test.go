package tests_e2e

import (
	"context"
	"fmt"
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
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"go.uber.org/zap"
)

func init() {
	logger.Register()
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
		response, err := stream.Recv()
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
			t.Logf("Accumulated BlockScopedData clock %d, total count: %d", blockData.Clock.Number, len(blockScopedDataSlice))
		}
	}

	return blockScopedDataSlice, session, nil

}

func startTier1App(t *testing.T, ctx context.Context, tmpDir string, container testcontainers.Container, zlog *zap.Logger) (*app.Tier1App, string) {

	port, err := container.MappedPort(ctx, "10014/tcp")
	require.NoError(t, err)

	relayerEndpoint := fmt.Sprintf("localhost:%d", port.Int())
	t.Logf("Tier1App relayer endpoint: %s", relayerEndpoint)
	substreamsEndpoint := ":10016"
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
		SubrequestsEndpoint:          "localhost:10017",
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

func startTier2App(t *testing.T, ctx context.Context, tmpDir string, zlog *zap.Logger) *app.Tier2App {

	dauthtrust.Register()

	t2conf := &app.Tier2Config{
		GRPCListenAddr:        ":10017",
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

	// Wait for t1app to be ready with polling
	timeout := time.After(5 * time.Second)
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()

waitReady:
	for {
		select {
		case <-timeout:
			t.Fatal("timeout waiting for t1app to be ready")
		case <-ticker.C:
			if t2app.IsReady(ctx) {
				t.Logf("t1app is ready")
				break waitReady
			}
		}
	}

	return t2app
}

func newDummyBlockchainContainer(ctx context.Context, tmpDir string) (testcontainers.Container, error) {

	req := testcontainers.ContainerRequest{
		Image: "ghcr.io/streamingfast/dummy-blockchain:v1.7.2",
		Cmd: []string{
			"start",
			"reader-node",
			"merger",
			"relayer",
			"-c",
			"",
			// --with-reorgs=false
			// --with-skipped-blocks=false
			"--advertise-chain-name=acme-dummy-blockchain",
			"--reader-node-path=dummy-blockchain",
			"--reader-node-arguments=start --log-level=error --tracer=firehose --store-dir=/data --genesis-block-burst=1000 --block-rate=600 --block-size=2560 --genesis-height=0 --server-addr=:9777",
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

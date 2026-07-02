package tests_e2e

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

<<<<<<< HEAD
=======
	"github.com/moby/moby/api/types/container"
>>>>>>> develop
	"github.com/streamingfast/bstream/stream"
	"github.com/streamingfast/dauth"
	dauthnull "github.com/streamingfast/dauth/null"
	dauthsecret "github.com/streamingfast/dauth/secret"
	dauthtrust "github.com/streamingfast/dauth/trust"
	"github.com/streamingfast/dmetering/logger"
	"github.com/streamingfast/dmetrics"
	"github.com/streamingfast/dsession"
	_ "github.com/streamingfast/dsession/local"
	"github.com/streamingfast/substreams/app"
	"github.com/streamingfast/substreams/client"
	pbsubstreamsrpcv2 "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
	pbsubstreamsrpcv3 "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v3"
	pbsubstreamsrpcv4 "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v4"
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
func ParseTxCounterSummary(output *pbsubstreamsrpcv2.MapModuleOutput) (blockNumber, currentTxCount, prevBlockTxCount, totalTxCount uint64, ok bool) {
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

	// Calculate stored tx count from the previous block in block_counts slice
	prevBlockTxCount = 0
	if len(summary.BlockCounts) > 1 {
		prevBlockTxCount = uint64(summary.BlockCounts[1].TxCount)
	}

	return summary.BlockNumber, currentTxCount, prevBlockTxCount, totalTxCount, true
}

func RunRequest(t *testing.T, req *pbsubstreamsrpcv3.Request, endpoint string) ([]*pbsubstreamsrpcv2.BlockScopedData, *pbsubstreamsrpcv2.SessionInit, error) {

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
	streamClient := pbsubstreamsrpcv4.NewStreamClient(conn)

	// Make the streaming call
	blockStream, err := streamClient.Blocks(ctx, req, callOpts...)
	if err != nil {
		t.Logf("Error making streaming call: %v", err)
		require.NoError(t, err)
	}

	// Read all responses and accumulate BlockScopedData
	var blockScopedDataSlice []*pbsubstreamsrpcv2.BlockScopedData
	var session *pbsubstreamsrpcv2.SessionInit

	for {
		var response *pbsubstreamsrpcv4.Response
		response, err = blockStream.Recv()
		if err != nil {
			if errors.Is(err, stream.ErrStopBlockReached) || errors.Is(err, io.EOF) {
				t.Logf("Reached stop block or EOF: %v", err)
				err = nil
			} else if strings.Contains(err.Error(), "RST_STREAM") {
				t.Logf("Error RST_STREAM in these tests usually means that the test panicked. Make sure that you run with `DLOG=info` (or similar) to see the server-side panic")
			} else {
				t.Logf("Unknown error: %v", err)
			}
			break
		}
		require.NotNil(t, response)

		// Validate session if received
		if sess := response.GetSession(); sess != nil {
			session = sess
			t.Logf("Received session: %+v", session)
		}

		// Accumulate BlockScopedData
		if blockDatas := response.GetBlockScopedDatas(); blockDatas != nil {
			blockScopedDataSlice = append(blockScopedDataSlice, blockDatas.Items...)
		}
	}

	return blockScopedDataSlice, session, err

}

func startTier1App(t *testing.T, ctx context.Context, tmpDir string, container testcontainers.Container, t2Endpoint string, zlog *zap.Logger) (*app.Tier1App, string) {

	os.Setenv("SUBSTREAMS_WORKERS_RAMPUP_TIME", "0")

	relayerPort, err := container.MappedPort(ctx, "10014/tcp")
	require.NoError(t, err)

	relayerEndpoint := fmt.Sprintf("localhost:%d", relayerPort.Num())
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

func startTier2App(t *testing.T, ctx context.Context, tmpDir string, zlog *zap.Logger, scratchSpace ...string) (out *app.Tier2App, endpoint string) {

	port := findFreePort(t)
	endpoint = fmt.Sprintf("localhost:%d", port)
	dauthtrust.Register()

	t2conf := &app.Tier2Config{
		GRPCListenAddr:        endpoint,
		ServiceDiscoveryURL:   nil,
		BlockExecutionTimeout: 5 * time.Second,
		TmpDir:                filepath.Join(tmpDir, "tmp"),
	}
	if len(scratchSpace) > 0 && scratchSpace[0] != "" {
		t2conf.StoresScratchSpace = scratchSpace[0]
	}

	t2app := app.NewTier2(zlog, t2conf, &app.Tier2Modules{
		CheckPendingShutDown: func() bool {
			return false
		},
	})

	return startTier2AppInternal(t, ctx, t2app, endpoint)
}

// startTier2AppWithSecret starts a tier2 that requires requests to carry
// "Authorization: Bearer <secret>" — mimicking a real deployment where tier1
// must authenticate its subrequests to tier2.
func startTier2AppWithSecret(t *testing.T, ctx context.Context, tmpDir string, secret string, zlog *zap.Logger) (out *app.Tier2App, endpoint string) {
	port := findFreePort(t)
	endpoint = fmt.Sprintf("localhost:%d", port)

	dauthsecret.Register()
	auth, err := dauth.New(fmt.Sprintf("secret://%s", secret), zlog)
	require.NoError(t, err)

	t2conf := &app.Tier2Config{
		GRPCListenAddr:        endpoint,
		ServiceDiscoveryURL:   nil,
		BlockExecutionTimeout: 5 * time.Second,
		TmpDir:                filepath.Join(tmpDir, "tmp"),
	}

	t2app := app.NewTier2(zlog, t2conf, &app.Tier2Modules{
		CheckPendingShutDown: func() bool { return false },
		Authenticator:        auth,
	})

	return startTier2AppInternal(t, ctx, t2app, endpoint)
}

func startTier2AppInternal(t *testing.T, ctx context.Context, t2app *app.Tier2App, endpoint string) (*app.Tier2App, string) {
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
				t.Logf("t2app is ready")
				break waitReady
			}
		}
	}

	return t2app, endpoint
}

// startTier1AppWithSecret starts a tier1 that sends subrequests to tier2 using the
// provided secret key in the Authorization header.
func startTier1AppWithSecret(t *testing.T, ctx context.Context, tmpDir string, container testcontainers.Container, t2Endpoint string, secret string, zlog *zap.Logger) (*app.Tier1App, string) {
	os.Setenv("SUBSTREAMS_WORKERS_RAMPUP_TIME", "0")

	relayerPort, err := container.MappedPort(ctx, "10014/tcp")
	require.NoError(t, err)

	relayerEndpoint := fmt.Sprintf("localhost:%d", relayerPort.Num())
	t.Logf("Tier1App (with secret) relayer endpoint: %s", relayerEndpoint)

	listenPort := findFreePort(t)
	substreamsEndpoint := fmt.Sprintf("localhost:%d", listenPort)
	t1conf := &app.Tier1Config{
		GRPCListenAddr:                substreamsEndpoint,
		OneBlocksStoreURL:             filepath.Join(tmpDir, "one-blocks"),
		MergedBlocksStoreURL:          filepath.Join(tmpDir, "merged-blocks"),
		BlockStreamAddr:               relayerEndpoint,
		MeteringConfig:                "logger://",
		FoundationalStoresConfigPath:  "",
		ForkedBlocksStoreURL:          "",
		GRPCShutdownGracePeriod:       0,
		ServiceDiscoveryURL:           nil,
		BlockExecutionTimeout:         5 * time.Second,
		TmpDir:                        filepath.Join(tmpDir, "tmp"),
		StateStoreURL:                 filepath.Join(tmpDir, "states"),
		QuickSaveStoreURL:             "",
		StateStoreDefaultTag:          "",
		BlockType:                     "sf.acme.type.v1.Block",
		StateBundleSize:               100,
		SubrequestsEndpoint:           t2Endpoint,
		SubrequestsPlaintext:          true,
		SubrequestsSecret:             secret,
		MaxSubrequests:                10,
		LiveBackFillerFinalBlockDelay: 2,
	}

	dauthnull.Register()
	auth, err := dauth.New("null://", zlog)
	require.NoError(t, err)
	metricset := dmetrics.NewSet()
	headBlockNumMetric := metricset.NewHeadBlockNumber("test-firehose-secret")
	headTimeDriftmetric := metricset.NewHeadTimeDrift("test-firehose-secret")

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

	timeout := time.After(20 * time.Second)
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()

waitReady:
	for {
		select {
		case <-timeout:
			t.Fatal("timeout waiting for t1app (with secret) to be ready")
		case <-ticker.C:
			if t1app.IsReady(ctx) {
				t.Logf("t1app (with secret) is ready")
				break waitReady
			}
		}
	}

	return t1app, substreamsEndpoint
}

func newDummyBlockchainContainer(ctx context.Context, tmpDir string, image string, additionalReaderArgs string, burst int) (testcontainers.Container, error) {
	baseReaderArgs := fmt.Sprintf("start --log-level=error --tracer=firehose --store-dir=/data --genesis-block-burst=%d --block-rate=120 --block-size=1500 --genesis-height=0 --server-addr=:9777 --with-reorgs=false --with-skipped-blocks=false", burst)
	readerArgs := baseReaderArgs
	if additionalReaderArgs != "" {
		readerArgs = baseReaderArgs + " " + additionalReaderArgs
	}

	return newDummyBlockchainContainerWithArgs(ctx, tmpDir, image, readerArgs)
}

func newDummyBlockchainContainerWithBlockRate(ctx context.Context, tmpDir string, image string, additionalReaderArgs string, burst int, blockRate int) (testcontainers.Container, error) {
	readerArgs := fmt.Sprintf("start --log-level=error --tracer=firehose --store-dir=/data --genesis-block-burst=%d --block-rate=%d --block-size=1500 --genesis-height=0 --server-addr=:9777 --with-reorgs=false --with-skipped-blocks=false", burst, blockRate)
	if additionalReaderArgs != "" {
		readerArgs = readerArgs + " " + additionalReaderArgs
	}

	return newDummyBlockchainContainerWithArgs(ctx, tmpDir, image, readerArgs)
}

func newDummyBlockchainContainerWithArgs(ctx context.Context, tmpDir string, image string, readerArgs string) (testcontainers.Container, error) {

	req := testcontainers.ContainerRequest{
		Image: image,
		Cmd: []string{
			"start",
			"reader-node",
			"merger",
			"relayer",
			"-c",
			"",
			"--common-system-shutdown-signal-delay=10s",
			"--advertise-chain-name=acme-dummy-blockchain",
			"--reader-node-path=dummy-blockchain",
			"--reader-node-arguments=" + readerArgs,
			"--advertise-block-id-encoding=hex",
		},
		Env: map[string]string{
			"DLOG": ".*=debug",
		},
		ExposedPorts: []string{"10014/tcp"},
		Mounts: testcontainers.Mounts(
			testcontainers.BindMount(tmpDir, "/app/firehose-data/storage/"),
		),
		WaitingFor: wait.ForAll(
			wait.ForListeningPort("10014/tcp"),
			wait.ForLog("serving gRPC").WithStartupTimeout(30*time.Second),
		),
	}

	fmt.Println(strings.Join(req.Cmd, " "))

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

type responses []interface{}

func blockScopedDataToString(bsd *pbsubstreamsrpcv2.BlockScopedData) string {
	if bsd.IsPartial {
		lastPartial := ""
		if bsd.IsLastPartial != nil && *bsd.IsLastPartial {
			lastPartial = "*"
		}
		idx := uint32(0)
		if bsd.PartialIndex != nil {
			idx = *bsd.PartialIndex
		}
		return fmt.Sprintf("P:%d:%d%s", bsd.Clock.Number, idx, lastPartial)
	}
	return fmt.Sprintf("F:%d", bsd.Clock.Number)
}

func (r *responses) Strings() (out []string) {
	for _, rr := range *r {
		switch rr := rr.(type) {
		case *pbsubstreamsrpcv4.Response:
			if bsds := rr.GetBlockScopedDatas(); bsds != nil {
				for _, bsd := range bsds.Items {
					out = append(out, blockScopedDataToString(bsd))
				}
			}
			if undo := rr.GetBlockUndoSignal(); undo != nil {
				out = append(out, fmt.Sprintf("U:%d", undo.LastValidBlock.Number))
			}
		case *pbsubstreamsrpcv2.BlockScopedData:
			out = append(out, blockScopedDataToString(rr))
		case *pbsubstreamsrpcv2.BlockUndoSignal:
			out = append(out, fmt.Sprintf("U:%d", rr.LastValidBlock.Number))
		}
	}
	return
}

func (r *responses) BlockScopedData() []*pbsubstreamsrpcv2.BlockScopedData {
	if r == nil {
		return nil
	}
	var blockScopedData []*pbsubstreamsrpcv2.BlockScopedData
	for _, resp := range *r {
		switch item := resp.(type) {
		case *pbsubstreamsrpcv4.Response:
			if data := item.GetBlockScopedDatas(); data != nil {
				blockScopedData = append(blockScopedData, data.Items...)
			}
		case *pbsubstreamsrpcv2.BlockScopedData:
			blockScopedData = append(blockScopedData, item)
		}
	}
	return blockScopedData
}

func (r *responses) SessionInit() *pbsubstreamsrpcv2.SessionInit {
	if r == nil {
		return nil
	}
	for _, resp := range *r {
		if response, ok := resp.(*pbsubstreamsrpcv4.Response); ok {
			if sess := response.GetSession(); sess != nil {
				return sess
			}
		}
	}
	return nil
}

func (r *responses) Undo() []*pbsubstreamsrpcv2.BlockUndoSignal {
	if r == nil {
		return nil
	}
	var blockUndoSignals []*pbsubstreamsrpcv2.BlockUndoSignal
	for _, resp := range *r {
		if response, ok := resp.(*pbsubstreamsrpcv4.Response); ok {
			if undo := response.GetBlockUndoSignal(); undo != nil {
				blockUndoSignals = append(blockUndoSignals, undo)
			}
		}
	}
	return blockUndoSignals
}

// RunRequestWithPartialBlocks is similar to RunRequest but specifically looks for partial block data
func RunRequestWithPartialBlocks(t *testing.T, req *pbsubstreamsrpcv3.Request, endpoint string, maxPartialResponses int) (responses, error) {
	var resps responses

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
	streamClient := pbsubstreamsrpcv4.NewStreamClient(conn)

	// Make the streaming call
	blockStream, err := streamClient.Blocks(ctx, req, callOpts...)
	if err != nil {
		t.Logf("Error making streaming call: %v", err)
		require.NoError(t, err)
	}
	var partialBlockDataCount int

	for {
		var response *pbsubstreamsrpcv4.Response
		response, err = blockStream.Recv()
		if err != nil {
			t.Logf("Stream ended with error: %v", err)
			if strings.Contains(err.Error(), "RST_STREAM") {
				t.Logf("Error RST_STREAM in these tests usually means that the test panicked. Make sure that you run with `DLOG=info` (or similar) to see the server-side panic")
			}
			if errors.Is(err, stream.ErrStopBlockReached) || errors.Is(err, io.EOF) {
				t.Logf("Reached stop block or EOF")
				err = nil
			}
			break
		}
		require.NotNil(t, response)

		// Validate session if received
		if sess := response.GetSession(); sess != nil {
			resps = append(resps, sess)
			t.Logf("Received session: %+v", sess)
		}

		// Accumulate BlockScopedData
		resps = append(resps, response)

		if blockDatas := response.GetBlockScopedDatas(); blockDatas != nil {
			for _, blockData := range blockDatas.Items {
				if blockData.IsPartial {
					partialBlockDataCount++
					//t.Logf("Received partial block data %d/%d", len(partialBlockDataSlice), partialResponseCount)
					// Break if we've received the desired number of partial responses
					if maxPartialResponses != 0 && partialBlockDataCount >= maxPartialResponses {
						t.Logf("Received %d partial responses - halting request", partialBlockDataCount)
						return resps, nil
					}
				}
			}
		}

		if undo := response.GetBlockUndoSignal(); undo != nil {
			//t.Logf("Accumulated BlockUndoSignal clock %d, total count: %d", undo.Clock.Number, len(blockUndoSignalSlice))
		}
	}

	return resps, err
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

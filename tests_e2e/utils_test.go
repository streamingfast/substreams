package tests_e2e

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/streamingfast/bstream/stream"
	"github.com/streamingfast/dmetering/logger"
	_ "github.com/streamingfast/dsession/local"
	"github.com/streamingfast/substreams/app"
	"github.com/streamingfast/substreams/client"
	pbsubstreamsrpcv2 "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
	pbsubstreamsrpcv3 "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v3"
	pbsubstreamsrpcv4 "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v4"
	pbtest "github.com/streamingfast/substreams/tests_e2e/partial_blocks_store/pb"
	"github.com/streamingfast/substreams/tools/devenv"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
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
	return startTier1(t, ctx, devenv.Tier1Config{
		TmpDir:               tmpDir,
		RelayerEndpoint:      relayerEndpoint(t, ctx, container),
		Tier2Endpoint:        t2Endpoint,
		MaxWorkersPerSession: 5,
		MetricsPrefix:        "test-firehose",
	}, zlog)
}

func startTier1(t *testing.T, ctx context.Context, config devenv.Tier1Config, zlog *zap.Logger) (*app.Tier1App, string) {
	t1app, endpoint, err := devenv.StartTier1(ctx, config, zlog)
	require.NoError(t, err)
	t.Logf("t1app is ready on %s", endpoint)

	return t1app, endpoint
}

func relayerEndpoint(t *testing.T, ctx context.Context, container testcontainers.Container) string {
	endpoint, err := devenv.RelayerEndpoint(ctx, container)
	require.NoError(t, err)
	t.Logf("Tier1App relayer endpoint: %s", endpoint)

	return endpoint
}

func startTier2App(t *testing.T, ctx context.Context, tmpDir string, zlog *zap.Logger, scratchSpace ...string) (out *app.Tier2App, endpoint string) {
	config := devenv.Tier2Config{TmpDir: tmpDir}
	if len(scratchSpace) > 0 {
		config.ScratchSpace = scratchSpace[0]
	}

	return startTier2(t, ctx, config, zlog)
}

// startTier2AppWithSecret starts a tier2 that requires requests to carry
// "Authorization: Bearer <secret>" — mimicking a real deployment where tier1
// must authenticate its subrequests to tier2.
func startTier2AppWithSecret(t *testing.T, ctx context.Context, tmpDir string, secret string, zlog *zap.Logger) (out *app.Tier2App, endpoint string) {
	return startTier2(t, ctx, devenv.Tier2Config{TmpDir: tmpDir, Secret: secret}, zlog)
}

func startTier2(t *testing.T, ctx context.Context, config devenv.Tier2Config, zlog *zap.Logger) (*app.Tier2App, string) {
	t2app, endpoint, err := devenv.StartTier2(ctx, config, zlog)
	require.NoError(t, err)
	t.Logf("t2app is ready on %s", endpoint)

	return t2app, endpoint
}

// startTier1AppWithSecret starts a tier1 that sends subrequests to tier2 using the
// provided secret key in the Authorization header.
func startTier1AppWithSecret(t *testing.T, ctx context.Context, tmpDir string, container testcontainers.Container, t2Endpoint string, secret string, zlog *zap.Logger) (*app.Tier1App, string) {
	return startTier1(t, ctx, devenv.Tier1Config{
		TmpDir:                        tmpDir,
		RelayerEndpoint:               relayerEndpoint(t, ctx, container),
		Tier2Endpoint:                 t2Endpoint,
		Tier2Secret:                   secret,
		MaxWorkersPerSession:          5,
		MetricsPrefix:                 "test-firehose-secret",
		LiveBackFillerFinalBlockDelay: 2,
	}, zlog)
}

func newDummyBlockchainContainer(ctx context.Context, tmpDir string, image string, additionalReaderArgs string, burst int) (testcontainers.Container, error) {
	return newDummyBlockchainContainerWithBlockRate(ctx, tmpDir, image, additionalReaderArgs, burst, 120)
}

func newDummyBlockchainContainerWithBlockRate(ctx context.Context, tmpDir string, image string, additionalReaderArgs string, burst int, blockRate int) (testcontainers.Container, error) {
	return devenv.StartDummyBlockchain(ctx, devenv.ChainConfig{
		Image:           image,
		TmpDir:          tmpDir,
		Burst:           burst,
		BlockRate:       blockRate,
		ExtraReaderArgs: additionalReaderArgs,
	})
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
	port, err := devenv.FindFreePort()
	require.NoError(t, err)

	return port
}

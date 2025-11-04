package tests_e2e

import (
	"context"
	"os"
	"testing"

	"github.com/streamingfast/logging"
	"github.com/streamingfast/substreams/manifest"
	pbsubstreamsrpcv2 "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDummyBlockchainContainer(t *testing.T) {
	ctx := context.Background()
	zlog := logging.MustCreateLoggerWithServiceName("dummy-blockchain-test")

	// Create temporary directory for volume mount
	tmpDir, err := os.MkdirTemp("", "firehose-data-")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// launch dummy blockchain container
	container, err := newDummyBlockchainContainer(ctx, tmpDir)
	require.NoError(t, err)
	defer container.Terminate(ctx)

	_, substreamsEndpoint := startTier1App(t, ctx, tmpDir, container, zlog)
	_ = startTier2App(t, ctx, tmpDir, zlog)

	pkg, err := manifest.MustNewReader("./dummy/e2e-v0.1.0.spkg").Read()
	require.NoError(t, err)

	request := &pbsubstreamsrpcv2.Request{
		StartBlockNum:   100,
		StopBlockNum:    160,
		FinalBlocksOnly: false,
		ProductionMode:  true,
		OutputModule:    "map_events",
		Modules:         pkg.Package.Modules,
	}

	// Run the request
	blockScopedDataSlice, session, err := RunRequest(t, request, substreamsEndpoint)
	if err != nil {

		// Let's try to get container logs to debug
		logs, logErr := container.Logs(ctx)
		if logErr == nil {
			defer logs.Close()
			buf := make([]byte, 4096)
			n, _ := logs.Read(buf)
			t.Logf("Container logs: %s", string(buf[:n]))
		}

		t.Errorf("Error running request: %v", err)
	}

	require.NotNil(t, session, "Should have received at least one session")
	assert.Equal(t, 55, len(blockScopedDataSlice), "Should have received all expected blocks")
	assert.Equal(t, 159, int(blockScopedDataSlice[len(blockScopedDataSlice)-1].Clock.Number), "Should end on expected block")

}

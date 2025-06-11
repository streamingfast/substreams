package integration

import (
	"context"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/streamingfast/substreams/manifest"
	"github.com/streamingfast/substreams/metering"
	"github.com/streamingfast/substreams/metrics"
	"github.com/streamingfast/substreams/orchestrator/work"
	pbsubstreamstest "github.com/streamingfast/substreams/pb/sf/substreams/v1/test"
	"github.com/streamingfast/substreams/reqctx"
	_ "github.com/streamingfast/substreams/wasm/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func Test_JSRuntime_SimpleMap(t *testing.T) {
	manifest.TestUseSimpleHash = true

	testTempDir := t.TempDir()

	ctx := context.Background()
	ctx = metering.WithMetricsSender(ctx)
	ctx = reqctx.WithReqStats(ctx, metrics.NewReqStats(&metrics.Config{}, zlog))

	manifestPath := "./testdata/js_substreams/substreams_eth_usdt_js.spkg"
	pkg := manifest.TestReadManifest(t, manifestPath)

	const (
		moduleName        = "test_map"
		startBlock uint64 = 10
		stage             = 0
	)

	ctx = reqctx.WithRequest(ctx, &reqctx.RequestDetails{
		Modules:      pkg.Modules,
		OutputModule: moduleName,
	})

	ctx = reqctx.WithTier2RequestParameters(ctx, reqctx.Tier2RequestParameters{
		BlockType:            "sf.substreams.v1.test.Block",
		FirstStreamableBlock: 10,
		StateBundleSize:      10,
		StateStoreURL:        filepath.Join(testTempDir, "test.store"),
		MergedBlockStoreURL:  "some_merged_block_store_url",
		MeteringConfig:       "some_metering_config",
		StateStoreDefaultTag: "tag",
	})

	responseCollector := newResponseCollector(ctx)

	newBlockGenerator := func(start, stop uint64) TestBlockGenerator {
		return &LinearBlockGenerator{startBlock: start, inclusiveStopBlock: stop}
	}

	request := work.NewRequest(ctx, reqctx.Details(ctx), stage, startBlock)
	require.NoError(t, request.Validate())
	require.NoError(
		t,
		processInternalRequest(t, ctx, request, nil, newBlockGenerator, responseCollector, nil, testTempDir),
	)

	jsHex := hex.EncodeToString([]byte(moduleName))
	outputRel := fmt.Sprintf("%s/outputs/0000000010-0000000020.output", jsHex)

	withZST := func(s []string) []string {
		res := make([]string, len(s))
		for i, v := range s {
			res[i] = fmt.Sprintf("%s.zst", v)
		}
		return res
	}

	assertFiles(t, testTempDir, false, withZST([]string{outputRel})...)
}

func Test_JSRuntime_ClockIntrinsic(t *testing.T) {
	manifest.TestUseSimpleHash = true

	testTempDir := t.TempDir()
	ctx := context.Background()
	ctx = metering.WithMetricsSender(ctx)
	ctx = reqctx.WithReqStats(ctx, metrics.NewReqStats(&metrics.Config{}, zlog))

	manifestPath := "./testdata/js_substreams/substreams_eth_usdt_js.spkg"
	pkg := manifest.TestReadManifest(t, manifestPath)

	const (
		moduleName        = "test_map_clock"
		startBlock uint64 = 10
		stage             = 0
	)

	ctx = reqctx.WithRequest(ctx, &reqctx.RequestDetails{
		Modules:      pkg.Modules,
		OutputModule: moduleName,
	})

	ctx = reqctx.WithTier2RequestParameters(ctx, reqctx.Tier2RequestParameters{
		BlockType:            "sf.substreams.v1.test.Block",
		FirstStreamableBlock: 10,
		StateBundleSize:      10,
		StateStoreURL:        filepath.Join(testTempDir, "test.store"),
		MeteringConfig:       "some_metering_config",
		MergedBlockStoreURL:  "some_merged_block_store_url",
		StateStoreDefaultTag: "tag",
	})

	responseCollector := newResponseCollector(ctx)

	newBlockGenerator := func(start, stop uint64) TestBlockGenerator {
		return &LinearBlockGenerator{startBlock: start, inclusiveStopBlock: stop}
	}

	request := work.NewRequest(ctx, reqctx.Details(ctx), stage, startBlock)
	require.NoError(t, request.Validate())
	require.NoError(
		t,
		processInternalRequest(t, ctx, request, nil, newBlockGenerator, responseCollector, nil, testTempDir),
	)

	jsHex := hex.EncodeToString([]byte(moduleName))
	outputFile := fmt.Sprintf("%s/outputs/0000000010-0000000020.output", jsHex)

	out, err := readOutputFile(ctx, filepath.Join(testTempDir, "test.store", "tag"), outputFile)
	require.NoError(t, err)

	var result pbsubstreamstest.MapResult

	item, found := out.Kv["block-10"]
	require.True(t, found, "missing expected key block-10")

	require.NoError(t, proto.Unmarshal(item.Payload, &result))
	assert.Equal(t, uint64(10), result.BlockNumber)
	assert.Equal(t, "block-10", result.BlockHash)

}

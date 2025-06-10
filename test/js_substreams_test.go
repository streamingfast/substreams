package integration

import (
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"path/filepath"
	"testing"

	"github.com/streamingfast/dstore"
	"github.com/streamingfast/substreams/manifest"
	"github.com/streamingfast/substreams/metering"
	"github.com/streamingfast/substreams/metrics"
	"github.com/streamingfast/substreams/orchestrator/work"
	"github.com/streamingfast/substreams/reqctx"
	_ "github.com/streamingfast/substreams/wasm/v8"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	pbsubstreamstest "github.com/streamingfast/substreams/pb/sf/substreams/v1/test"
	pboutput "github.com/streamingfast/substreams/storage/execout/pb"
)

func runJSSimpleMap(t *testing.T, moduleName string, startBlock uint64) (testTempDir string, out *pboutput.Map, rel string) {
	manifest.TestUseSimpleHash = true
	testTempDir = t.TempDir()

	ctx := context.Background()
	ctx = metering.WithMetricsSender(ctx)
	ctx = reqctx.WithReqStats(ctx, metrics.NewReqStats(&metrics.Config{}, zlog))

	pkg := manifest.TestReadManifest(t, "./testdata/js_substreams/substreams_eth_usdt_js.spkg")
	ctx = reqctx.WithRequest(ctx, &reqctx.RequestDetails{
		Modules:      pkg.Modules,
		OutputModule: moduleName,
	})

	ctx = reqctx.WithTier2RequestParameters(ctx, reqctx.Tier2RequestParameters{
		BlockType:            "sf.substreams.v1.test.Block",
		FirstStreamableBlock: startBlock,
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

	request := work.NewRequest(ctx, reqctx.Details(ctx), 0, startBlock)
	require.NoError(t, request.Validate())
	require.NoError(
		t,
		processInternalRequest(t, ctx, request, nil, newBlockGenerator, responseCollector, nil, testTempDir),
	)

	jsHex := hex.EncodeToString([]byte(moduleName))
	rel = fmt.Sprintf("%s/outputs/%010d-%010d.output", jsHex, startBlock, startBlock+10)

	storeRoot := filepath.Join(testTempDir, "test.store", "tag")
	s, err := dstore.NewStore(storeRoot, "zst", "zstd", false)
	require.NoError(t, err)

	rdr, err := s.OpenObject(context.Background(), rel)
	require.NoError(t, err)

	data, err := io.ReadAll(rdr)
	require.NoError(t, err)

	out = &pboutput.Map{}
	require.NoError(t, out.UnmarshalFast(data))
	return
}

func withZST(paths []string) []string {
	res := make([]string, len(paths))
	for i, p := range paths {
		res[i] = p + ".zst"
	}
	return res
}

func Test_JSRuntime_SimpleMap(t *testing.T) {
	testTempDir, _, rel := runJSSimpleMap(t, "test_map", 10)
	assertFiles(t, testTempDir, false, withZST([]string{rel})...)
}

// Test_JSRuntime_SimpleMap_Input verifies that the JS map handler actually received the block number it was passed.
func Test_JSRuntime_SimpleMap_Input(t *testing.T) {
	_, out, _ := runJSSimpleMap(t, "test_map", 10)

	for _, kv := range out.Kv {
		var res pbsubstreamstest.MapResult
		require.NoError(t, proto.Unmarshal(kv.Payload, &res), "unmarshal payload for block %d", kv.BlockNum)
		require.Equal(t, kv.BlockNum, res.BlockNumber, "saw wrong blockNumber")
	}
}

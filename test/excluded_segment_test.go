package integration

import (
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"path/filepath"
	"testing"

	"github.com/RoaringBitmap/roaring/roaring64"
	"github.com/streamingfast/dstore"
	"github.com/streamingfast/substreams/manifest"
	"github.com/streamingfast/substreams/metering"
	"github.com/streamingfast/substreams/metrics"
	"github.com/streamingfast/substreams/orchestrator/work"
	pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/reqctx"
	"github.com/streamingfast/substreams/storage/store/marshaller"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

// excludedSegments are the segments whose cached test_index file has no "even" key, so that
// every module filtered on "even" is excluded from them.
var excludedSegments = [][2]uint64{{20, 30}, {30, 40}}

// excludedSegmentPackage returns the simple_substreams package with two more modules built on
// existing handlers and filtered on test_index "even": store_f (stage 0) and map_f (stage 1),
// which reads store_f.
func excludedSegmentPackage(t *testing.T) *pbsubstreams.Package {
	t.Helper()
	manifest.TestUseSimpleHash = true

	pkg := manifest.TestReadManifest(t, "./testdata/simple_substreams/substreams-test-v0.1.0.spkg")
	filter := &pbsubstreams.Module_BlockFilter{
		Module: "test_index",
		Query:  &pbsubstreams.Module_BlockFilter_QueryString{QueryString: "even"},
	}

	var storeF, mapF *pbsubstreams.Module
	for _, mod := range pkg.Modules.Modules {
		switch mod.Name {
		case "setup_test_store_set_i64":
			storeF = proto.Clone(mod).(*pbsubstreams.Module)
		case "assert_test_store_set_i64":
			mapF = proto.Clone(mod).(*pbsubstreams.Module)
		}
	}
	require.NotNil(t, storeF)
	require.NotNil(t, mapF)

	storeF.Name = "store_f"
	storeF.BlockFilter = filter
	mapF.Name = "map_f"
	mapF.BlockFilter = filter
	for _, input := range mapF.Inputs {
		if s := input.GetStore(); s != nil {
			s.ModuleName = "store_f"
		}
	}
	pkg.Modules.Modules = append(pkg.Modules.Modules, storeF, mapF)
	return pkg
}

func stateDir(tempDir string) string {
	return filepath.Join(tempDir, "test.store", "tag")
}

func writeExcludingIndexFiles(t *testing.T, tempDir string) {
	t.Helper()
	for _, seg := range excludedSegments {
		odd := roaring64.New()
		for blk := seg[0]; blk < seg[1]; blk++ {
			if blk%2 == 1 {
				odd.Add(blk)
			}
		}
		filename := fmt.Sprintf("%s/index/%010d-%010d.index", hex.EncodeToString([]byte("test_index")), seg[0], seg[1])
		require.NoError(t, createIndexFile(context.Background(), stateDir(tempDir), filename, map[string]*roaring64.Bitmap{"odd": odd}))
	}
}

// runExcludedSegmentsRequest runs map_f over [1, 60) in production mode, with the cached
// test_index files of excludedSegments written before the request starts.
func runExcludedSegmentsRequest(t *testing.T) *testRun {
	t.Helper()
	run := newTestRun(t, 1, 60, 60, 0, "map_f", "./testdata/simple_substreams/substreams-test-v0.1.0.spkg")
	run.Package = excludedSegmentPackage(t)
	run.ProductionMode = true
	run.ParallelSubrequests = 5
	run.PreWork = func(t *testing.T, run *testRun, _ work.WorkerPoolFactory) {
		writeExcludingIndexFiles(t, run.TempDir)
	}
	require.NoError(t, run.Run(t, "excluded_segments"))
	return run
}

// runExcludedSegmentCopy runs map_f over [1, 20) through tier1, then runs map_f's stage-1 job
// for segment [20, 30) directly on tier2, with the segment excluded and no store_f snapshot at
// 30 yet: the job has to write that snapshot without running store_f. It returns the temp dir.
func runExcludedSegmentCopy(t *testing.T) string {
	t.Helper()
	run := newTestRun(t, 1, 20, 20, 0, "map_f", "./testdata/simple_substreams/substreams-test-v0.1.0.spkg")
	run.Package = excludedSegmentPackage(t)
	run.ProductionMode = true
	require.NoError(t, run.Run(t, "excluded_segment_copy"))

	storeFStates := filepath.Join(stateDir(run.TempDir), hex.EncodeToString([]byte("store_f")), "states")
	require.FileExists(t, filepath.Join(storeFStates, "0000000020-0000000001.kv.zst"), "the tier1 run must leave store_f's snapshot at 20")
	require.NoFileExists(t, filepath.Join(storeFStates, "0000000030-0000000001.kv.zst"))

	writeExcludingIndexFiles(t, run.TempDir)

	ctx := context.Background()
	ctx = reqctx.WithLogger(ctx, zlog)
	ctx = metering.WithMetricsSender(ctx)
	ctx = reqctx.WithRequest(ctx, &reqctx.RequestDetails{Modules: run.Package.Modules, OutputModule: "map_f"})
	ctx = reqctx.WithReqStats(ctx, metrics.NewReqStats(&metrics.Config{}, nil, nil, zlog))
	ctx = reqctx.WithTier2RequestParameters(ctx, reqctx.Tier2RequestParameters{
		BlockType:            "sf.substreams.v1.test.Block",
		StateBundleSize:      10,
		StateStoreURL:        filepath.Join(run.TempDir, "test.store"),
		MeteringConfig:       "some_metering_config",
		MergedBlockStoreURL:  "some_merged_block_store_url",
		StateStoreDefaultTag: "tag",
	})

	newBlockGenerator := func(startBlock uint64, inclusiveStopBlock uint64) TestBlockGenerator {
		return &LinearBlockGenerator{startBlock: startBlock, inclusiveStopBlock: inclusiveStopBlock}
	}
	request := work.NewRequest(ctx, reqctx.Details(ctx), 1, 20, false)
	require.NoError(t, request.Validate())
	require.NoError(t, processInternalRequest(t, ctx, request, nil, newBlockGenerator, newResponseCollector(ctx, "map_f", 0, 0), nil, run.TempDir))

	return run.TempDir
}

func readStoreFile(t *testing.T, tempDir, module, filename string) map[string][]byte {
	t.Helper()
	s, err := dstore.NewStore(filepath.Join(stateDir(tempDir), hex.EncodeToString([]byte(module)), "states"), "zst", "zstd", false)
	require.NoError(t, err)
	reader, err := s.OpenObject(context.Background(), filename)
	require.NoError(t, err)
	defer reader.Close()
	content, err := io.ReadAll(reader)
	require.NoError(t, err)
	data, _, err := marshaller.Default().Unmarshal(content)
	require.NoError(t, err)
	return data.Kv
}

func mapOutputBlocks(responses []*pbsubstreamsrpc.Response) (out []uint64) {
	for _, resp := range responses {
		data := resp.GetBlockScopedData()
		if data == nil || data.Output == nil || data.Output.MapOutput == nil || len(data.Output.MapOutput.Value) == 0 {
			continue
		}
		out = append(out, data.Clock.Number)
	}
	return out
}

func TestExcludedSegments_Tier1(t *testing.T) {
	run := runExcludedSegmentsRequest(t)

	var expected []uint64
	for blk := uint64(2); blk < 60; blk += 2 {
		if blk >= 20 && blk < 40 {
			continue
		}
		expected = append(expected, blk)
	}
	assert.Equal(t, expected, mapOutputBlocks(run.Responses))

	at20 := readStoreFile(t, run.TempDir, "store_f", "0000000020-0000000001.kv")
	assert.NotEmpty(t, at20)
	assert.Equal(t, at20, readStoreFile(t, run.TempDir, "store_f", "0000000030-0000000001.kv"))
	assert.Equal(t, at20, readStoreFile(t, run.TempDir, "store_f", "0000000040-0000000001.kv"))
}

func TestExcludedSegment_EarlierStageStoreCopied(t *testing.T) {
	tempDir := runExcludedSegmentCopy(t)

	at20 := readStoreFile(t, tempDir, "store_f", "0000000020-0000000001.kv")
	assert.NotEmpty(t, at20)
	assert.Equal(t, at20, readStoreFile(t, tempDir, "store_f", "0000000030-0000000001.kv"))
}

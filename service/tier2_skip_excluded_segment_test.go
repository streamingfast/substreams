package service

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/RoaringBitmap/roaring/roaring64"
	"github.com/streamingfast/dmetering"
	"github.com/streamingfast/dstore"
	"github.com/streamingfast/substreams"
	"github.com/streamingfast/substreams/block"
	"github.com/streamingfast/substreams/metering"
	"github.com/streamingfast/substreams/metrics"
	pbssinternal "github.com/streamingfast/substreams/pb/sf/substreams/intern/v2"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/pipeline/exec"
	"github.com/streamingfast/substreams/reqctx"
	"github.com/streamingfast/substreams/storage/execout"
	"github.com/streamingfast/substreams/storage/index"
	"github.com/streamingfast/substreams/storage/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type noopEmitter struct{}

func (noopEmitter) Emit(context.Context, dmetering.Event) {}
func (noopEmitter) Shutdown(error)                        {}

// TestProcessRange_SkipsSegmentExcludedByCachedBlockIndex exercises the tier2 fast path: a
// map module whose block filter resolves, from an already-cached index, to an empty bitmap
// for this entire segment. The WASM binary is deliberately not real WASM: if the optimization
// regresses and tier2 ever builds module executors for this segment, compiling it fails and
// the test fails loudly. The test then checks that processRange still produced the same files
// a normal run would have left behind for a segment with zero matching blocks (an empty but
// present exec-out, readable back as a cache hit by a second execution-plan computation), and
// that it still reported completion like any other segment.
func TestProcessRange_SkipsSegmentExcludedByCachedBlockIndex(t *testing.T) {
	logger := zap.NewNop()

	modules := []*pbsubstreams.Module{
		{
			Name: "idx_a",
			Kind: &pbsubstreams.Module_KindBlockIndex_{
				KindBlockIndex: &pbsubstreams.Module_KindBlockIndex{OutputType: "proto:sf.substreams.index.v1.Keys"},
			},
			Inputs: []*pbsubstreams.Module_Input{
				{Input: &pbsubstreams.Module_Input_Source_{Source: &pbsubstreams.Module_Input_Source{Type: "sf.substreams.v1.test.Block"}}},
			},
		},
		{
			Name: "map_a",
			Kind: &pbsubstreams.Module_KindMap_{},
			Inputs: []*pbsubstreams.Module_Input{
				{Input: &pbsubstreams.Module_Input_Source_{Source: &pbsubstreams.Module_Input_Source{Type: "sf.substreams.v1.test.Block"}}},
			},
			BlockFilter: &pbsubstreams.Module_BlockFilter{
				Module: "idx_a",
				Query:  &pbsubstreams.Module_BlockFilter_QueryString{QueryString: "never_present_key"},
			},
		},
	}

	modulesPkg := &pbsubstreams.Modules{
		Modules: modules,
		Binaries: []*pbsubstreams.Binary{
			{Type: "not-a-real-wasm-binary-type", Content: []byte("garbage: must never be compiled when the segment is skipped")},
		},
	}

	execGraph, err := exec.NewOutputModuleGraph("map_a", true, modulesPkg, 0, false)
	require.NoError(t, err)

	const stage = uint32(0)
	const startBlock = uint64(0)
	const stopBlock = uint64(10)

	tmpDir := t.TempDir()
	stateStoreURL := filepath.Join(tmpDir, "state")

	baseStateStore, err := dstore.NewStore(stateStoreURL, "zst", "zstd", false)
	require.NoError(t, err)
	cacheStore, err := baseStateStore.SubStore("tag")
	require.NoError(t, err)

	// Pre-populate idx_a's cached index for this exact segment with a bitmap stored under a
	// different key than map_a's filter query: the bitmap map_a resolves against that index is
	// therefore empty, proving (from the cache alone) that no block in the segment can match.
	indexConfigs, err := index.NewConfigs(cacheStore, execGraph.UsedIndexesModulesUpToStage(int(stage)), execGraph.ModuleHashes(), 0, logger)
	require.NoError(t, err)
	indexFile := indexConfigs.ConfigMap["idx_a"].NewFile(&block.Range{StartBlock: startBlock, ExclusiveEndBlock: stopBlock})
	other := roaring64.New()
	other.Add(5)
	indexFile.Set(map[string]*roaring64.Bitmap{"some_other_key": other})
	require.NoError(t, indexFile.Save(context.Background()))

	svc := &Tier2Service{logger: zlog}

	ctx := context.Background()
	ctx = reqctx.WithLogger(ctx, zlog)
	ctx = metering.WithMetricsSender(ctx)
	ctx = dmetering.WithBytesMeter(ctx)
	ctx = reqctx.WithReqStats(ctx, metrics.NewReqStats(&metrics.Config{}, nil, nil, zlog))
	ctx = reqctx.WithEmitter(ctx, noopEmitter{})

	request := &pbssinternal.ProcessRangeRequest{
		Modules:              modulesPkg,
		OutputModule:         "map_a",
		Stage:                stage,
		SegmentSize:          stopBlock - startBlock,
		SegmentNumber:        0,
		FirstStreamableBlock: 0,
		BlockType:            "sf.substreams.v1.test.Block",
		MergedBlocksStore:    filepath.Join(tmpDir, "blocks"),
		StateStore:           stateStoreURL,
		StateStoreDefaultTag: "tag",
	}

	var responses []*pbssinternal.ProcessRangeResponse
	respFunc := func(r substreams.ResponseFromAnyTier) error {
		if resp, ok := r.(*pbssinternal.ProcessRangeResponse); ok {
			responses = append(responses, resp)
		}
		return nil
	}

	require.NoError(t, svc.processRange(ctx, request, respFunc))

	var completed *pbssinternal.Completed
	for _, r := range responses {
		if c := r.GetCompleted(); c != nil {
			completed = c
		}
	}
	require.NotNil(t, completed, "a skipped segment must still report completion, like any other segment")
	assert.Equal(t, uint64(0), completed.ProcessedBlocks)

	// A fresh execution-plan computation over the same files must now see map_a's exec-out
	// as an existing, loadable cache hit -- exactly what a normal (non-skipped) run would have
	// left behind for a segment with zero matching blocks -- and report the segment skippable.
	execoutConfigs, err := execout.NewConfigs(cacheStore, execGraph.UsedModulesUpToStage(int(stage)), execGraph.ModuleHashes(), request.SegmentSize, request.FirstStreamableBlock, logger)
	require.NoError(t, err)
	storeConfigs, err := store.NewConfigMap(cacheStore, nil, execGraph.Stores(), execGraph.ModuleHashes(), request.FirstStreamableBlock, 0, "", "")
	require.NoError(t, err)

	plan, err := GetExecutionPlan(context.Background(), logger, execGraph, stage, startBlock, stopBlock, "map_a", execoutConfigs, indexConfigs, storeConfigs)
	require.NoError(t, err)
	require.NotNil(t, plan)
	assert.True(t, plan.Skippable, "map_a's exec-out must be a cache hit after the segment was skipped")
	assert.Contains(t, plan.ExistingExecOuts, "map_a")
}

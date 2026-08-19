package service

import (
	"context"
	"fmt"
	"testing"

	"github.com/streamingfast/dstore"
	"github.com/streamingfast/substreams/block"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/pipeline/exec"
	"github.com/streamingfast/substreams/service/config"
	"github.com/streamingfast/substreams/storage/execout"
	"github.com/streamingfast/substreams/storage/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestPickEvenly(t *testing.T) {
	in := []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}

	assert.Equal(t, in, pickEvenly(in, 20))
	assert.Equal(t, in, pickEvenly(in, 10))
	assert.Equal(t, []int{0, 2, 4, 6, 8}, pickEvenly(in, 5))
	assert.Equal(t, []int{0, 3, 6}, pickEvenly(in, 3))
	assert.Equal(t, []int{0}, pickEvenly(in, 1))
}

const testSegmentSize = 1000

// TestPlanSampling_NoStores: without stores every segment can run on its own, so the sample
// is spread over the whole range.
func TestPlanSampling_NoStores(t *testing.T) {
	svc, execGraph, baseStore := newEstimateTestService(t, false)
	execoutConfigs, storeConfigs := newEstimateTestConfigs(t, execGraph, baseStore)

	plan, err := svc.planSampling(context.Background(), execGraph, execoutConfigs, storeConfigs, 0, 1_000_000, 1)
	require.NoError(t, err)

	require.Len(t, plan.segments, 10)
	assert.Equal(t, uint64(0), plan.segments[0].rng.StartBlock)
	assert.Equal(t, uint64(100_000), plan.segments[1].rng.StartBlock)
	assert.Equal(t, uint64(900_000), plan.segments[9].rng.StartBlock)
	assert.Equal(t, uint64(10_000), plan.blockCount())
}

// TestPlanSampling_StoresCoverRange: every segment can run on its own when the stores are
// snapshotted at each of its boundaries, so the sample is spread over the whole range.
func TestPlanSampling_StoresCoverRange(t *testing.T) {
	svc, execGraph, baseStore := newEstimateTestService(t, true)
	writeStoreSnapshots(t, execGraph, baseStore, 1_000_000)

	execoutConfigs, storeConfigs := newEstimateTestConfigs(t, execGraph, baseStore)

	plan, err := svc.planSampling(context.Background(), execGraph, execoutConfigs, storeConfigs, 0, 1_000_000, 1)
	require.NoError(t, err)

	require.Len(t, plan.segments, 10)
	assert.Equal(t, uint64(0), plan.segments[0].rng.StartBlock)
	assert.Equal(t, uint64(900_000), plan.segments[9].rng.StartBlock)
}

// TestPlanSampling_StoresCoverPartOfRange: the stores stop partway, so the request is refused
// and told which part of the range it could ask for instead.
func TestPlanSampling_StoresCoverPartOfRange(t *testing.T) {
	svc, execGraph, baseStore := newEstimateTestService(t, true)
	writeStoreSnapshots(t, execGraph, baseStore, 500_000)

	execoutConfigs, storeConfigs := newEstimateTestConfigs(t, execGraph, baseStore)

	_, err := svc.planSampling(context.Background(), execGraph, execoutConfigs, storeConfigs, 0, 1_000_000, 1)
	require.Error(t, err)
	// the snapshot ending at 500000 makes the segment that starts there runnable too
	assert.Contains(t, err.Error(), "only covers [0, 501000)")
	assert.Contains(t, err.Error(), "Estimate [0, 501000) instead")
}

// TestPlanSampling_StoresNotCached: with nothing snapshotted, only the segment the stores
// start on can run, which is not a range worth estimating.
func TestPlanSampling_StoresNotCached(t *testing.T) {
	svc, execGraph, baseStore := newEstimateTestService(t, true)
	execoutConfigs, storeConfigs := newEstimateTestConfigs(t, execGraph, baseStore)

	_, err := svc.planSampling(context.Background(), execGraph, execoutConfigs, storeConfigs, 0, 1_000_000, 1)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "only covers [0, 1000)")
}

// TestPlanSampling_StoresStartAboveRange: no segment of the range can run at all.
func TestPlanSampling_StoresStartAboveRange(t *testing.T) {
	svc, execGraph, baseStore := newEstimateTestService(t, true, setInitialBlock("store_a", 500_000))
	execoutConfigs, storeConfigs := newEstimateTestConfigs(t, execGraph, baseStore)

	_, err := svc.planSampling(context.Background(), execGraph, execoutConfigs, storeConfigs, 600_000, 1_000_000, 1)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "holds no store state")
}

// TestPlanSampling_AlreadyCachedOutputs: segments whose output file is already there are
// flagged, so that no job gets run for them.
func TestPlanSampling_AlreadyCachedOutputs(t *testing.T) {
	svc, execGraph, baseStore := newEstimateTestService(t, false)

	outputHash := execGraph.ModuleHashes()["map_out"]
	baseStore.SetFile(fmt.Sprintf("%s/outputs/%010d-%010d.output", outputHash, 100_000, 101_000), []byte("data"))

	execoutConfigs, storeConfigs := newEstimateTestConfigs(t, execGraph, baseStore)

	plan, err := svc.planSampling(context.Background(), execGraph, execoutConfigs, storeConfigs, 0, 1_000_000, 1)
	require.NoError(t, err)

	require.Len(t, plan.segments, 10)
	assert.False(t, plan.segments[0].fromCache)
	assert.True(t, plan.segments[1].fromCache, "segment [100000, 101000) is in the output cache")
}

func TestPlanSampling_RangeTooSmall(t *testing.T) {
	svc, execGraph, baseStore := newEstimateTestService(t, false)
	execoutConfigs, storeConfigs := newEstimateTestConfigs(t, execGraph, baseStore)

	_, err := svc.planSampling(context.Background(), execGraph, execoutConfigs, storeConfigs, 0, 500, 1)
	require.Error(t, err)
}

// newEstimateTestService builds a service over a `map_out` module, optionally fed by a
// `store_a` store. The `tweak` functions get to change the modules (initial blocks, ...)
// before the graph is built from them.
func newEstimateTestService(t *testing.T, withStore bool, tweaks ...func(modules []*pbsubstreams.Module)) (*Tier1Service, *exec.Graph, *dstore.MockStore) {
	t.Helper()

	modules := []*pbsubstreams.Module{{
		Name: "map_out",
		Kind: &pbsubstreams.Module_KindMap_{},
		Inputs: []*pbsubstreams.Module_Input{{
			Input: &pbsubstreams.Module_Input_Source_{
				Source: &pbsubstreams.Module_Input_Source{Type: "test.Block"},
			},
		}},
	}}

	if withStore {
		modules = append(modules, &pbsubstreams.Module{
			Name: "store_a",
			Kind: &pbsubstreams.Module_KindStore_{
				KindStore: &pbsubstreams.Module_KindStore{UpdatePolicy: pbsubstreams.Module_KindStore_UPDATE_POLICY_SET, ValueType: "string"},
			},
			Inputs: []*pbsubstreams.Module_Input{{
				Input: &pbsubstreams.Module_Input_Source_{
					Source: &pbsubstreams.Module_Input_Source{Type: "test.Block"},
				},
			}},
		})
		modules[0].Inputs = append(modules[0].Inputs, &pbsubstreams.Module_Input{
			Input: &pbsubstreams.Module_Input_Store_{
				Store: &pbsubstreams.Module_Input_Store{ModuleName: "store_a"},
			},
		})
	}

	for _, tweak := range tweaks {
		tweak(modules)
	}

	execGraph, err := exec.NewOutputModuleGraph("map_out", true, &pbsubstreams.Modules{
		Modules:  modules,
		Binaries: []*pbsubstreams.Binary{{Type: "test", Content: []byte("some-fake-binary-data")}},
	}, 0)
	require.NoError(t, err)

	svc := &Tier1Service{
		logger:        zap.NewNop(),
		runtimeConfig: config.RuntimeConfig{SegmentSize: testSegmentSize},
	}
	return svc, execGraph, dstore.NewMockStore(nil)
}

// writeStoreSnapshots lays down a full KV snapshot at every segment boundary up to
// `upToBlock`, which is what makes those segments runnable on their own.
func writeStoreSnapshots(t *testing.T, execGraph *exec.Graph, baseStore *dstore.MockStore, upToBlock uint64) {
	t.Helper()

	storeHash := execGraph.ModuleHashes()["store_a"]
	for endBlock := uint64(testSegmentSize); endBlock <= upToBlock; endBlock += testSegmentSize {
		baseStore.SetFile(fmt.Sprintf("%s/states/%010d-%010d.kv", storeHash, endBlock, 0), []byte("kv"))
	}
}

func newEstimateTestConfigs(t *testing.T, execGraph *exec.Graph, baseStore dstore.Store) (*execout.Configs, store.ConfigMap) {
	t.Helper()

	execoutConfigs, err := execout.NewConfigs(baseStore, execGraph.UsedModules(), execGraph.ModuleHashes(), testSegmentSize, 0, zap.NewNop())
	require.NoError(t, err)

	storeConfigs, err := store.NewConfigMap(baseStore, nil, execGraph.Stores(), execGraph.ModuleHashes(), 0, 0, t.TempDir(), "memory")
	require.NoError(t, err)

	return execoutConfigs, storeConfigs
}

func setInitialBlock(moduleName string, initialBlock uint64) func([]*pbsubstreams.Module) {
	return func(modules []*pbsubstreams.Module) {
		for _, module := range modules {
			if module.Name == moduleName {
				module.InitialBlock = initialBlock
			}
		}
	}
}

func TestExtrapolateSamples(t *testing.T) {
	// a segment the module ran on every block of, emitting once per block
	sampled := func(startBlock, bytes uint64) *sampleSegment {
		return &sampleSegment{rng: block.NewRange(startBlock, startBlock+1000), uncompressedSize: bytes, messageCount: 1000, processedBlocks: 1000}
	}
	// payload rate straight through, one message per block, so the spans and the averaging are
	// what is measured here
	payloadOnly := func(seg *sampleSegment) (float64, float64) {
		return float64(seg.uncompressedSize) / float64(seg.rng.Size()), float64(seg.messageCount) / float64(seg.rng.Size())
	}

	tests := []struct {
		name       string
		startBlock uint64
		stopBlock  uint64
		segments   []*sampleSegment
		expect     []sampleSpan
	}{
		{
			name:     "no sample",
			segments: nil,
			expect:   []sampleSpan{},
		},
		{
			// a lone sample stands for the whole range at its own rate
			name:       "single sample",
			startBlock: 5_000_000,
			stopBlock:  10_000_000,
			segments:   []*sampleSegment{sampled(5_000_000, 2_000)},
			expect:     []sampleSpan{{startBlock: 5_000_000, blocks: 5_000_000, bytes: 10_000_000, messages: 5_000_000, processedBlocks: 5_000_000}},
		},
		{
			// rates are 1, 2 and 3 bytes per block: a span between two samples averages its
			// two ends, the last one keeps the rate of the sample that opens it
			name:       "averages between adjacent samples",
			startBlock: 5_000_000,
			stopBlock:  20_000_000,
			segments:   []*sampleSegment{sampled(5_000_000, 1_000), sampled(10_000_000, 2_000), sampled(15_000_000, 3_000)},
			expect: []sampleSpan{
				{startBlock: 5_000_000, blocks: 5_000_000, bytes: 7_500_000, messages: 5_000_000, processedBlocks: 5_000_000},
				{startBlock: 10_000_000, blocks: 5_000_000, bytes: 12_500_000, messages: 5_000_000, processedBlocks: 5_000_000},
				{startBlock: 15_000_000, blocks: 5_000_000, bytes: 15_000_000, messages: 5_000_000, processedBlocks: 5_000_000},
			},
		},
		{
			// sampling starts on a segment boundary, the blocks below it are part of the range
			name:       "first span reaches down to the start block",
			startBlock: 4_999_500,
			stopBlock:  10_000_000,
			segments:   []*sampleSegment{sampled(5_000_000, 1_000)},
			expect:     []sampleSpan{{startBlock: 4_999_500, blocks: 5_000_500, bytes: 5_000_500, messages: 5_000_500, processedBlocks: 5_000_500}},
		},
		{
			// a contiguous sample: every span but the last is one segment wide
			name:       "contiguous samples",
			startBlock: 1_000,
			stopBlock:  1_000_000,
			segments:   []*sampleSegment{sampled(1_000, 500), sampled(2_000, 1_500)},
			expect: []sampleSpan{
				{startBlock: 1_000, blocks: 1_000, bytes: 1_000, messages: 1_000, processedBlocks: 1_000},
				{startBlock: 2_000, blocks: 998_000, bytes: 1_497_000, messages: 998_000, processedBlocks: 998_000},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			spans := extrapolateSamples(test.segments, test.startBlock, test.stopBlock, payloadOnly)
			assert.Equal(t, test.expect, spans)

			// the spans partition the range: they add up to it exactly
			var covered uint64
			for _, span := range spans {
				covered += span.blocks
			}
			if len(spans) != 0 {
				assert.Equal(t, test.stopBlock-test.startBlock, covered)
			}
		})
	}
}

// TestExtrapolateSamples_SparseModule: a module gated by a block index produces output on a
// fraction of the blocks it covers, so a real request sends that many messages and pays the
// framing that many times — not once per block of the range.
func TestExtrapolateSamples_SparseModule(t *testing.T) {
	framing := newEgressFraming(&pbsubstreams.Module{
		Name:   "map_out",
		Output: &pbsubstreams.Module_Output{Type: "proto:sf.substreams.v1.test.MapResult"},
	}, 1_000_000)

	// one segment of 1000 blocks holding 10 messages of 100 payload bytes each
	segments := []*sampleSegment{{rng: block.NewRange(0, 1000), uncompressedSize: 1_000, messageCount: 10}}

	spans := extrapolateSamples(segments, 0, 1_000_000, framing.ratesOf)
	require.Len(t, spans, 1)

	// 1% of blocks carry a message, so a million blocks send ten thousand of them
	assert.Equal(t, uint64(10_000), spans[0].messages)
	assert.Equal(t, uint64(10_000)*framing.wireSize(100), spans[0].bytes)

	// counting the framing once per block instead would be two orders of magnitude out
	assert.Less(t, spans[0].bytes, 1_000_000*framing.overhead(100))
}

// TestExtrapolateSamples_BlockFiltered: a module gated by a block index is only run on the
// blocks its index keeps, and that share is what a real request over the range would process.
func TestExtrapolateSamples_BlockFiltered(t *testing.T) {
	framing := newEgressFraming(&pbsubstreams.Module{Name: "map_out"}, 1_000_000)

	segments := []*sampleSegment{
		// ran on 5% of the blocks, emitted on 4% of them
		{rng: block.NewRange(0, 1000), uncompressedSize: 4_000, messageCount: 40, processedBlocks: 50},
		// already cached: no job ran, so the item count stands in for the processed blocks
		{rng: block.NewRange(500_000, 501_000), uncompressedSize: 3_000, messageCount: 30, fromCache: true},
	}

	spans := extrapolateSamples(segments, 0, 1_000_000, framing.ratesOf)
	require.Len(t, spans, 2)

	// first span averages 5% and 3%, the last keeps the 3% of the sample that opens it
	assert.Equal(t, uint64(20_000), spans[0].processedBlocks)
	assert.Equal(t, uint64(15_000), spans[1].processedBlocks)

	// far below the 1,000,000 blocks the range holds
	assert.Less(t, spans[0].processedBlocks+spans[1].processedBlocks, uint64(50_000))
}

// TestSamplePlanProcessedBlockCount: the segments the sample jobs ran are in the cache once
// the estimate is done, so what they processed comes off the work a real request has left.
func TestSamplePlanProcessedBlockCount(t *testing.T) {
	plan := &samplePlan{segments: []*sampleSegment{
		{rng: block.NewRange(0, 1000), processedBlocks: 1000},
		{rng: block.NewRange(500_000, 501_000), processedBlocks: 50},
		// already cached before the estimate ran: no job, nothing to discount
		{rng: block.NewRange(900_000, 901_000), messageCount: 1000, fromCache: true},
	}}

	assert.Equal(t, uint64(1050), plan.processedBlockCount())
}

func TestWithFilteredOutputStage(t *testing.T) {
	// one stage, nothing cached: the measured count is the whole answer
	blocks, effective := withFilteredOutputStage(1_000_000, 1_000_000, 1, 50_000)
	assert.Equal(t, uint64(50_000), blocks)
	assert.Equal(t, uint64(50_000), effective)

	// two stages, half of the work already cached: only the output module's stage is filtered
	blocks, effective = withFilteredOutputStage(2_000_000, 1_000_000, 2, 50_000)
	assert.Equal(t, uint64(1_050_000), blocks)
	assert.Equal(t, uint64(525_000), effective)

	// a module that runs on every block leaves the plan's numbers untouched
	blocks, effective = withFilteredOutputStage(2_000_000, 1_400_000, 2, 1_000_000)
	assert.Equal(t, uint64(2_000_000), blocks)
	assert.Equal(t, uint64(1_400_000), effective)
}

func TestEgressFraming(t *testing.T) {
	framing := newEgressFraming(&pbsubstreams.Module{
		Name:   "map_out",
		Output: &pbsubstreams.Module_Output{Type: "proto:sf.substreams.v1.test.MapResult"},
	}, 20_000_000)

	// the wrapper around an empty payload is the module name, the type URL, the clock, the
	// cursor and the final block height
	empty := framing.wireSize(0)
	assert.Greater(t, empty, uint64(200))
	assert.Equal(t, empty, framing.overhead(0))

	// a payload is carried whole, plus the length prefixes around it
	assert.Equal(t, uint64(1_000)+framing.overhead(1_000), framing.wireSize(1_000))
	assert.GreaterOrEqual(t, framing.overhead(1_000), empty)

	// framing is noise on a large payload
	assert.Less(t, float64(framing.overhead(1_000_000))/float64(1_000_000), 0.001)

	// a segment emitting on every block pays the framing once per block
	dense := &sampleSegment{rng: block.NewRange(0, 1000), uncompressedSize: 10_000, messageCount: 1000, processedBlocks: 1000}
	bytesPerBlock, messagesPerBlock := framing.ratesOf(dense)
	assert.Equal(t, float64(1), messagesPerBlock)
	assert.Equal(t, float64(framing.wireSize(10)), bytesPerBlock)

	// a segment with nothing in it costs nothing
	silent := &sampleSegment{rng: block.NewRange(0, 1000)}
	bytesPerBlock, messagesPerBlock = framing.ratesOf(silent)
	assert.Zero(t, bytesPerBlock)
	assert.Zero(t, messagesPerBlock)
}

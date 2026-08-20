package service

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"connectrpc.com/connect"
	"github.com/abourget/llerrgroup"
	"github.com/streamingfast/bstream"
	bsstream "github.com/streamingfast/bstream/stream"
	"github.com/streamingfast/dauth"
	"github.com/streamingfast/dmetering"
	"github.com/streamingfast/dsession"
	"github.com/streamingfast/dstore"
	"github.com/streamingfast/logging"
	tracing "github.com/streamingfast/sf-tracing"
	"github.com/streamingfast/substreams"
	"github.com/streamingfast/substreams/block"
	"github.com/streamingfast/substreams/manifest"
	"github.com/streamingfast/substreams/metering"
	"github.com/streamingfast/substreams/metrics"
	"github.com/streamingfast/substreams/orchestrator/plan"
	"github.com/streamingfast/substreams/orchestrator/response"
	"github.com/streamingfast/substreams/orchestrator/stage"
	"github.com/streamingfast/substreams/orchestrator/work"
	pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
	pbsubstreamsrpcv4 "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v4"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/pipeline"
	"github.com/streamingfast/substreams/pipeline/exec"
	"github.com/streamingfast/substreams/reqctx"
	"github.com/streamingfast/substreams/storage/execout"
	"github.com/streamingfast/substreams/storage/store"
	"github.com/streamingfast/substreams/storage/store/state"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// defaultSamplePercentage is how much of the requested range gets actually executed when
// the request does not ask for a specific percentage.
const defaultSamplePercentage = 1.0

// maxSamplePercentage is the highest sampling ratio a request may ask for: above that, running
// the sample costs about as much as running the real thing, which defeats the purpose.
const maxSamplePercentage = 10.0

// estimateProgressInterval is how often the sampling phase reports back to the client.
// It only exists to keep the connection alive and show that something is happening.
const estimateProgressInterval = 5 * time.Second

// sampleSegment is one segment of the range that gets measured, and what it measured.
type sampleSegment struct {
	segmentIdx int
	rng        *block.Range

	fromCache        bool // output file was already there, no job needed
	sizeFromMetadata bool // sizes came from object metadata rather than from reading the file
	uncompressedSize uint64
	messageCount     uint64 // items in the output file: one `BlockScopedData` each on the wire
	processedBlocks  uint64 // blocks the job actually ran on, blocks skipped by a block index excluded
}

// samplePlan is the set of segments that will be executed and measured, plus how they were
// picked.
type samplePlan struct {
	segments []*sampleSegment
	note     string
}

func (p *samplePlan) blockCount() (out uint64) {
	for _, seg := range p.segments {
		out += seg.rng.Size()
	}
	return
}

func (p *samplePlan) byteCount() (out uint64) {
	for _, seg := range p.segments {
		out += seg.uncompressedSize
	}
	return
}

func (p *samplePlan) messageCount() (out uint64) {
	for _, seg := range p.segments {
		out += seg.messageCount
	}
	return
}

// processedBlockCount is the work the sample jobs did. Segments that were already in the
// cache did not run one, and contribute nothing.
func (p *samplePlan) processedBlockCount() (out uint64) {
	for _, seg := range p.segments {
		if !seg.fromCache {
			out += seg.processedBlocks
		}
	}
	return
}

// estimate runs the whole estimation: resolve the range, look at what the cache already
// holds, sample a fraction of the range on tier2 workers, measure the size of the output
// that sample produced, and extrapolate.
//
// It deliberately never reads back any module data: sizes come from the object store's
// metadata whenever it is there.
func (s *Tier1Service) estimate(
	ctx context.Context,
	req *pbsubstreamsrpcv4.EstimateRequest,
	header http.Header,
	send func(*pbsubstreamsrpcv4.EstimateResponse) error,
) (err error) {
	if s.IsTerminating() {
		return connect.NewError(connect.CodeUnavailable, errShuttingDown)
	}

	if stat := s.getOverloadedStatus(); stat.hardLimitReached() {
		return connect.NewError(connect.CodeUnavailable, fmt.Errorf("service under heavy load, please try connecting again"))
	}

	s.activeRequestsWG.Add(1)
	defer s.activeRequestsWG.Done()

	logger := s.logger.Named("estimate")
	ctx = logging.WithLogger(ctx, logger)
	ctx = reqctx.WithTracer(ctx, s.tracer)
	ctx = dmetering.WithBytesMeter(ctx)
	ctx = metering.WithMetricsSender(ctx)
	ctx = reqctx.WithTier2RequestParameters(ctx, s.tier2RequestParameters)
	ctx = reqctx.WithEthCallFallbackToLatestDuration(ctx, fallbackDuration)
	ctx = reqctx.WithEthCallUseBlockNumberDuration(ctx, useBlockNumberDuration)

	ctx, span := reqctx.WithSpan(ctx, "substreams/tier1/estimate")
	defer span.EndWithErr(&err)

	if req.Package == nil {
		return connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("package is required"))
	}
	if _, err := manifest.ApplyPackageTransformations(req.Package, false, req.Network, req.OutputModule, req.Params); err != nil {
		return connect.NewError(connect.CodeInvalidArgument, err)
	}
	ctx = reqctx.WithSpkg(ctx, req.Package)

	percentage := req.SamplePercentage
	if percentage == 0 {
		percentage = defaultSamplePercentage
	}
	if percentage < 0 || percentage > maxSamplePercentage {
		return connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("sample_percentage must be in (0, %g], got %f", maxSamplePercentage, percentage))
	}

	// The estimated request is always a production-mode one: that is the mode whose cost we
	// can plan, and the only one that produces the output cache we measure.
	blocksReq := &pbsubstreamsrpc.Request{
		StartBlockNum:   req.StartBlockNum,
		StopBlockNum:    req.StopBlockNum,
		OutputModule:    req.OutputModule,
		Modules:         req.Package.Modules,
		ProductionMode:  true,
		FinalBlocksOnly: true,
	}
	if err := ValidateTier1Request(blocksReq, s.blockType); err != nil {
		return connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("validate request: %w", err))
	}

	execGraph, err := exec.NewOutputModuleGraph(req.OutputModule, true, blocksReq.Modules, bstream.GetProtocolFirstStreamableBlock)
	if err != nil {
		return connect.NewError(connect.CodeInvalidArgument, err)
	}
	outputModuleHash := execGraph.ModuleHashes()[req.OutputModule]
	ctx = reqctx.WithOutputModuleHash(ctx, outputModuleHash)

	var reqStats *metrics.Stats
	ctx, reqStats = setupRequestStats(ctx, req.OutputModule, outputModuleHash, execGraph, true, false)

	segmentSize := s.runtimeConfig.SegmentSize
	details, _, err := pipeline.BuildRequestDetails(ctx, blocksReq, s.getRecentFinalBlock, s.resolveCursor, s.getHeadBlock, segmentSize)
	if err != nil {
		return fmt.Errorf("build request details: %w", err)
	}

	// An open-ended request is estimated up to where parallel processing would hand over to
	// the linear pipeline: past that point there is nothing to plan against.
	stopBlock := details.StopBlockNum
	if stopBlock == 0 {
		stopBlock = details.LinearHandoffBlockNum
	}
	if stopBlock <= details.ResolvedStartBlockNum {
		return bsstream.NewErrInvalidArg("nothing to estimate: resolved start block %d is not below stop block %d", details.ResolvedStartBlockNum, stopBlock)
	}
	if err := execGraph.ValidateRequestStartBlock(details.ResolvedStartBlockNum); err != nil {
		return bsstream.NewErrInvalidArg("%s", err.Error())
	}

	// The estimate never streams anything linearly, so the whole range is parallel work.
	details.StopBlockNum = stopBlock
	details.LinearHandoffBlockNum = stopBlock
	parallelism := reqctx.GetEffectiveHeaderValues(ctx, header, s.runtimeConfig.DefaultParallelSubrequests, reqctx.DefaultMaxStageLayerParallelExecutorCount)
	details.MaxParallelJobs = parallelism.Workers
	details.MaxStageLayerParallelExecutor = parallelism.StageLayerExecutors
	ctx = reqctx.WithRequest(ctx, details)

	logger.Info("incoming Substreams Estimate request",
		zap.String("output_module", req.OutputModule),
		zap.String("output_module_hash", outputModuleHash),
		zap.Uint64("resolved_start_block", details.ResolvedStartBlockNum),
		zap.Uint64("stop_block", stopBlock),
		zap.Float64("sample_percentage", percentage),
	)
	defer func() {
		reqStats.SetError(err)
		reqStats.LogAndClose(ctx, details.ResolvedStartBlockNum)
	}()

	cacheStore, err := s.estimateCacheStore(ctx)
	if err != nil {
		return err
	}

	execOutputConfigs, err := execout.NewConfigs(cacheStore, execGraph.UsedModules(), execGraph.ModuleHashes(), segmentSize, bstream.GetProtocolFirstStreamableBlock, logger)
	if err != nil {
		return fmt.Errorf("new config map: %w", err)
	}
	storeConfigs, err := store.NewConfigMap(cacheStore, nil, execGraph.Stores(), execGraph.ModuleHashes(), bstream.GetProtocolFirstStreamableBlock, s.runtimeConfig.StoreSizeLimit, s.runtimeConfig.StoresScratchSpace, s.runtimeConfig.StoresBackend)
	if err != nil {
		return fmt.Errorf("configuring stores: %w", err)
	}

	scheduleStores := execGraph.StagedUsedModules()[0].LastLayer().IsStoreLayer()
	var lowestStoresInitBlock uint64
	if scheduleStores {
		lowestStoresInitBlock = *execGraph.LowestStoresInitBlock()
		if lowestStoresInitBlock > stopBlock {
			lowestStoresInitBlock = stopBlock
		}
	}

	reqPlan, err := plan.BuildTier1RequestPlan(true, segmentSize, execGraph.LowestInitBlock(), lowestStoresInitBlock, details.ResolvedStartBlockNum, stopBlock, stopBlock, scheduleStores)
	if err != nil {
		return bsstream.NewErrInvalidArg("%s", err.Error())
	}
	if !reqPlan.RequiresParallelProcessing() {
		return bsstream.NewErrInvalidArg("nothing to estimate on range [%d, %d)", details.ResolvedStartBlockNum, stopBlock)
	}

	// (A) How many blocks the real request would have to process, cache included.
	stages := stage.NewStages(ctx, execGraph, reqPlan, execOutputConfigs, storeConfigs)
	defer stages.Close()
	if err := stages.FetchCachesState(ctx); err != nil {
		return fmt.Errorf("fetch caches state: %w", err)
	}
	// Publishes the stage list to the request stats. The sparse sample runs its jobs without
	// the scheduler, which is what would otherwise report it, and the per-job accounting needs
	// it to attribute processed blocks to modules.
	stages.UpdateStats()
	var headBlock uint64
	if s.getHeadBlock != nil {
		if headBlock, err = s.getHeadBlock(); err != nil {
			logger.Warn("cannot get head block", zap.Error(err))
		}
	}
	// The blocks needed before the start block are always already cached: a range whose stores
	// do not reach it is refused when the sample is planned, so there is nothing to build.
	_, _, blocksAfter, effectiveBlocksAfter := stages.BlocksToProcess(headBlock)

	// (B) What the real request would send back, measured on a sample of the range.
	samples, err := s.planSampling(ctx, execGraph, execOutputConfigs, storeConfigs, details.ResolvedStartBlockNum, stopBlock, percentage)
	if err != nil {
		return fmt.Errorf("planning sample jobs: %w", err)
	}

	if err := s.runSampling(ctx, execGraph, execOutputConfigs, storeConfigs, samples, send); err != nil {
		return err
	}

	outputConfig := execOutputConfigs.ConfigMap[req.OutputModule]
	for _, seg := range samples.segments {
		size, messages, fromMetadata, err := outputConfig.OutputStats(ctx, seg.rng)
		if errors.Is(err, dstore.ErrNotFound) {
			logger.Warn("cannot measure sampled segment, counting it as empty", zap.Stringer("segment", seg.rng), zap.Error(err))
			continue
		}
		if err != nil {
			return fmt.Errorf("measuring output size of segment %s: %w", seg.rng, err)
		}
		seg.uncompressedSize = size
		seg.messageCount = messages
		seg.sizeFromMetadata = fromMetadata
	}

	sampledBlocks := samples.blockCount()
	sampledBytes := samples.byteCount()
	sampledMessages := samples.messageCount()
	rangeBlocks := stopBlock - details.ResolvedStartBlockNum

	framing := newEgressFraming(execGraph.OutputModule(), stopBlock)
	spans := extrapolateSamples(samples.segments, details.ResolvedStartBlockNum, stopBlock, framing.ratesOf)

	var estimatedEgressBytes, estimatedMessages, outputStageBlocks uint64
	for _, span := range spans {
		estimatedEgressBytes += span.bytes
		estimatedMessages += span.messages
		outputStageBlocks += span.processedBlocks
	}

	// Without a block filter the module runs on every block, so the sampled rate is 1 and the
	// two numbers below come out exactly as the plan says. With one, the output module's stage
	// only runs on the blocks its index keeps, and that measured share replaces the stage's
	// full share of the plan.
	blockFiltered := graphHasBlockFilter(execGraph)
	if blockFiltered {
		stageCount := uint64(len(execGraph.StagedUsedModules()))
		blocksAfter, effectiveBlocksAfter = withFilteredOutputStage(blocksAfter, effectiveBlocksAfter, stageCount, outputStageBlocks)
	}

	// The sample jobs filled the output cache for the segments they ran, and the cache state
	// was read before they did. A real request issued now would find those segments there, so
	// the work left is what the plan said minus what estimating just did. The uncached figure
	// is deliberately left alone: it answers what a first-ever run would have cost.
	justProcessed := min(samples.processedBlockCount(), effectiveBlocksAfter)
	effectiveBlocksAfter -= justProcessed

	var bytesPerBlock float64
	if rangeBlocks != 0 {
		bytesPerBlock = float64(estimatedEgressBytes) / float64(rangeBlocks)
	}

	estimate := &pbsubstreamsrpcv4.Estimate{
		ResolvedStartBlock:           details.ResolvedStartBlockNum,
		ResolvedStopBlock:            stopBlock,
		SegmentBlockCount:            segmentSize,
		StageCount:                   uint32(len(execGraph.StagedUsedModules())),
		BlocksToProcess:              effectiveBlocksAfter,
		TotalBlocksToProcessUncached: blocksAfter,
		BlockFiltered:                blockFiltered,
		EstimatedEgressBytes:         estimatedEgressBytes,
		BytesPerBlock:                bytesPerBlock,
		FramingBytesPerMessage:       framing.overhead(averagePayloadPerMessage(sampledBytes, sampledMessages)),
		EstimatedMessageCount:        estimatedMessages,
		Sampling: &pbsubstreamsrpcv4.Sampling{
			Percentage:      float64(sampledBlocks) / float64(rangeBlocks) * 100,
			Note:            samples.note,
			SampledBlocks:   sampledBlocks,
			SampledBytes:    sampledBytes,
			SampledMessages: sampledMessages,
			Segments:        make([]*pbsubstreamsrpcv4.SampledSegment, 0, len(samples.segments)),
		},
	}
	for i, seg := range samples.segments {
		segment := &pbsubstreamsrpcv4.SampledSegment{
			StartBlock:        seg.rng.StartBlock,
			StopBlock:         seg.rng.ExclusiveEndBlock,
			UncompressedBytes: seg.uncompressedSize,
			MessageCount:      seg.messageCount,
			ProcessedBlocks:   seg.processedBlocks,
			FromCache:         seg.fromCache,
			SizeFromMetadata:  seg.sizeFromMetadata,
		}
		if i < len(spans) {
			segment.RepresentedStartBlock = spans[i].startBlock
			segment.RepresentedBlocks = spans[i].blocks
			segment.EstimatedBytes = spans[i].bytes
		}
		estimate.Sampling.Segments = append(estimate.Sampling.Segments, segment)
	}

	logger.Info("estimate completed",
		zap.Uint64("blocks_to_process", estimate.BlocksToProcess),
		zap.Bool("block_filtered", blockFiltered),
		zap.Uint64("blocks_processed_by_the_estimate", justProcessed),
		zap.Uint64("estimated_egress_bytes", estimate.EstimatedEgressBytes),
		zap.Uint64("estimated_message_count", estimate.EstimatedMessageCount),
		zap.Uint64("sampled_blocks", sampledBlocks),
		zap.Uint64("sampled_bytes", sampledBytes),
		zap.Uint64("sampled_messages", sampledMessages),
	)

	return send(&pbsubstreamsrpcv4.EstimateResponse{
		Message: &pbsubstreamsrpcv4.EstimateResponse_Estimate{Estimate: estimate},
	})
}

// estimatedBlockIDLength is how long a block ID string is on most firehose chains: a 32-byte
// hash, hex-encoded. Chains whose IDs are shorter (base58, for one) make the framing estimate
// a few bytes per block too generous.
const estimatedBlockIDLength = 64

// egressFraming models what a client actually receives for one block. The estimate measures
// module payloads, but those travel wrapped in a `BlockScopedData` that also carries the
// module name, the output type URL, the clock, the cursor and the final block height with
// every single block. On a module whose per-block output is small that wrapper is most of the
// egress, so counting payloads alone reads far too low.
type egressFraming struct {
	outputModuleName string
	outputType       string
	blockNum         uint64
	blockID          string
	cursor           string
}

func newEgressFraming(outputModule *pbsubstreams.Module, blockNum uint64) *egressFraming {
	// A representative block: the ID and the cursor derived from it are the two parts whose
	// length is not known here, and the cursor is built the same way the server builds a real
	// one, so only the ID length is assumed.
	blockID := strings.Repeat("f", estimatedBlockIDLength)
	ref := bstream.NewBlockRef(blockID, blockNum)
	cursor := (&bstream.Cursor{Step: bstream.StepNewIrreversible, Block: ref, LIB: ref, HeadBlock: ref}).ToOpaque()

	var outputType string
	if outputModule.Output != nil {
		outputType = strings.TrimPrefix(outputModule.Output.Type, "proto:")
	}

	return &egressFraming{
		outputModuleName: outputModule.Name,
		outputType:       outputType,
		blockNum:         blockNum,
		blockID:          blockID,
		cursor:           cursor,
	}
}

// wireSize is the size of the message that carries `payloadSize` bytes of module output for
// one block. Built and measured rather than added up by hand so that the nested length
// prefixes, which grow with the payload, are accounted for.
func (f *egressFraming) wireSize(payloadSize uint64) uint64 {
	data := &pbsubstreamsrpc.BlockScopedData{
		Output: &pbsubstreamsrpc.MapModuleOutput{
			Name:      f.outputModuleName,
			MapOutput: &anypb.Any{TypeUrl: "type.googleapis.com/" + f.outputType, Value: make([]byte, payloadSize)},
		},
		Clock:            &pbsubstreams.Clock{Id: f.blockID, Number: f.blockNum, Timestamp: timestamppb.New(time.Unix(int64(f.blockNum), 0))},
		Cursor:           f.cursor,
		FinalBlockHeight: f.blockNum,
	}
	return uint64(proto.Size(data))
}

// overhead is what wireSize adds on top of the payload itself.
func (f *egressFraming) overhead(payloadSize uint64) uint64 {
	return f.wireSize(payloadSize) - payloadSize
}

// ratesOf turns what was measured on one sample into per-block rates: how many bytes a real
// request would put on the wire for each block of the range, and how many messages it would
// send for it.
//
// A module gated by a block index only runs on matching blocks, so its segment holds fewer
// items than it covers blocks, and the framing is paid once per item — not once per block.
// Deriving the payload average per *message* and multiplying by the message rate keeps the
// two apart, and collapses to the obvious thing on a module that emits on every block.
func (f *egressFraming) ratesOf(seg *sampleSegment) (bytesPerBlock, messagesPerBlock float64) {
	blocks := seg.rng.Size()
	if blocks == 0 || seg.messageCount == 0 {
		return 0, 0
	}

	messagesPerBlock = float64(seg.messageCount) / float64(blocks)
	payloadPerMessage := float64(seg.uncompressedSize) / float64(seg.messageCount)
	wirePerMessage := payloadPerMessage + float64(f.overhead(uint64(math.Round(payloadPerMessage))))

	return messagesPerBlock * wirePerMessage, messagesPerBlock
}

// processedBlocksPerBlock is the share of the blocks a sample covers that the module was
// actually run on. A block index gates execution, so a filtered module runs on a fraction of
// them, and that fraction is what a real request over the range would be billed for.
//
// A segment served from the cache ran no job, so there is no count to take: its output item
// count stands in for it, which is exact for a module that emits whenever it runs.
func processedBlocksPerBlock(seg *sampleSegment) float64 {
	blocks := seg.rng.Size()
	if blocks == 0 {
		return 0
	}

	processed := seg.processedBlocks
	if seg.fromCache {
		processed = seg.messageCount
	}
	return float64(processed) / float64(blocks)
}

// sampleSpan is the slice of the requested range that one sample stands for, and what was
// extrapolated over it.
type sampleSpan struct {
	startBlock      uint64
	blocks          uint64
	bytes           uint64
	messages        uint64
	processedBlocks uint64
}

// extrapolateSamples partitions [startBlock, stopBlock) between the samples: each one stands
// for the blocks between its own start and the start of the next sample, the last one for
// everything left up to the end of the range. The first span reaches down to `startBlock`,
// since sampling begins on a segment boundary at or above it.
//
// The rate applied to a span is the average of the rates measured at both of its ends, so a
// span between two samples of unequal density lands between the two rather than inheriting
// either. The last span, having no sample after it, keeps the rate of the sample that opens
// it.
//
// One span is returned per sample, in the same order, so that the two can be zipped; a span
// that covers no block has a zero block count.
func extrapolateSamples(segments []*sampleSegment, startBlock, stopBlock uint64, ratesOf func(*sampleSegment) (bytesPerBlock, messagesPerBlock float64)) []sampleSpan {
	out := make([]sampleSpan, len(segments))
	if len(segments) == 0 {
		return out
	}

	byteRates := make([]float64, len(segments))
	messageRates := make([]float64, len(segments))
	processedRates := make([]float64, len(segments))
	for i, seg := range segments {
		byteRates[i], messageRates[i] = ratesOf(seg)
		processedRates[i] = processedBlocksPerBlock(seg)
	}

	for i, seg := range segments {
		spanStart := seg.rng.StartBlock
		if i == 0 {
			spanStart = min(spanStart, startBlock)
		}

		spanStop := stopBlock
		byteRate, messageRate, processedRate := byteRates[i], messageRates[i], processedRates[i]
		if i+1 < len(segments) {
			spanStop = segments[i+1].rng.StartBlock
			byteRate = (byteRates[i] + byteRates[i+1]) / 2
			messageRate = (messageRates[i] + messageRates[i+1]) / 2
			processedRate = (processedRates[i] + processedRates[i+1]) / 2
		}

		out[i].startBlock = spanStart
		if spanStop <= spanStart {
			continue
		}
		out[i].blocks = spanStop - spanStart
		out[i].bytes = uint64(byteRate * float64(out[i].blocks))
		out[i].messages = uint64(messageRate * float64(out[i].blocks))
		out[i].processedBlocks = uint64(processedRate * float64(out[i].blocks))
	}
	return out
}

// graphHasBlockFilter reports whether any module of the graph is gated by a block index.
func graphHasBlockFilter(execGraph *exec.Graph) bool {
	for _, module := range execGraph.UsedModules() {
		if module.BlockFilter != nil {
			return true
		}
	}
	return false
}

// withFilteredOutputStage swaps the output module's stage out of the planned block counts and
// puts the measured one in its place. Every stage covers the same segments, so the plan's
// figure divides evenly between them; only the stage the sample actually ran is known to be
// filtered, and the others are left whole.
//
// The cache-aware figure is scaled by the same share of the work the plan says is left, since
// the sample measures the range rather than what the cache still misses.
func withFilteredOutputStage(blocksAfter, effectiveBlocksAfter, stageCount, outputStageBlocks uint64) (uint64, uint64) {
	if stageCount == 0 || blocksAfter == 0 {
		return blocksAfter, effectiveBlocksAfter
	}

	otherStages := blocksAfter - blocksAfter/stageCount
	filtered := otherStages + outputStageBlocks
	uncachedShare := float64(effectiveBlocksAfter) / float64(blocksAfter)

	return filtered, uint64(float64(filtered) * uncachedShare)
}

func averagePayloadPerMessage(sampledBytes, sampledMessages uint64) uint64 {
	if sampledMessages == 0 {
		return 0
	}
	return sampledBytes / sampledMessages
}

// estimateCacheStore resolves the same cache store a `Blocks` request would use, so that
// the estimate sees (and fills) exactly the same cache.
func (s *Tier1Service) estimateCacheStore(ctx context.Context) (dstore.Store, error) {
	cacheTag := s.runtimeConfig.DefaultCacheTag
	if ct := dauth.FromContext(ctx).Get(reqctx.HeaderCacheTag); ct != "" && IsValidCacheTag(ct) {
		cacheTag = ct
	}

	cacheStore, err := s.runtimeConfig.BaseObjectStore.SubStore(cacheTag)
	if err != nil {
		return nil, fmt.Errorf("internal error setting store: %w", err)
	}

	if clonableStore, ok := cacheStore.(dstore.Clonable); ok {
		cloned, err := clonableStore.Clone(ctx, metering.WithBytesMeteringOptions(dmetering.GetBytesMeter(ctx), reqctx.Logger(ctx))...)
		if err != nil {
			return nil, fmt.Errorf("cloning store: %w", err)
		}
		cloned.SetMeter(dmetering.GetBytesMeter(ctx))
		cacheStore = cloned
	}
	return cacheStore, nil
}

// planSampling decides which segments of the range get executed to measure the output size.
//
// Without store modules, any segment can be run on its own, so the sample is spread evenly
// over the range: a handful of segments at regular intervals is far more representative
// than a contiguous block of them.
//
// Store modules make that impossible in general: a segment can only run if every store is
// already usable at its first block, which means all previous segments have been squashed.
// So with stores we sample evenly, but only among the segments the cache already covers.
// When the cache does not cover enough of them, we fall back to a contiguous sample
// starting where the stores *are* usable (their highest cached boundary at or below the
// start block, or their initial block, where they are legitimately empty).
func (s *Tier1Service) planSampling(
	ctx context.Context,
	execGraph *exec.Graph,
	execOutputConfigs *execout.Configs,
	storeConfigs store.ConfigMap,
	startBlock uint64,
	stopBlock uint64,
	percentage float64,
) (*samplePlan, error) {
	segmentSize := s.runtimeConfig.SegmentSize
	outputModule := execGraph.OutputModule()

	// Sampling only ever considers whole segments at or after the output module's initial
	// block: those are the ones a tier2 job produces a full output file for.
	sampleStart := max(startBlock, execGraph.LowestInitBlock())
	if rest := sampleStart % segmentSize; rest != 0 {
		sampleStart += segmentSize - rest
	}
	if sampleStart+segmentSize > stopBlock {
		return nil, bsstream.NewErrInvalidArg("range [%d, %d) is too small to sample: it does not hold a complete %d-blocks segment", startBlock, stopBlock, segmentSize)
	}
	segmenter := block.NewSegmenter(segmentSize, sampleStart, stopBlock)

	// The last segment is skipped when the range ends inside it: a partial segment measures
	// fewer blocks than it costs to run.
	firstIdx, lastIdx := segmenter.FirstIndex(), segmenter.LastIndex()
	if segmenter.Range(lastIdx).Size() != segmentSize && lastIdx > firstIdx {
		lastIdx--
	}
	totalSegments := lastIdx - firstIdx + 1

	wanted := int(math.Ceil(float64(totalSegments) * percentage / 100))
	wanted = min(max(wanted, 1), totalSegments)

	candidates := make([]int, 0, totalSegments)
	for idx := firstIdx; idx <= lastIdx; idx++ {
		candidates = append(candidates, idx)
	}

	note := fmt.Sprintf("%d segments of %d blocks, spread evenly over the %d segments of the range", wanted, segmentSize, totalSegments)

	// A segment can only run on its own if every store is loadable at its first block. Rather
	// than work around a partially built cache, the request is refused and told where the
	// stores do reach.
	if len(storeConfigs) != 0 {
		if err := requireStoresOver(ctx, storeConfigs, segmentSize, segmenter, firstIdx, lastIdx); err != nil {
			return nil, err
		}
	}

	picked := pickEvenly(candidates, wanted)

	// Segments whose output is already cached cost nothing to measure.
	cached := make(map[uint64]bool)
	outputFiles, err := execOutputConfigs.ConfigMap[outputModule.Name].ListSnapshotFiles(ctx, segmenter.Range(picked[0]).StartBlock, segmenter.Range(picked[len(picked)-1]).ExclusiveEndBlock)
	if err != nil {
		return nil, fmt.Errorf("listing output files: %w", err)
	}
	for _, file := range outputFiles {
		cached[file.BlockRange.ExclusiveEndBlock] = true
	}

	out := &samplePlan{note: note}
	for _, idx := range picked {
		rng := segmenter.Range(idx)
		out.segments = append(out.segments, &sampleSegment{
			segmentIdx: idx,
			rng:        rng,
			fromCache:  cached[rng.ExclusiveEndBlock],
		})
	}
	return out, nil
}

// requireStoresOver checks that every segment of the sample range can run on its own, and
// refuses the request otherwise, naming the part of the range where the stores do reach.
//
// A tier2 job on a segment loads each store from the snapshot ending at that segment's first
// block, so a segment is only runnable where every store is either snapshotted or has not
// started yet. Building the missing state would mean processing the range up to it, which is
// the very cost the estimate is supposed to report rather than incur.
func requireStoresOver(ctx context.Context, storeConfigs store.ConfigMap, segmentSize uint64, segmenter *block.Segmenter, firstIdx, lastIdx int) error {
	readyBoundaries, err := storeReadyBoundaries(ctx, storeConfigs, segmentSize, segmenter.ExclusiveEndBlock())
	if err != nil {
		return err
	}

	// The runnable part of the range is a prefix: stores are built forward, so the first
	// segment whose stores are missing ends it.
	usableUpTo := segmenter.Range(firstIdx).StartBlock
	for idx := firstIdx; idx <= lastIdx; idx++ {
		if _, found := readyBoundaries[segmenter.Range(idx).StartBlock]; !found {
			break
		}
		usableUpTo = segmenter.Range(idx).ExclusiveEndBlock
	}

	if usableUpTo >= segmenter.Range(lastIdx).ExclusiveEndBlock {
		return nil
	}

	firstBlock := segmenter.Range(firstIdx).StartBlock
	if usableUpTo <= firstBlock {
		return bsstream.NewErrInvalidArg("this endpoint holds no store state for range [%d, %d): estimating it would mean building the stores first, which is the cost being estimated. Run the request over a range whose stores are already built, or ask for an estimate once they are",
			firstBlock, segmenter.ExclusiveEndBlock())
	}

	return bsstream.NewErrInvalidArg("this endpoint's store state only covers [%d, %d) of the requested range: estimating past it would mean building the missing stores, which is the cost being estimated. Estimate [%d, %d) instead",
		firstBlock, usableUpTo, firstBlock, usableUpTo)
}

// storeReadyBoundaries returns the block boundaries at which a tier2 job can load every
// store it needs: each store either has a full KV snapshot ending there, or has not started
// yet at that block, in which case there is nothing to load.
func storeReadyBoundaries(ctx context.Context, storeConfigs store.ConfigMap, segmentSize uint64, stopBlock uint64) (map[uint64]struct{}, error) {
	var lowestInitBlock uint64 = math.MaxUint64
	for _, config := range storeConfigs {
		lowestInitBlock = min(lowestInitBlock, config.ModuleInitialBlock())
	}

	cacheState, err := state.FetchState(ctx, storeConfigs, lowestInitBlock, stopBlock)
	if err != nil {
		return nil, fmt.Errorf("fetching stores storage state: %w", err)
	}

	// A boundary is only worth considering if some store starts there or was snapshotted there.
	candidates := make(map[uint64]struct{})
	fullKVs := make(map[string]map[uint64]bool, len(storeConfigs))
	for name, config := range storeConfigs {
		candidates[config.ModuleInitialBlock()] = struct{}{}

		ends := make(map[uint64]bool)
		if snapshots := cacheState.Snapshots[name]; snapshots != nil {
			for _, fullKV := range snapshots.FullKVFiles {
				if fullKV.Range.ExclusiveEndBlock%segmentSize != 0 {
					continue
				}
				ends[fullKV.Range.ExclusiveEndBlock] = true
				candidates[fullKV.Range.ExclusiveEndBlock] = struct{}{}
			}
		}
		fullKVs[name] = ends
	}

	out := make(map[uint64]struct{})
	for boundary := range candidates {
		ready := true
		for name, config := range storeConfigs {
			if config.ModuleInitialBlock() >= boundary {
				continue // that store holds nothing yet at this block
			}
			if !fullKVs[name][boundary] {
				ready = false
				break
			}
		}
		if ready {
			out[boundary] = struct{}{}
		}
	}
	return out, nil
}

// pickEvenly takes `count` elements spread as evenly as possible over `in`.
func pickEvenly(in []int, count int) []int {
	if count >= len(in) {
		return in
	}
	out := make([]int, 0, count)
	for i := 0; i < count; i++ {
		out = append(out, in[i*len(in)/count])
	}
	return out
}

// runSampling executes the planned segments on tier2 workers. Nothing is streamed back:
// the jobs only fill the output cache, which is then measured.
func (s *Tier1Service) runSampling(
	ctx context.Context,
	execGraph *exec.Graph,
	execOutputConfigs *execout.Configs,
	storeConfigs store.ConfigMap,
	samples *samplePlan,
	send func(*pbsubstreamsrpcv4.EstimateResponse) error,
) error {
	var todo []*sampleSegment
	for _, seg := range samples.segments {
		if !seg.fromCache {
			todo = append(todo, seg)
		}
	}

	progress := &estimateProgress{
		send:  send,
		total: uint64(len(samples.segments)),
	}
	progress.completeMany(uint64(len(samples.segments) - len(todo)))
	if len(todo) == 0 {
		return progress.report()
	}

	sessionKey, releaseSession, err := s.acquireEstimateSession(ctx)
	if err != nil {
		return err
	}
	defer releaseSession()
	if sessionKey != "" {
		ctx = reqctx.WithSessionKey(ctx, sessionKey)
	}

	stop := progress.reportPeriodically(ctx)
	defer stop()

	return s.runSampleJobs(ctx, execGraph, todo, progress)
}

// runSampleJobs runs one tier2 job per sampled segment. Every store is usable at each
// segment's first block — planSampling refuses the request otherwise — so a single job on the
// last stage produces everything that segment needs, with no squashing and no ordering
// constraint between them.
func (s *Tier1Service) runSampleJobs(ctx context.Context, execGraph *exec.Graph, todo []*sampleSegment, progress *estimateProgress) error {
	details := reqctx.Details(ctx)
	workerPool := s.runtimeConfig.WorkerPoolFactory(ctx)
	lastStage := execGraph.OutputModuleStageIndex()
	moduleNames := []string{details.OutputModule}
	upstream := response.New(func(substreams.ResponseFromAnyTier) error { return nil })

	eg := llerrgroup.New(int(max(details.MaxParallelJobs, 1)))
	for _, seg := range todo {
		if eg.Stop() {
			break
		}
		eg.Go(func() error {
			worker, err := borrowWorker(ctx, workerPool)
			if err != nil {
				return err
			}
			defer workerPool.Return(ctx, worker)

			unit := stage.Unit{Stage: lastStage, Segment: seg.segmentIdx}
			switch msg := worker.Work(ctx, unit, seg.rng.StartBlock, moduleNames, upstream, false)().(type) {
			case work.MsgJobFailed:
				return fmt.Errorf("sample job on segment %s failed: %w", seg.rng, msg.Error)
			case work.MsgJobSucceeded:
				// What the module was actually run on. A block index gates execution, so this
				// is below the segment size whenever the request filters blocks.
				seg.processedBlocks = msg.ProcessedBlocks
			}
			return progress.completeOne()
		})
	}
	return eg.Wait()
}

// acquireEstimateSession takes the same kind of session a `Blocks` request takes: the
// tier2 worker pool is handed out per session.
func (s *Tier1Service) acquireEstimateSession(ctx context.Context) (sessionKey string, release func(), err error) {
	if s.sessionPool == nil {
		return "", func() {}, nil
	}

	auth := dauth.FromContext(ctx)
	sessionID, err := s.sessionPool.Get(ctx, "t1r", auth.OrganizationID(), auth.APIKeyID(), tracing.GetTraceID(ctx).String(), func(error) {})
	if err != nil {
		switch {
		case errors.Is(err, dsession.ErrConcurrentStreamLimitExceeded),
			errors.Is(err, dsession.ErrPermissionDenied),
			errors.Is(err, dsession.ErrQuotaExceeded):
			s.logger.Info("session denied to user", zap.String("api_key_id", auth.APIKeyID()), zap.Error(err))
		default:
			s.logger.Error("failed to acquire session", zap.Error(err))
		}
		return "", nil, err
	}

	return sessionID, func() { s.sessionPool.Release(sessionID) }, nil
}

func borrowWorker(ctx context.Context, pool work.WorkerPool) (work.Worker, error) {
	for {
		worker, err := pool.Borrow(ctx)
		if err == nil {
			return worker, nil
		}
		if !errors.Is(err, work.ErrorResourceExhausted) && !errors.Is(err, work.ErrorResourceExhaustedRampUp) {
			return nil, fmt.Errorf("borrowing worker: %w", err)
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Second):
		}
	}
}

// estimateProgress serializes the progress messages sent while the sample runs: they come
// from the job goroutines and from a ticker, and a stream cannot be written concurrently.
type estimateProgress struct {
	send func(*pbsubstreamsrpcv4.EstimateResponse) error

	sendMutex sync.Mutex
	total     uint64
	completed atomic.Uint64
}

func (p *estimateProgress) completeOne() error {
	p.completed.Add(1)
	return p.report()
}

func (p *estimateProgress) completeMany(count uint64) {
	p.completed.Add(count)
}

func (p *estimateProgress) report() error {
	p.sendMutex.Lock()
	defer p.sendMutex.Unlock()
	return p.send(&pbsubstreamsrpcv4.EstimateResponse{
		Message: &pbsubstreamsrpcv4.EstimateResponse_Progress{
			Progress: &pbsubstreamsrpcv4.EstimateProgress{
				CompletedSegments: p.completed.Load(),
				TotalSegments:     p.total,
			},
		},
	})
}

// reportPeriodically keeps the stream alive while long jobs run.
func (p *estimateProgress) reportPeriodically(ctx context.Context) (stop func()) {
	ctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(estimateProgressInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := p.report(); err != nil {
					return
				}
			}
		}
	}()
	// waits for the reporter to be gone: nothing else may write to the stream while it lives
	return func() {
		cancel()
		<-done
	}
}

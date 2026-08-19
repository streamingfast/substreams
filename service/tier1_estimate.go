package service

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
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
	"github.com/streamingfast/substreams/orchestrator"
	"github.com/streamingfast/substreams/orchestrator/plan"
	"github.com/streamingfast/substreams/orchestrator/response"
	"github.com/streamingfast/substreams/orchestrator/stage"
	"github.com/streamingfast/substreams/orchestrator/work"
	pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
	pbsubstreamsrpcv4 "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v4"
	"github.com/streamingfast/substreams/pipeline"
	"github.com/streamingfast/substreams/pipeline/exec"
	"github.com/streamingfast/substreams/reqctx"
	"github.com/streamingfast/substreams/storage/execout"
	"github.com/streamingfast/substreams/storage/store"
	"github.com/streamingfast/substreams/storage/store/state"
	"go.uber.org/zap"
)

// defaultSamplePercentage is how much of the requested range gets actually executed when
// the request does not ask for a specific percentage.
const defaultSamplePercentage = 1.0

// estimateProgressInterval is how often the sampling phase reports back to the client.
// It only exists to keep the connection alive and show that something is happening.
const estimateProgressInterval = 5 * time.Second

// sampleSegment is one segment of the range that gets measured, and what it measured.
type sampleSegment struct {
	segmentIdx int
	rng        *block.Range

	fromCache        bool // output file was already there, no job needed
	sizeFromMetadata bool // size came from object metadata rather than from reading the file
	uncompressedSize uint64
}

// samplePlan is the set of segments that will be executed and measured, plus how they were
// picked. `sparse` false means the segments are contiguous: see planSampling.
type samplePlan struct {
	segments []*sampleSegment
	sparse   bool
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
	if percentage < 0 || percentage > 100 {
		return connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("sample_percentage must be in (0, 100], got %f", percentage))
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
	var headBlock uint64
	if s.getHeadBlock != nil {
		if headBlock, err = s.getHeadBlock(); err != nil {
			logger.Warn("cannot get head block", zap.Error(err))
		}
	}
	blocksBefore, effectiveBlocksBefore, blocksAfter, effectiveBlocksAfter := stages.BlocksToProcess(headBlock)

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
		size, fromMetadata, err := outputConfig.UncompressedSize(ctx, seg.rng)
		if errors.Is(err, dstore.ErrNotFound) {
			logger.Warn("cannot measure sampled segment, counting it as empty", zap.Stringer("segment", seg.rng), zap.Error(err))
			continue
		}
		if err != nil {
			return fmt.Errorf("measuring output size of segment %s: %w", seg.rng, err)
		}
		seg.uncompressedSize = size
		seg.sizeFromMetadata = fromMetadata
	}

	sampledBlocks := samples.blockCount()
	sampledBytes := samples.byteCount()
	rangeBlocks := stopBlock - details.ResolvedStartBlockNum

	var bytesPerBlock float64
	if sampledBlocks != 0 {
		bytesPerBlock = float64(sampledBytes) / float64(sampledBlocks)
	}

	estimate := &pbsubstreamsrpcv4.Estimate{
		ResolvedStartBlock:              details.ResolvedStartBlockNum,
		ResolvedStopBlock:               stopBlock,
		SegmentBlockCount:               segmentSize,
		StageCount:                      uint32(len(execGraph.StagedUsedModules())),
		BlocksToProcess:                 effectiveBlocksAfter,
		BlocksToProcessBeforeStartBlock: effectiveBlocksBefore,
		TotalBlocksToProcessUncached:    blocksBefore + blocksAfter,
		EstimatedEgressBytes:            uint64(bytesPerBlock * float64(rangeBlocks)),
		BytesPerBlock:                   bytesPerBlock,
		Sampling: &pbsubstreamsrpcv4.Sampling{
			Percentage:    float64(sampledBlocks) / float64(rangeBlocks) * 100,
			Sparse:        samples.sparse,
			Note:          samples.note,
			SampledBlocks: sampledBlocks,
			SampledBytes:  sampledBytes,
			Segments:      make([]*pbsubstreamsrpcv4.SampledSegment, 0, len(samples.segments)),
		},
	}
	for _, seg := range samples.segments {
		estimate.Sampling.Segments = append(estimate.Sampling.Segments, &pbsubstreamsrpcv4.SampledSegment{
			StartBlock:        seg.rng.StartBlock,
			StopBlock:         seg.rng.ExclusiveEndBlock,
			UncompressedBytes: seg.uncompressedSize,
			FromCache:         seg.fromCache,
			SizeFromMetadata:  seg.sizeFromMetadata,
		})
	}

	logger.Info("estimate completed",
		zap.Uint64("blocks_to_process", estimate.BlocksToProcess),
		zap.Uint64("blocks_to_process_before_start_block", estimate.BlocksToProcessBeforeStartBlock),
		zap.Uint64("estimated_egress_bytes", estimate.EstimatedEgressBytes),
		zap.Uint64("sampled_blocks", sampledBlocks),
		zap.Uint64("sampled_bytes", sampledBytes),
		zap.Bool("sparse", samples.sparse),
	)

	return send(&pbsubstreamsrpcv4.EstimateResponse{
		Message: &pbsubstreamsrpcv4.EstimateResponse_Estimate{Estimate: estimate},
	})
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

	sparse := true
	note := fmt.Sprintf("%d segments of %d blocks, spread evenly over the %d segments of the range", wanted, segmentSize, totalSegments)

	if len(storeConfigs) != 0 {
		readyBoundaries, err := storeReadyBoundaries(ctx, storeConfigs, segmentSize, stopBlock)
		if err != nil {
			return nil, err
		}

		usable := make([]int, 0, len(candidates))
		for _, idx := range candidates {
			if _, found := readyBoundaries[segmenter.Range(idx).StartBlock]; found {
				usable = append(usable, idx)
			}
		}

		if len(usable) >= wanted {
			candidates = usable
			note = fmt.Sprintf("%d segments of %d blocks, spread evenly over the %d segments whose stores are already cached", wanted, segmentSize, len(usable))
		} else {
			// Not enough cached store state to jump around: run a contiguous sample from the
			// highest boundary where the stores are usable.
			seqStart := highestBoundaryAtOrBelow(readyBoundaries, startBlock)

			// Below the output module's initial block there is nothing to measure: those
			// segments run the module before it starts, produce no output file, and would be
			// counted as empty, reporting an egress of zero for the whole range. Refuse rather
			// than answer with a number we know is wrong.
			if seqStart < outputModule.InitialBlock {
				return nil, bsstream.NewErrInvalidArg(
					"cannot sample range [%d, %d): the stores are only usable from block %d, below the initial block %d of module %q, so a contiguous sample would measure blocks on which the module produces nothing. Estimate a range whose stores this endpoint has already cached",
					startBlock, stopBlock, seqStart, outputModule.InitialBlock, outputModule.Name)
			}

			seqSegmenter := block.NewSegmenter(segmentSize, seqStart, stopBlock)
			firstSeqIdx := seqSegmenter.FirstIndex()
			if seqSegmenter.Range(firstSeqIdx).Size() != segmentSize {
				// A store's initial block does not have to sit on a segment boundary, so the
				// first segment can be a partial one. It still gets processed to keep the
				// stores continuous, it is just not measured: it covers fewer blocks than a
				// whole segment and would weigh the extrapolation down.
				firstSeqIdx++
			}

			candidates = candidates[:0]
			for i, idx := 0, firstSeqIdx; i < wanted && idx <= seqSegmenter.LastIndex(); i, idx = i+1, idx+1 {
				candidates = append(candidates, idx)
			}
			if len(candidates) == 0 {
				return nil, bsstream.NewErrInvalidArg("range [%d, %d) holds no complete segment to sample contiguously from block %d, where the stores become usable", startBlock, stopBlock, seqStart)
			}

			segmenter = seqSegmenter
			sparse = false
			note = fmt.Sprintf("%d contiguous segments of %d blocks from block %d: store modules are sequential and the cache does not cover enough of the range to sample it sparsely", len(candidates), segmentSize, seqSegmenter.Range(candidates[0]).StartBlock)
		}
	}

	picked := pickEvenly(candidates, wanted)
	if len(picked) == 0 {
		// Every branch above is meant to leave at least one candidate; if none did, the range
		// holds no segment this request can run, and the indexing below would panic.
		return nil, bsstream.NewErrInvalidArg("no segment of range [%d, %d) can be sampled", startBlock, stopBlock)
	}

	// Segments whose output is already cached cost nothing to measure.
	cached := make(map[uint64]bool)
	outputFiles, err := execOutputConfigs.ConfigMap[outputModule.Name].ListSnapshotFiles(ctx, segmenter.Range(picked[0]).StartBlock, segmenter.Range(picked[len(picked)-1]).ExclusiveEndBlock)
	if err != nil {
		return nil, fmt.Errorf("listing output files: %w", err)
	}
	for _, file := range outputFiles {
		cached[file.BlockRange.ExclusiveEndBlock] = true
	}

	out := &samplePlan{sparse: sparse, note: note}
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

func highestBoundaryAtOrBelow(boundaries map[uint64]struct{}, blockNum uint64) (out uint64) {
	for boundary := range boundaries {
		if boundary <= blockNum && boundary >= out {
			out = boundary
		}
	}
	return
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

	if !samples.sparse {
		return s.runContiguousSample(ctx, execGraph, execOutputConfigs, storeConfigs, samples, progress)
	}
	return s.runSparseSample(ctx, execGraph, todo, progress)
}

// runSparseSample runs one tier2 job per sampled segment. The segments were picked so that
// all stores are usable at their first block, so a single job on the last stage produces
// everything that segment needs, with no squashing and no ordering constraint between them.
func (s *Tier1Service) runSparseSample(ctx context.Context, execGraph *exec.Graph, todo []*sampleSegment, progress *estimateProgress) error {
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
			}
			return progress.completeOne()
		})
	}
	return eg.Wait()
}

// runContiguousSample runs a contiguous sample through the regular parallel processor: the
// stores have to be built stage by stage and squashed between segments, which is exactly
// what it does. The request plan is stripped of its read side so that no module data is
// ever read back or sent.
func (s *Tier1Service) runContiguousSample(
	ctx context.Context,
	execGraph *exec.Graph,
	execOutputConfigs *execout.Configs,
	storeConfigs store.ConfigMap,
	samples *samplePlan,
	progress *estimateProgress,
) error {
	// The whole contiguous range is handed to the processor, cached segments included: it
	// skips what the cache already covers, and starting past them would leave the stores
	// unusable at the first segment it does have to run.
	details := *reqctx.Details(ctx)
	sampleStart := samples.segments[0].rng.StartBlock
	sampleStop := samples.segments[len(samples.segments)-1].rng.ExclusiveEndBlock

	details.ResolvedStartBlockNum = sampleStart
	details.StopBlockNum = sampleStop
	details.LinearHandoffBlockNum = sampleStop
	ctx = reqctx.WithRequest(ctx, &details)

	scheduleStores := execGraph.StagedUsedModules()[0].LastLayer().IsStoreLayer()
	var lowestStoresInitBlock uint64
	if scheduleStores {
		lowestStoresInitBlock = min(*execGraph.LowestStoresInitBlock(), sampleStop)
	}

	reqPlan, err := plan.BuildTier1RequestPlan(true, s.runtimeConfig.SegmentSize, execGraph.LowestInitBlock(), lowestStoresInitBlock, sampleStart, sampleStop, sampleStop, scheduleStores)
	if err != nil {
		return fmt.Errorf("building sample request plan: %w", err)
	}
	reqPlan.ReadExecOut = nil // we measure the output files, we never read them back

	processor, err := orchestrator.BuildParallelProcessor(
		ctx,
		reqPlan,
		s.runtimeConfig.WorkerPoolFactory(ctx),
		execGraph,
		execOutputConfigs,
		func(substreams.ResponseFromAnyTier) error { return nil },
		storeConfigs,
		true, // noop mode: no output is sent anywhere
		0,
		false,
	)
	if err != nil {
		return fmt.Errorf("building parallel processor: %w", err)
	}

	if _, err := processor.Run(ctx, s.IsTerminating); err != nil {
		return fmt.Errorf("running sample jobs: %w", err)
	}

	progress.completeMany(uint64(len(samples.segments)) - progress.completed.Load())
	return progress.report()
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

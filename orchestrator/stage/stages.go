package stage

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/dustin/go-humanize"
	"github.com/streamingfast/bstream"
	"github.com/streamingfast/substreams/block"
	"github.com/streamingfast/substreams/metrics"
	"github.com/streamingfast/substreams/orchestrator/loop"
	"github.com/streamingfast/substreams/orchestrator/plan"
	pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
	"github.com/streamingfast/substreams/pipeline/exec"
	"github.com/streamingfast/substreams/reqctx"
	"github.com/streamingfast/substreams/storage/execout"
	"github.com/streamingfast/substreams/storage/store"
)

// NOTE:
// Would we have an internal StoreMap here where there's a
// store.FullKV _and_ a State, so this thing would be top-level
// here in the `Stages`, it would keep track of what's happening with
// its internal `store.FullKV`, and the merging state.
// The `ModuleState` would be merely pointing to that Map,
// or a Map of "MergeableStore" ? with a completedSegments, etc, etc..
// most of the things in the `modstate` ?
// and they'd live here: Stages::storeMap: map[storeName]*MergeableStore
// any incoming message that a merged store finished, would funnel
// to its Stage, and would keep track of all those MergeableStore, see if
// all of its modules are completed for that stage, and then send the signal
// that the Stage is completed, kicking off the next layer of jobs.

type Stages struct {
	ctx    context.Context
	logger *zap.Logger

	globalSegmenter   *block.Segmenter // This segmenter covers both the stores and the mapper
	storeSegmenter    *block.Segmenter // This segmenter covers only jobs needed to build up stores according to the RequestPlan.
	mapSegmenter      *block.Segmenter // This segmenter covers only what is needed to produce the mapper output for the FileWalker.
	hasLinearPipeline bool

	stages []*Stage

	// segmentStates is a matrix of segment and stages
	segmentStates       []stageStates // segmentStates[offsetSegment][StageIndex]
	lastStatUpdate      time.Time
	outputModuleIsIndex bool

	// If you're processing at 12M blocks, offset 12,000 segments, so you don't need to allocate 12k empty elements.
	// Any previous segment is assumed to have completed successfully, and any stores that we sync'd prior to this offset
	// are assumed to have been either fully loaded, or merged up until this offset.
	segmentOffset int

	storeConfigs   store.ConfigMap
	execoutConfigs *execout.Configs

	// first segment where we can run directly the higher stages (shadowing the lower stages)
	shadowableSegment int

	// allStoresDone latches true once every store stage is fully completed, and
	// allStoresCursor[stageIdx] is the lowest store segment not yet known terminal.
	// Together they make AllStoresCompleted O(1) amortized instead of rescanning
	// the whole completed prefix on every call. See AllStoresCompleted.
	allStoresDone   bool
	allStoresCursor []int

	// nextJobCursor is the lowest segment that may still yield a job. NextJob
	// starts its scan here instead of at globalSegmenter.FirstIndex(), so it no
	// longer re-walks the (ever-growing) prefix of finished segments on every
	// call. See NextJob / segmentMayYieldJob.
	nextJobCursor int
}
type stageStates []UnitState

func NewStages(
	ctx context.Context,
	execGraph *exec.Graph,
	reqPlan *plan.RequestPlan,
	execoutConfigs *execout.Configs,
	storeConfigs store.ConfigMap,
) (out *Stages) {

	logger := reqctx.Logger(ctx)
	out = &Stages{
		ctx:                 ctx,
		logger:              reqctx.Logger(ctx),
		globalSegmenter:     reqPlan.BackprocessSegmenter(),
		outputModuleIsIndex: execGraph.OutputModule().GetKindBlockIndex() != nil,
		execoutConfigs:      execoutConfigs,
		storeConfigs:        storeConfigs,

		hasLinearPipeline: reqPlan.LinearPipeline != nil,
		storeSegmenter:    reqPlan.StoresSegmenter(),
		mapSegmenter:      reqPlan.WriteOutSegmenter(),
	}

	if out.storeSegmenter == nil && out.mapSegmenter == nil {
		panic("internal error: new_stages called without writeExecOut or buildStores")
	}

	modulesInitBlocks := execGraph.ModulesInitBlocks()
	for idx, stageLayers := range execGraph.StagedUsedModules() {
		var allModules, storeModules, mapperModules []string
		for _, layer := range stageLayers {
			for _, mod := range layer {
				allModules = append(allModules, mod.Name)
				if mod.GetKindStore() != nil {
					storeModules = append(storeModules, mod.Name)
				} else {
					mapperModules = append(mapperModules, mod.Name)
				}
			}
		}
		layer := stageLayers.LastLayer()
		kind := layerKind(layer)

		if kind == KindMap && reqPlan.WriteExecOut == nil {
			continue
		}
		if kind == KindStore && reqPlan.BuildStores == nil {
			continue
		}

		var segmenter *block.Segmenter

		if kind == KindMap {
			segmenter = reqPlan.WriteOutSegmenter()
		} else {
			segmenter = reqPlan.StoresSegmenter()
		}

		var moduleStates []*StoreModuleState
		stageLowestInitBlock := modulesInitBlocks[layer[0].Name]
		for _, mod := range layer {
			modSegmenter := segmenter.WithInitialBlock(modulesInitBlocks[mod.Name])
			modState := NewModuleState(logger, mod.Name, modSegmenter, storeConfigs[mod.Name])
			moduleStates = append(moduleStates, modState)

			stageLowestInitBlock = min(stageLowestInitBlock, modulesInitBlocks[mod.Name])
		}

		stageSegmenter := segmenter.WithInitialBlock(stageLowestInitBlock)
		stage := NewStage(idx, kind, stageSegmenter, moduleStates, allModules, storeModules, mapperModules)
		out.stages = append(out.stages, stage)
	}

	out.initSegmentsOffset(reqPlan)
	out.nextJobCursor = out.globalSegmenter.FirstIndex()

	return out
}

func (s *Stages) Close() {
	for _, stage := range s.stages {
		for _, modState := range stage.storeModuleStates {
			modState.Close()
		}
	}
}

func layerKind(layer exec.LayerModules) Kind {
	if layer.IsStoreLayer() {
		return KindStore
	}
	return KindMap
}

func (s *Stages) OutputModuleIsIndex() bool {
	return s.outputModuleIsIndex
}

func (s *Stages) IsFirstMapperJob(segment, stage int) bool {
	if s.mapSegmenter == nil || stage != len(s.stages)-1 {
		return false
	}
	return s.mapSegmenter.FirstIndex() == segment
}

func (s *Stages) FirstMapperSegmentRequiresProcessing() bool {
	return s.getState(Unit{
		Segment: s.mapSegmenter.FirstIndex(),
		Stage:   len(s.stages) - 1,
	}) == UnitPending
}

func (s *Stages) LastStageCompleted() bool {
	lastSegment := s.mapSegmenter.LastIndex()

	idx := len(s.stages) - 1
	for seg := s.mapSegmenter.FirstIndex(); seg <= lastSegment; seg++ {
		state := s.getState(Unit{Segment: seg, Stage: idx})
		if state != UnitCompleted && state != UnitPartialPresent && state != UnitNoOp {
			return false
		}
	}
	return true
}

func (s *Stages) AllStoresCompleted() bool {
	if s.storeSegmenter == nil { // no store at all
		return true
	}
	if s.storeSegmenter.ExclusiveEndBlock() == s.storeSegmenter.InitialBlock() { // first segment on a mapper, no store to process
		return true
	}
	if s.allStoresDone {
		return true
	}
	lastSegment := s.storeSegmenter.LastIndex()

	if s.allStoresCursor == nil {
		s.allStoresCursor = make([]int, len(s.stages))
		for i := range s.allStoresCursor {
			s.allStoresCursor[i] = s.storeSegmenter.FirstIndex()
		}
	}

	for idx, stage := range s.stages {
		if stage.kind != KindStore {
			continue
		}
		// Advance the per-stage cursor over the contiguous prefix of terminal
		// (Completed/NoOp) segments. Both states are permanent, so segments below
		// the cursor never need re-checking again — repeated calls become O(1)
		// amortized instead of O(completed-prefix) (which made this O(n^2) over a run).
		seg := s.allStoresCursor[idx]
		for ; seg <= lastSegment; seg++ {
			state := s.getState(Unit{Segment: seg, Stage: idx})
			if state != UnitCompleted && state != UnitNoOp {
				break
			}
		}
		s.allStoresCursor[idx] = seg
		if seg <= lastSegment {
			return false
		}
	}
	s.allStoresDone = true
	return true
}

// UpdateStats is gated to be called at most once per second. It runs the first time it is called.
func (s *Stages) UpdateStats() {
	if time.Since(s.lastStatUpdate) < 1*time.Second {
		return
	}
	s.lastStatUpdate = time.Now()
	out := make([]*pbsubstreamsrpc.Stage, len(s.stages))

	// The progress view is computed once and feeds both the RPC message and the progress log,
	// so the segment matrix is still walked a single time per update.
	stats := s.computeStageStats()
	progress := s.stagesProgress(stats)
	for stgIdx := range s.stages {
		mods := make([]string, len(s.stages[stgIdx].allExecutedModules))
		_ = copy(mods, s.stages[stgIdx].allExecutedModules)

		out[stgIdx] = &pbsubstreamsrpc.Stage{
			Modules: mods,
			// CompletedRanges counts a store segment as soon as its partial exists, while
			// ReadyUpToExclusive stops at the last squashed one. The gap between the two is
			// what SquashWaitSegmentCount measures.
			CompletedRanges:        toProtoRanges(stats.ranges[stgIdx].Merged()),
			ReadyUpToExclusive:     progress[stgIdx].HighestContiguousBlock,
			SquashWaitSegmentCount: progress[stgIdx].SegmentsReadyForSquashing,
		}
	}

	reqStats := reqctx.ReqStats(s.ctx)
	reqStats.RecordStages(out)
	reqStats.RecordStagesProgress(progress)
}

// stageStats is what a single pass over the segment matrix yields, per stage.
type stageStats struct {
	// ranges are the segments that are in progress or done (Completed/PartialPresent/Merging).
	ranges []block.Ranges
	// contiguousSegment is the highest segment of the uninterrupted prefix of usable
	// segments. "Usable" excludes store partials: a partial that was not squashed yet is
	// not readable as part of the store.
	contiguousSegment []int
	// partialSegments counts the store partials sitting above the contiguous prefix, i.e.
	// work that is done but still waiting for the squasher.
	partialSegments []uint64
}

// computeStageStats walks the segment matrix once. Because segments are visited in
// ascending order the ranges come out already sorted and de-duplicated (one range per
// segment), so callers can Merged() them directly — no per-stage map allocation and no sort.
// The contiguous prefix and the pending-partials count are folded into the same pass to keep
// this O(segments × stages) overall, called at most once per second.
func (s *Stages) computeStageStats() stageStats {
	out := stageStats{
		ranges:            make([]block.Ranges, len(s.stages)),
		contiguousSegment: make([]int, len(s.stages)),
		partialSegments:   make([]uint64, len(s.stages)),
	}
	// Everything below segmentOffset is assumed to have completed, so the contiguous
	// prefix starts there.
	for stgIdx := range s.stages {
		out.contiguousSegment[stgIdx] = s.segmentOffset - 1
	}
	prefixBroken := make([]bool, len(s.stages))

	for segmentIdx, segment := range s.segmentStates {
		absoluteSegment := segmentIdx + s.segmentOffset
		for stgIdx := range s.stages {
			state := segment[stgIdx]
			isStore := s.stages[stgIdx].kind == KindStore
			segmenter := s.stages[stgIdx].storeModuleStates[0].segmenter

			switch state {
			case UnitCompleted, UnitPartialPresent, UnitMerging:
				if rng := segmenter.Range(absoluteSegment); rng != nil {
					out.ranges[stgIdx] = append(out.ranges[stgIdx], rng)
				}
			}

			// A map segment is usable as soon as its partial exec-out file exists (maps are
			// never squashed); a store segment is only usable once it has been merged.
			usable := state == UnitCompleted || state == UnitNoOp || (!isStore && state == UnitPartialPresent)
			if usable && !prefixBroken[stgIdx] {
				out.contiguousSegment[stgIdx] = absoluteSegment
				continue
			}
			prefixBroken[stgIdx] = true

			if isStore && (state == UnitPartialPresent || state == UnitMerging) {
				// Bounds-checked rather than asking for the Range: only the segment's existence
				// matters here, and Range allocates one per call in a loop that already runs
				// once per second over every segment of the run.
				if absoluteSegment >= segmenter.FirstIndex() && absoluteSegment <= segmenter.LastIndex() {
					out.partialSegments[stgIdx]++
				}
			}
		}
	}
	return out
}

// stagesProgress reports, per stage, the whole range of work it is planned to cover for
// this request, and per module, up to which block it is contiguously ready. Stores stop at
// the last squashed segment and report their unsquashed partials separately; mappers and
// indexes count their partial exec-out files as ready, since that is exactly what the
// output stream reads from.
func (s *Stages) stagesProgress(stats stageStats) []metrics.StageProgress {
	firstStreamableBlock := bstream.GetProtocolFirstStreamableBlock

	out := make([]metrics.StageProgress, 0, len(s.stages))
	for stgIdx, stage := range s.stages {
		progress := metrics.StageProgress{
			Stage:   stgIdx,
			Stores:  stage.executedStores,
			Mappers: stage.executedMappers,
		}
		// The stage segmenter comes straight from the request plan, so this is the span of
		// jobs the stage is expected to run over the session, regardless of what the
		// scheduler has picked up so far. Both ranges are nil on an empty stage.
		if first := stage.segmenter.Range(stage.segmenter.FirstIndex()); first != nil {
			if last := stage.segmenter.Range(stage.segmenter.LastIndex()); last != nil {
				progress.PlannedFirstJobStartBlock = first.StartBlock
				progress.PlannedLastJobStopBlock = last.ExclusiveEndBlock
			}
		}

		// A stage is only as advanced as its least advanced module, so report the lowest
		// contiguous block across them rather than a per-module breakdown.
		for _, modState := range stage.storeModuleStates {
			// Base value when nothing was processed yet: where this module starts from.
			highest := max(modState.segmenter.InitialBlock(), firstStreamableBlock)
			if seg := stats.contiguousSegment[stgIdx]; seg >= modState.segmenter.FirstIndex() {
				if rng := modState.segmenter.Range(seg); rng != nil && rng.ExclusiveEndBlock > highest {
					highest = rng.ExclusiveEndBlock
				}
			}
			if progress.HighestContiguousBlock == 0 || highest < progress.HighestContiguousBlock {
				progress.HighestContiguousBlock = highest
			}
		}
		if stage.kind == KindStore {
			progress.SegmentsReadyForSquashing = stats.partialSegments[stgIdx]
		}

		out = append(out, progress)
	}
	return out
}

func toProtoRanges(in block.Ranges) []*pbsubstreamsrpc.BlockRange {
	if len(in) == 0 {
		return nil
	}
	out := make([]*pbsubstreamsrpc.BlockRange, len(in))
	for i := range in {
		out[i] = &pbsubstreamsrpc.BlockRange{
			StartBlock: in[i].StartBlock,
			EndBlock:   in[i].ExclusiveEndBlock,
		}
	}
	return out
}

// TODO: implement the `merged` Progress messages, which will provide
// the progress of the linearly merged stores, so we know if the merger
// is the thing having a hard time moving forward.

func (s *Stages) CmdStartMerge() loop.Cmd {
	var cmds []loop.Cmd
	for idx, stage := range s.stages {
		if stage.kind != KindStore {
			continue
		}
		cmds = append(cmds, s.CmdTryMerge(idx))
	}
	return loop.Batch(cmds...)
}

func (s *Stages) CmdTryMerge(stageIdx int) loop.Cmd {
	// TODO: this function needs to be broken into a message and a few
	// functions, and be called directly within the Scheduler's Update()
	// function, similar to the CmdDownloadCurrentSegment flow, and the
	// NextJob thing.

	if s.AllStoresCompleted() {
		// FIXME: this CmdTryMerge function is called once for each stage,
		// so we could receive multiple such calls, and thus
		// issue multiple MsgAllStoresCompleted. But this signal
		// should be unique, once and for all (it is an indicator that the
		// full job of the Scheduler is done in a way).
		// Here we risk putting out multiple messages of that kind,
		// However, it's probably all right, because it produces a QuitMsg
		// and duplicates of that might just be piled and not read.
		return CmdAllStoresCompleted()
	}

	stage := s.stages[stageIdx]
	if stage.kind != KindStore {
		return nil
	}

	mergeUnit := stage.nextUnit()

	if mergeUnit.Segment > stage.segmenter.LastIndex() {
		return CmdMergeNotReady(mergeUnit, "this stage is done")
	}

	if s.getState(mergeUnit) != UnitPartialPresent {
		return CmdMergeNotReady(mergeUnit, "next unit's partial isn't present")
	}

	if !s.previousUnitComplete(mergeUnit) {
		return CmdMergeNotReady(mergeUnit, "previous unit not complete")
	}

	s.MarkSegmentMerging(mergeUnit)

	return func() loop.Msg {
		if err := s.multiSquash(stage, mergeUnit); err != nil {
			return MsgMergeFailed{Unit: mergeUnit, Error: err}
		}
		return MsgMergeFinished{Unit: mergeUnit}
	}
}

func (s *Stages) MergeCompleted(mergeUnit Unit) {
	s.markSegmentCompleted(mergeUnit)
	s.MoveSegmentCompletedForward(mergeUnit.Stage)
}

func (s *Stages) MoveSegmentCompletedForward(stageIdx int) {
	stage := s.stages[stageIdx]
	for i := stage.segmentCompleted + 1; i <= stage.segmenter.LastIndex(); i++ {
		unit := Unit{Stage: stageIdx, Segment: i}
		if state := s.getState(unit); state == UnitCompleted || state == UnitNoOp {
			stage.segmentCompleted = i
		} else {
			return
		}
	}
}

// initSegmentsOffset marks the first segments as NoOp if they are not required, for
// the Stores stages, or the Mapping stage.
func (s *Stages) initSegmentsOffset(reqPlan *plan.RequestPlan) {
	firstIndex := s.globalSegmenter.FirstIndex()
	s.segmentOffset = firstIndex
	lastStageIndex := len(s.stages) - 1

	// OPTIMIZATION: Let's change the name for `BuildStores` and `ExecOut`. Nowadays, ExecOut is only for
	// mapper output.. so why not align everything: BuildStores, BuildMap, StreamMap, or WriteStores, WriteMap, ReadMap ?
	// all ExecOut could become MapOutput ?

	if reqPlan.WriteExecOut != nil {
		writeOutFirstIndex := reqPlan.WriteOutSegmenter().FirstIndex()
		for i := firstIndex; i < writeOutFirstIndex; i++ {
			// take the last stages layer, and mark the NoOp
			s.allocSegments(i)
			s.setState(Unit{Segment: i, Stage: lastStageIndex}, UnitNoOp)
		}
	}
	if reqPlan.BuildStores != nil {
		storesFirstIndex := reqPlan.StoresSegmenter().FirstIndex()
		for i := firstIndex; i < storesFirstIndex; i++ {
			// take the last stages layer, and mark the NoOp
			for idx := range s.stages {
				if idx >= lastStageIndex {
					continue
				}
				s.allocSegments(i)
				s.setState(Unit{Segment: i, Stage: idx}, UnitNoOp)
			}
			// loop all the Stores layers, and mark them all NoOp up to this point.
		}
	}
}

func (s *Stages) getState(u Unit) UnitState {
	index := u.Segment - s.segmentOffset
	if index >= len(s.segmentStates) {
		return UnitPending
	} else if index < 0 || (len(s.stages) != 0 && u.Segment < s.stages[u.Stage].segmenter.FirstIndex()) {
		return UnitNoOp
	} else {
		return s.segmentStates[index][u.Stage]
	}
}

func (s *Stages) setState(u Unit, state UnitState) {
	s.segmentStates[u.Segment-s.segmentOffset][u.Stage] = state
}

func (s *Stages) WaitAsyncWork() error {
	for _, stage := range s.stages {
		if err := stage.asyncWork.Wait(); err != nil {
			return err
		}
	}
	return nil
}

// segmentMayYieldJob reports whether NextJob could ever still return a job for
// this segment. It is used to advance nextJobCursor over the finished prefix.
//
// A segment can no longer yield a job once every in-range stage's unit is in a
// state that never transitions back to UnitPending:
//   - Completed / NoOp are terminal for any stage;
//   - PartialPresent is terminal only for map stages (maps are never squashed);
//     a store partial can revert to Pending if a squash fails, so it still counts.
//
// Out-of-range stages (segment before the stage's first index or after its last)
// never schedule there, matching the skip/break in NextJob's main loop. Being
// conservative here (returning true when unsure) is always safe: it just keeps the
// cursor a bit further back, never skips a schedulable segment.
func (s *Stages) segmentMayYieldJob(segmentIdx int) bool {
	for stageIdx := range s.stages {
		stage := s.stages[stageIdx]
		if segmentIdx < stage.segmenter.FirstIndex() || segmentIdx > stage.segmenter.LastIndex() {
			continue
		}
		switch s.getState(Unit{Segment: segmentIdx, Stage: stageIdx}) {
		case UnitCompleted, UnitNoOp:
			// permanently done for this stage
		case UnitPartialPresent:
			if stage.kind != KindMap {
				return true
			}
		default:
			// Pending, Scheduled, Merging, Shadowed: may still (re)schedule
			return true
		}
	}
	return false
}

// Returns the unit, its block range and a boolean indicating if we are backing off because of 'notAbove'
func (s *Stages) NextJob(notAboveSegment int) (Unit, *block.Range, bool) {
	// OPTIMIZATION: before calling NextJob, keep a small reserve (10% ?) of workers
	//  so that when a job finishes, it can start immediately a potentially
	//  higher priority one (we'll go do all those first-level jobs
	//  but we want to keep the diagonal balanced).
	//
	// OPTIMIZATION: Another option is to have an algorithm that doesn't return a job
	//  right away when there are too many jobs scheduled before others
	//  in a given stage.

	lastStage := len(s.stages) - 1
	lastSegment := s.globalSegmenter.LastIndex()

	// Advance the scan cursor past the contiguous prefix of segments that can no
	// longer yield a job, so we don't re-walk the finished prefix on every call
	// (which made NextJob O(n^2) over a reprocessing run). The states we skip on
	// are permanent — see segmentMayYieldJob — so the cursor only ever moves forward.
	if s.nextJobCursor < s.globalSegmenter.FirstIndex() {
		s.nextJobCursor = s.globalSegmenter.FirstIndex()
	}
	for s.nextJobCursor <= lastSegment && !s.segmentMayYieldJob(s.nextJobCursor) {
		s.nextJobCursor++
	}

	for segmentIdx := s.nextJobCursor; segmentIdx <= lastSegment; segmentIdx++ {
		someShadowed := s.markShadowedUnits(segmentIdx)
		for stageIdx := lastStage; stageIdx >= 0; stageIdx-- {
			stage := s.stages[stageIdx]
			unit := Unit{Segment: segmentIdx, Stage: stageIdx}
			segmentState := s.getState(unit)
			if segmentState != UnitPending {
				continue
			}
			if segmentIdx < stage.segmenter.FirstIndex() {
				// Don't process stages where all modules' initial blocks are only later
				continue
			}
			if segmentIdx > stage.segmenter.LastIndex() {
				break
			}
			if !s.dependenciesCompleted(unit) {
				continue
			}

			r := stage.segmenter.Range(unit.Segment)
			if r.Len() == 0 {
				// empty units get marked as completed automatically
				s.markSegmentCompleted(unit)
				continue
			}

			if someShadowed && stageIdx == lastStage {
				for i := 0; i < len(s.stages); i++ {
					u := Unit{Segment: segmentIdx, Stage: i}
					if st := s.getState(u); st == UnitPending {
						s.markSegmentScheduled(u)
						return u, r, false
					}
				}
			}

			if segmentIdx > notAboveSegment {
				return Unit{}, nil, true
			}

			s.markSegmentScheduled(unit)
			return unit, r, false
		}
	}
	return Unit{}, nil, false
}

// setShadowableSegment sets the value to the first segment between
// "segmentOffset" and "startBlockSegment" (incl.) for which all the dependencies are completed
func (s *Stages) setShadowableSegment(startBlockSegment int) {
	s.shadowableSegment = s.segmentOffset
	if len(s.stages) < 2 {
		return
	}
	for seg := s.segmentOffset + 1; seg <= startBlockSegment; seg++ {
		for stg := 0; stg <= len(s.stages)-2; stg++ {
			if !s.previousUnitComplete(Unit{Segment: seg, Stage: stg}) {
				return
			}
		}
		s.shadowableSegment = seg
	}
}

func (s *Stages) shadowable(segmentIdx int) bool {
	if len(s.stages) < 2 {
		return false
	}
	return segmentIdx-s.shadowableSegment <= len(s.stages)-1
}

func (s *Stages) markShadowedUnits(segmentIdx int) (someShadowed bool) {
	if !s.shadowable(segmentIdx) {
		return
	}

	relSegmentOrdinal := segmentIdx - s.shadowableSegment
	s.allocSegments(segmentIdx)

	lastStage := len(s.stages) - 1
	for stageIdx := lastStage - 1; stageIdx >= relSegmentOrdinal && stageIdx >= 0; stageIdx-- { // skip the last stage
		unit := Unit{Segment: segmentIdx, Stage: stageIdx}
		segmentState := s.getState(unit)
		if segmentState != UnitCompleted && segmentState != UnitNoOp {
			// A unit can only be shadowed by a job of the stage above that has not run
			// yet: that job executes this stage too and reports it on success. A unit
			// above that is Merging or PartialPresent got its partial from disk, no job
			// will ever come back for the shadowed one.
			nextState := s.getState(Unit{Segment: segmentIdx, Stage: stageIdx + 1})
			if nextState == UnitPending || nextState == UnitScheduled || nextState == UnitShadowed {
				s.setState(unit, UnitShadowed)
				someShadowed = true
			}
		}
	}
	return
}

func (s *Stages) allocSegments(segmentIdx int) {
	segmentsNeeded := segmentIdx - s.segmentOffset
	if len(s.segmentStates) > segmentsNeeded {
		return
	}
	by := segmentsNeeded - len(s.segmentStates) + 1
	for i := 0; i < by; i++ {
		s.segmentStates = append(s.segmentStates, make([]UnitState, len(s.stages)))
	}
}

func (s *Stages) dependenciesCompleted(u Unit) bool {
	if u.Stage == 0 {
		return true
	}

	// A job loads the lower stages' fullKV at its segment start block, which is only
	// guaranteed to exist once the previous segment of every lower stage is done. A
	// completed unit says nothing about the segments before it: snapshots may be pruned.
	for i := u.Stage - 1; i >= 0; i-- {
		if !s.previousUnitComplete(Unit{Segment: u.Segment, Stage: i}) {
			return false
		}
		state := s.getState(Unit{Segment: u.Segment, Stage: i})
		switch state {
		case UnitCompleted, UnitNoOp, UnitShadowed, UnitPartialPresent:
		default:
			return false
		}
	}
	return true
}

func (s *Stages) previousUnitComplete(u Unit) bool {
	state := s.getState(Unit{Segment: u.Segment - 1, Stage: u.Stage})
	return state == UnitCompleted || state == UnitNoOp
}

type loadedStore struct {
	name string
	kv   *store.FullKV
	err  error
}

func (s *Stages) FinalStoreMap(exclusiveEndBlock uint64) (store.Map, error) {

	var storeModuleStates []*StoreModuleState
	for _, stage := range s.stages {
		if stage.kind != KindStore {
			continue
		}
		for _, modState := range stage.storeModuleStates {
			storeModuleStates = append(storeModuleStates, modState)
		}
	}

	out := store.NewMap()
	if len(storeModuleStates) == 0 {
		return out, nil
	}

	loadingChan := make(chan loadedStore, len(storeModuleStates))

	storesMetadata := make(map[string]map[string]string)
	var approxStoreSize uint64
	for _, modState := range storeModuleStates {
		size, metadata, err := modState.estimateStoreSizeBytes(s.ctx, exclusiveEndBlock)
		if err != nil {
			return nil, err
		}
		approxStoreSize += size
		storesMetadata[modState.name] = metadata
	}

	s.logger.Info("about to load stores", zap.String("approx_store_size", humanize.IBytes(approxStoreSize)))

	if reqHandler := reqctx.ActiveRequestsHandler(s.ctx); reqHandler != nil {
		reqHandler.AllocateFullKVSizeOrForceCancelRequest(approxStoreSize)
		if s.ctx.Err() != nil {
			return nil, s.ctx.Err()
		}
	}

	for _, modState := range storeModuleStates {
		go func() {
			fullKV, err := modState.getStore(s.ctx, exclusiveEndBlock)
			loaded := loadedStore{
				name: modState.name,
				kv:   fullKV,
				err:  err,
			}
			select {
			case loadingChan <- loaded:
			case <-s.ctx.Done():
				return
			}

			if err == nil {
				//  add loaded file size to metadata
				met := storesMetadata[modState.name]
				if met == nil {
					met = make(map[string]string)
				}
				if met["datasize"] == "" {
					met["datasize"] = fmt.Sprintf("%d", fullKV.SizeBytes())
				}
				// Detached metadata write: does not retain fullKV or ride the
				// request ctx (see store.SetMetadataDetached). Tier1 twin of the
				// tier2 fix in pipeline.setupSubrequestStores.
				store.SetMetadataDetached(fullKV.Store(), fullKV.Filename(), modState.name, met, s.logger)
			}
		}()
	}

	var actualRequestStoresSize uint64
	for len(out) < len(storeModuleStates) {
		select {
		case <-s.ctx.Done():
			return nil, s.ctx.Err()
		case loaded, ok := <-loadingChan:
			if !ok {
				break
			}
			if loaded.err != nil {
				return nil, fmt.Errorf("while loading %s: %w", loaded.name, loaded.err)
			}
			out[loaded.name] = loaded.kv
			actualRequestStoresSize += loaded.kv.SizeBytes()
		}
	}

	if reqHandler := reqctx.ActiveRequestsHandler(s.ctx); reqHandler != nil {
		s.logger.Info("adjusting to stores size", zap.Uint64("actual_store_size", actualRequestStoresSize/1024/1024))
		reqHandler.AdjustFullKVSize(actualRequestStoresSize)
	}

	// Ownership of the loaded stores transfers to the returned map: the linear
	// pipeline keeps writing to them and closes them at request end. Detach
	// them from the module states so Stages.Close() does not close stores
	// still in use.
	for _, modState := range storeModuleStates {
		modState.cachedStore = nil
	}

	return out, nil
}

func (s *Stages) BlocksToProcess(headBlockNum uint64) (beforeStartBlock, effectiveBeforeStartBlock, afterEndBlock, effectiveEndBlock uint64) {
	details := reqctx.Details(s.ctx)
	startSegment := s.globalSegmenter.IndexForStartBlock(details.ResolvedStartBlockNum)
	var segmentsBefore int
	var segmentsAfter int
	var effectiveSegmentsBefore int
	var effectiveSegmentsAfter int

	for i := range s.stages {

		cur := s.segmentOffset
		for _, segment := range s.segmentStates {
			cached := segment[i] != UnitPending && segment[i] != UnitScheduled

			if cur < startSegment {
				if s.stages[i].kind == KindStore {
					segmentsBefore++
					if !cached {
						effectiveSegmentsBefore++
					}
				}
			} else {
				segmentsAfter++
				if !cached {
					effectiveSegmentsAfter++
				}
			}
			cur++
		}

		for cur < startSegment {
			if s.stages[i].kind == KindStore {
				segmentsBefore++
				effectiveSegmentsBefore++
			}
			cur++
		}

		for cur <= s.stages[i].segmenter.LastIndex() {
			segmentsAfter++
			effectiveSegmentsAfter++
			cur++
		}

	}
	stopBlock := details.StopBlockNum
	if stopBlock == 0 {
		stopBlock = headBlockNum
	}

	var extraBlocks uint64
	rangeEndBlock := (s.globalSegmenter.Range(s.globalSegmenter.LastIndex()).ExclusiveEndBlock)
	if stopBlock > rangeEndBlock {
		extraBlocks = stopBlock - rangeEndBlock // blocks processed in linear mode...
	}

	return uint64(segmentsBefore) * s.globalSegmenter.Interval(),
		uint64(effectiveSegmentsBefore) * s.globalSegmenter.Interval(),
		uint64(segmentsAfter)*s.globalSegmenter.Interval() + extraBlocks,
		uint64(effectiveSegmentsAfter)*s.globalSegmenter.Interval() + extraBlocks
}

func (s *Stages) StatesString() string {
	out := strings.Builder{}
	for i := range s.stages {
		if s.stages[i].kind == KindMap {
			out.WriteString("M:")
		} else {
			out.WriteString("S:")
		}
		for _, segment := range s.segmentStates {
			out.WriteString(map[UnitState]string{
				UnitPending:        ".",
				UnitPartialPresent: "P",
				UnitScheduled:      "S",
				UnitMerging:        "M",
				UnitCompleted:      "C",
				UnitNoOp:           "N",
				UnitShadowed:       "Z",
			}[segment[i]])
		}
		out.WriteString("\n")
	}
	return out.String()
}

var stringRepr = map[UnitState]string{
	UnitPending:        ".",
	UnitPartialPresent: "P",
	UnitScheduled:      "S",
	UnitMerging:        "M",
	UnitCompleted:      "C",
	UnitNoOp:           "N",
	UnitShadowed:       "Z",
}

func (s *Stages) StageModules(stage int) (out []string) {
	for _, modState := range s.stages[stage].storeModuleStates {
		out = append(out, modState.name)
	}
	return
}

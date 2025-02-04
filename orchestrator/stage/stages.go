package stage

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/streamingfast/substreams/block"
	"github.com/streamingfast/substreams/orchestrator/loop"
	"github.com/streamingfast/substreams/orchestrator/plan"
	pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
	"github.com/streamingfast/substreams/pipeline/exec"
	"github.com/streamingfast/substreams/reqctx"
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

	globalSegmenter *block.Segmenter // This segmenter covers both the stores and the mapper
	storeSegmenter  *block.Segmenter // This segmenter covers only jobs needed to build up stores according to the RequestPlan.
	mapSegmenter    *block.Segmenter // This segmenter covers only what is needed to produce the mapper output for the FileWalker.

	stages []*Stage

	// segmentStates is a matrix of segment and stages
	segmentStates       []stageStates // segmentStates[offsetSegment][StageIndex]
	lastStatUpdate      time.Time
	outputModuleIsIndex bool

	// If you're processing at 12M blocks, offset 12,000 segments, so you don't need to allocate 12k empty elements.
	// Any previous segment is assumed to have completed successfully, and any stores that we sync'd prior to this offset
	// are assumed to have been either fully loaded, or merged up until this offset.
	segmentOffset int

	// first segment where we can run directly the higher stages (shadowing the lower stages)
	shadowableSegment int
}
type stageStates []UnitState

func NewStages(
	ctx context.Context,
	execGraph *exec.Graph,
	reqPlan *plan.RequestPlan,
	storeConfigs store.ConfigMap,
) (out *Stages) {

	if !reqPlan.RequiresParallelProcessing() {
		panic("internal error: new_stages should never be called outside of parallel processing")
	}

	logger := reqctx.Logger(ctx)

	stagedModules := execGraph.StagedUsedModules()
	modulesInitBlocks := execGraph.ModulesInitBlocks()
	out = &Stages{
		ctx:                 ctx,
		logger:              reqctx.Logger(ctx),
		globalSegmenter:     reqPlan.BackprocessSegmenter(),
		outputModuleIsIndex: execGraph.OutputModule().GetKindBlockIndex() != nil,
	}
	if reqPlan.BuildStores != nil {
		out.storeSegmenter = reqPlan.StoresSegmenter()
	}
	if reqPlan.WriteExecOut != nil {
		out.mapSegmenter = reqPlan.WriteOutSegmenter()
	}
	for idx, stageLayers := range stagedModules {
		var allModules []string
		for _, layer := range stageLayers {
			for _, mod := range layer {
				allModules = append(allModules, mod.Name)
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
		stage := NewStage(idx, kind, stageSegmenter, moduleStates, allModules)
		out.stages = append(out.stages, stage)
	}

	out.initSegmentsOffset(reqPlan)

	return out
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
	lastSegment := s.storeSegmenter.LastIndex()

	for idx, stage := range s.stages {
		if stage.kind != KindStore {
			continue
		}
		for seg := s.storeSegmenter.FirstIndex(); seg <= lastSegment; seg++ {
			state := s.getState(Unit{Segment: seg, Stage: idx})
			if state != UnitCompleted && state != UnitNoOp {
				return false
			}
		}
	}
	return true
}

// UpdateStats is gated to be called at most once per second. It runs the first time it is called.
func (s *Stages) UpdateStats() {
	if time.Since(s.lastStatUpdate) < 1*time.Second {
		return
	}
	s.lastStatUpdate = time.Now()
	out := make([]*pbsubstreamsrpc.Stage, len(s.stages))

	for stgIdx := range s.stages {

		mods := make([]string, len(s.stages[stgIdx].allExecutedModules))
		_ = copy(mods, s.stages[stgIdx].allExecutedModules)

		br := make(map[uint64]*block.Range)
		for segmentIdx, segment := range s.segmentStates {
			state := segment[stgIdx]
			segmenter := s.stages[stgIdx].storeModuleStates[0].segmenter
			if state == UnitCompleted || state == UnitPartialPresent || state == UnitMerging {
				if rng := segmenter.Range(segmentIdx + s.segmentOffset); rng != nil {
					br[rng.StartBlock] = rng
				}
			}
		}

		blockRanges := block.Ranges(make([]*block.Range, len(br)))
		i := 0
		for _, v := range br {
			blockRanges[i] = v
			i++
		}
		sort.Sort(blockRanges)
		blockRanges = blockRanges.Merged()

		out[stgIdx] = &pbsubstreamsrpc.Stage{
			Modules:         mods,
			CompletedRanges: toProtoRanges(blockRanges),
		}
	}

	reqctx.ReqStats(s.ctx).RecordStages(out)
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
	for i := stage.segmentCompleted + 1; i < stage.segmenter.LastIndex(); i++ {
		unit := Unit{Stage: stageIdx, Segment: i}
		if s.getState(unit) == UnitCompleted {
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
				if idx == lastStageIndex {
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

func (s *Stages) NextJob() (Unit, *block.Range) {
	// OPTIMIZATION: before calling NextJob, keep a small reserve (10% ?) of workers
	//  so that when a job finishes, it can start immediately a potentially
	//  higher priority one (we'll go do all those first-level jobs
	//  but we want to keep the diagonal balanced).
	//
	// OPTIMIZATION: Another option is to have an algorithm that doesn't return a job
	//  right away when there are too much jobs scheduled before others
	//  in a given stage.
	//
	// OPTIMIZATION: eventually, we can push `segmentsOffset`
	//  each time contiguous segments are completed for all stages.

	lastStage := len(s.stages) - 1
	for segmentIdx := s.globalSegmenter.FirstIndex(); segmentIdx <= s.globalSegmenter.LastIndex(); segmentIdx++ {
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
						return u, r
					}
				}
			}

			s.markSegmentScheduled(unit)
			return unit, r
		}
	}
	return Unit{}, nil
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
			nextState := s.getState(Unit{Segment: segmentIdx, Stage: stageIdx + 1})
			if nextState == UnitPending || nextState == UnitScheduled || nextState == UnitMerging || nextState == UnitShadowed {
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
	if u.Segment <= s.stages[u.Stage].segmenter.FirstIndex() {
		return true
	}
	if u.Stage == 0 {
		return true
	}

	previousSegmentParent := s.getState(Unit{Segment: u.Segment - 1, Stage: u.Stage - 1})

	for i := u.Stage - 1; i >= 0; i-- {
		state := s.getState(Unit{Segment: u.Segment, Stage: i})
		switch state {
		case UnitCompleted, UnitNoOp:
		case UnitShadowed, UnitPartialPresent:
			// if the direct parent stage is shadowed or UnitPartialPresent, we need the previous segment's previous stage to be completed
			if previousSegmentParent != UnitCompleted && previousSegmentParent != UnitNoOp {
				return false
			}
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

func (s *Stages) FinalStoreMap(exclusiveEndBlock uint64) (store.Map, error) {
	out := store.NewMap()
	for _, stage := range s.stages {
		if stage.kind != KindStore {
			continue
		}
		for _, modState := range stage.storeModuleStates {
			fullKV, err := modState.getStore(s.ctx, exclusiveEndBlock)
			if err != nil {
				return nil, fmt.Errorf("stores didn't sync up properly, expected store %q to be at block %d but was at %d: %w", modState.name, exclusiveEndBlock, modState.lastBlockInStore, err)
			}
			out[modState.name] = fullKV
		}
	}
	return out, nil
}

func (s *Stages) StatesString() string {
	out := strings.Builder{}
	for i := 0; i < len(s.stages); i++ {
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

func (s *Stages) StageModules(stage int) (out []string) {
	for _, modState := range s.stages[stage].storeModuleStates {
		out = append(out, modState.name)
	}
	return
}

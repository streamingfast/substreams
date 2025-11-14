package stage

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/dustin/go-humanize"
	"github.com/streamingfast/substreams/block"
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

// Returns the unit, its block range and a boolean indicating if we are backing off because of 'notAbove'
func (s *Stages) NextJob(notAboveSegment int) (Unit, *block.Range, bool) {
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
		modState := modState
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
				go func() {
					err := fullKV.Store().SetMetadata(s.ctx, fullKV.Filename(), met)
					if err != nil {
						s.logger.Warn("failed to set metadata on store", zap.String("store_name", modState.name), zap.String("filename", fullKV.Filename()), zap.Error(err))
					}
				}()
			}
		}()
	}

	var errs error
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
				errs = errors.Join(errs, fmt.Errorf("while loading %s: %w", loaded.name, loaded.err))
				break
			}
			out[loaded.name] = loaded.kv
			actualRequestStoresSize += loaded.kv.SizeBytes()
		}
	}

	if reqHandler := reqctx.ActiveRequestsHandler(s.ctx); reqHandler != nil {
		s.logger.Info("adjusting to stores size", zap.Uint64("actual_store_size", actualRequestStoresSize/1024/1024))
		reqHandler.AdjustFullKVSize(actualRequestStoresSize)
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

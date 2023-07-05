package stage

import (
	"context"
	"fmt"
	"strings"

	"go.uber.org/zap"

	"github.com/streamingfast/substreams/block"
	"github.com/streamingfast/substreams/orchestrator/loop"
	"github.com/streamingfast/substreams/orchestrator/plan"
	"github.com/streamingfast/substreams/pipeline/outputmodules"
	"github.com/streamingfast/substreams/reqctx"
	"github.com/streamingfast/substreams/storage/store"
	"github.com/streamingfast/substreams/utils"
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
	ctx     context.Context
	logger  *zap.Logger
	traceID string

	globalSegmenter *block.Segmenter // This segmenter covers both the stores and the mapper
	storeSegmenter  *block.Segmenter // This segmenter covers only jobs needed to build up stores according to the RequestPlan.
	mapSegmenter    *block.Segmenter // This segmenter covers only what is needed to produce the mapper output for the FileWalker.

	stages []*Stage

	// segmentStates is a matrix of segment and stages
	segmentStates []stageStates // segmentStates[offsetSegment][StageIndex]

	// If you're processing at 12M blocks, offset 12,000 segments, so you don't need to allocate 12k empty elements.
	// Any previous segment is assumed to have completed successfully, and any stores that we sync'd prior to this offset
	// are assumed to have been either fully loaded, or merged up until this offset.
	segmentOffset int
}
type stageStates []UnitState

func NewStages(
	ctx context.Context,
	outputGraph *outputmodules.Graph,
	reqPlan *plan.RequestPlan,
	storeConfigs store.ConfigMap,
	traceID string,
) (out *Stages) {
	logger := reqctx.Logger(ctx)

	stagedModules := outputGraph.StagedUsedModules()
	out = &Stages{
		ctx:             ctx,
		traceID:         traceID,
		logger:          reqctx.Logger(ctx),
		globalSegmenter: reqPlan.BackprocessSegmenter(),
	}
	if reqPlan.BuildStores != nil {
		out.storeSegmenter = reqPlan.StoresSegmenter()
	}
	if reqPlan.WriteExecOut != nil {
		out.mapSegmenter = reqPlan.WriteOutSegmenter()
	}
	for idx, stageLayer := range stagedModules {
		mods := stageLayer.LastLayer()
		kind := layerKind(mods)

		if kind == KindMap && reqPlan.WriteExecOut == nil {
			continue
		}
		if kind == KindStore && reqPlan.BuildStores == nil {
			continue
		}

		// TODO: what will happen if we have only a single _mapper_  module, and
		// no stores? will the BuildStores requestPlan field be defined?
		// Right now it is, but what if we had it distinct, and clearly distinct everywhere?

		segmenter := reqPlan.StoresSegmenter()
		if kind == KindMap {
			segmenter = reqPlan.WriteOutSegmenter()
		}

		var moduleStates []*ModuleState
		stageLowestInitBlock := mods[0].InitialBlock
		for _, mod := range mods {
			modSegmenter := segmenter.WithInitialBlock(mod.InitialBlock)
			modState := NewModuleState(logger, mod.Name, modSegmenter, storeConfigs[mod.Name])
			moduleStates = append(moduleStates, modState)

			stageLowestInitBlock = utils.MinOf(stageLowestInitBlock, mod.InitialBlock)
		}

		stageSegmenter := segmenter.WithInitialBlock(stageLowestInitBlock)
		stage := NewStage(idx, kind, stageSegmenter, moduleStates)
		out.stages = append(out.stages, stage)
	}

	out.initSegmentsOffset(reqPlan)

	return out
}

func layerKind(layer outputmodules.LayerModules) Kind {
	if layer.IsStoreLayer() {
		return KindStore
	}
	return KindMap
}

func (s *Stages) AllStoresCompleted() bool {
	lastSegment := s.storeSegmenter.LastIndex()

	for idx, stage := range s.stages {
		if stage.kind != KindStore {
			continue
		}
		if s.getState(Unit{Segment: lastSegment, Stage: idx}) != UnitCompleted {
			return false
		}
	}
	return true
}

func (s *Stages) InitialProgressMessages() map[string]block.Ranges {
	out := make(map[string]block.Ranges)
	for segmentIdx, segment := range s.segmentStates {
		for stageIdx, state := range segment {
			if state == UnitCompleted {
				for _, mod := range s.stages[stageIdx].moduleStates {
					rng := mod.segmenter.Range(segmentIdx + s.segmentOffset)
					if rng != nil {
						out[mod.name] = append(out[mod.name], rng)
					}
				}
			}
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
		return func() loop.Msg {
			return MsgAllStoresCompleted{}
		}
	}

	stage := s.stages[stageIdx]
	if stage.kind != KindStore {
		return nil
	}

	mergeUnit := stage.nextUnit()

	if mergeUnit.Segment > s.storeSegmenter.LastIndex() {
		return nil // We're done here.
	}

	if s.getState(mergeUnit) != UnitPartialPresent {
		return nil
	}

	if !s.previousUnitComplete(mergeUnit) {
		return nil
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
	s.stages[mergeUnit.Stage].markSegmentCompleted(mergeUnit.Segment)
	s.markSegmentCompleted(mergeUnit)
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
	} else if index < 0 {
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

	for segmentIdx := s.globalSegmenter.FirstIndex(); segmentIdx <= s.globalSegmenter.LastIndex(); segmentIdx++ {
		for stageIdx := len(s.stages) - 1; stageIdx >= 0; stageIdx-- {
			stage := s.stages[stageIdx]
			unit := Unit{Segment: segmentIdx, Stage: stageIdx}
			segmentState := s.getState(unit)
			if segmentState != UnitPending {
				continue
			}
			if segmentState == UnitNoOp {
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

			s.markSegmentScheduled(unit)
			return unit, stage.segmenter.Range(unit.Segment)
		}
	}
	return Unit{}, nil
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
	for i := u.Stage - 1; i >= 0; i-- {
		state := s.getState(Unit{Segment: u.Segment - 1, Stage: i})
		if !(state == UnitCompleted || state == UnitNoOp) {
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
		for _, modState := range stage.moduleStates {
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
			}[segment[i]])
		}
		out.WriteString("\n")
	}
	return out.String()
}

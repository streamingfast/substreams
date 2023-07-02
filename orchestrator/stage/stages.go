package stage

import (
	"context"
	"fmt"
	"strings"

	"go.uber.org/zap"

	"github.com/streamingfast/substreams/block"
	"github.com/streamingfast/substreams/orchestrator/loop"
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

	segmenter *block.Segmenter

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
	segmenter *block.Segmenter,
	storeConfigs store.ConfigMap,
	traceID string,
) (out *Stages) {
	logger := reqctx.Logger(ctx)

	stagedModules := outputGraph.StagedUsedModules()
	out = &Stages{
		ctx:           ctx,
		traceID:       traceID,
		segmenter:     segmenter,
		segmentOffset: segmenter.IndexForStartBlock(outputGraph.LowestInitBlock()),
		logger:        reqctx.Logger(ctx),
	}
	for idx, stageLayer := range stagedModules {
		mods := stageLayer.LastLayer()

		kind := KindMap
		if mods.IsStoreLayer() {
			kind = KindStore
		}

		var moduleStates []*ModuleState
		lowestStageInitBlock := mods[0].InitialBlock
		for _, mod := range mods {
			modSegmenter := segmenter.WithInitialBlock(mod.InitialBlock)
			modState := NewModuleState(logger, mod.Name, modSegmenter, storeConfigs[mod.Name])
			moduleStates = append(moduleStates, modState)

			lowestStageInitBlock = utils.MinOf(lowestStageInitBlock, mod.InitialBlock)
		}

		stageSegmenter := segmenter.WithInitialBlock(lowestStageInitBlock)
		stage := NewStage(idx, kind, stageSegmenter, moduleStates)
		out.stages = append(out.stages, stage)
	}
	return out
}

func (s *Stages) AllStagesFinished() bool {
	lastSegment := s.segmenter.LastIndex()
	lastSegmentIndex := lastSegment - s.segmentOffset
	if lastSegmentIndex >= len(s.segmentStates) {
		return false
	}

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
	if s.AllStagesFinished() {
		// FIXME: this CmdTryMerge function is called once for each stage,
		// so we could receive multiple such calls, and thus
		// issue multiple MsgMergeStoresCompleted. But this signal
		// should be unique, once and for all (it is an indicator that the
		// full job of the Scheduler is done in a way).
		// Here we risk putting out multiple messages of that kind,
		// However, it's probably all right, because it produces a QuitMsg
		// and duplicates of that might just be piled and not read.
		return func() loop.Msg {
			return MsgMergeStoresCompleted{}
		}
	}

	stage := s.stages[stageIdx]
	if stage.kind != KindStore {
		fmt.Println("TRYM: kindnot store")
		return nil
	}

	mergeUnit := stage.nextUnit()

	if mergeUnit.Segment > s.segmenter.LastIndex() {
		fmt.Println("TRYM: past last segment")

		return nil // We're done here.
	}

	if s.getState(mergeUnit) != UnitPartialPresent {
		fmt.Println("TRYM: wasn't in partial state")
		return nil
	}

	if !s.previousUnitComplete(mergeUnit) {
		fmt.Println("TRYM: prev unit not complete")
		return nil
	}

	s.MarkSegmentMerging(mergeUnit)

	return func() loop.Msg {
		fmt.Println("TRYM: launching multiSquash", stage, mergeUnit)
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

func (s *Stages) getState(u Unit) UnitState {
	index := u.Segment - s.segmentOffset
	if index >= len(s.segmentStates) {
		return UnitPending
	}
	return s.segmentStates[index][u.Stage]
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
	segmentIdx := s.segmenter.FirstIndex()
	for {
		if segmentIdx > s.segmenter.LastIndex() {
			break
		}
		for stageIdx := len(s.stages) - 1; stageIdx >= 0; stageIdx-- {
			unit := Unit{Segment: segmentIdx, Stage: stageIdx}
			segmentState := s.getState(unit)
			if segmentState != UnitPending {
				continue
			}
			if segmentIdx < s.stages[stageIdx].segmenter.FirstIndex() {
				// Don't process stages where all modules' initial blocks are only later
				continue
			}
			if !s.dependenciesCompleted(unit) {
				continue
			}

			s.markSegmentScheduled(unit)
			return unit, s.segmenter.Range(unit.Segment)
		}
		segmentIdx++
	}
	return Unit{}, nil
}

func (s *Stages) allocSegments(segmentIdx int) {
	if len(s.segmentStates) > segmentIdx-s.segmentOffset {
		return
	}
	by := len(s.segmentStates)
	if by == 0 {
		by = 2
	}
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
		if s.getState(Unit{Segment: u.Segment - 1, Stage: i}) != UnitCompleted {
			return false
		}
	}
	return true
}

func (s *Stages) previousUnitComplete(u Unit) bool {
	if u.Segment-s.segmentOffset <= 0 {
		return true
	}
	return s.getState(Unit{Segment: u.Segment - 1, Stage: u.Stage}) == UnitCompleted
}

func (s *Stages) FinalStoreMap(exclusiveEndBlock uint64) (store.Map, error) {
	out := store.NewMap()
	for _, stage := range s.stages {
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
			}[segment[i]])
		}
		out.WriteString("\n")
	}
	return out.String()
}

package stage

import (
	"github.com/streamingfast/substreams/block"
	"github.com/streamingfast/substreams/pipeline/outputmodules"
)

type Stages struct {
	segmenter *block.Segmenter

	stages []*Stage

	// segmentStates is a matrix of segment and stages
	segmentStates []stageStates // segmentStates[offsetSegment][StageIndex]

	// If you're processing at 12M blocks, offset 12,000 segments so you don't need to allocate 12k empty elements.
	// Any previous segment is assumed to have completed successfully, and any stores that we sync'd prior to this offset
	// are assumed to have been either fully loaded, or merged up until this offset.
	segmentOffset int
}
type stageStates []UnitState

func NewStages(
	outputGraph *outputmodules.Graph,
	segmenter *block.Segmenter,
) (out *Stages) {
	stagedModules := outputGraph.StagedUsedModules()
	lastIndex := len(stagedModules) - 1
	out = &Stages{
		segmenter:     segmenter,
		segmentOffset: segmenter.IndexForBlock(outputGraph.LowestInitBlock()),
	}
	for idx, mods := range stagedModules {
		isLastStage := idx == lastIndex
		kind := stageKind(mods)
		if kind == KindMap && !isLastStage {
			continue
		}
		stage := &Stage{
			kind: kind,
		}
		lowestStageInitBlock := mods[0].InitialBlock
		for _, mod := range mods {
			stage.moduleStates = append(stage.moduleStates, &ModuleState{
				segmenter: segmenter.WithInitialBlock(mod.InitialBlock),
				name:      mod.Name,
			})
			if lowestStageInitBlock > mod.InitialBlock {
				lowestStageInitBlock = mod.InitialBlock
			}
		}

		stage.segmenter = segmenter.WithInitialBlock(lowestStageInitBlock)

		out.stages = append(out.stages, stage)
	}
	return out
}

func (s *Stages) GetState(u Unit) UnitState {
	return s.segmentStates[u.Segment-s.segmentOffset][u.Stage]
}

func (s *Stages) setState(u Unit, state UnitState) {
	s.segmentStates[u.Segment-s.segmentOffset][u.Stage] = state
}

func (s *Stages) NextJob() *Unit {
	// TODO: before calling NextJob, keep a small reserve (10% ?) of workers
	//  so that when a job finishes, it can start immediately a potentially
	//  higher priority one (we'll go do all those first-level jobs
	//  but we want to keep the diagonal balanced).
	// TODO: Another option is to have an algorithm that doesn't return a job
	//  right away when there are too much jobs scheduled before others
	//  in a given stage.

	// FIXME: eventually, we can start from s.segmentsOffset, and push `segmentsOffset`
	//  each time contiguous segments are completed for all stages.
	segmentIdx := s.segmenter.FirstIndex()
	for {
		if len(s.segmentStates) <= segmentIdx-s.segmentOffset {
			s.growSegments()
		}
		if segmentIdx > s.segmenter.LastIndex() {
			break
		}
		for stageIdx := len(s.stages) - 1; stageIdx >= 0; stageIdx-- {
			unit := Unit{Segment: segmentIdx, Stage: stageIdx}
			segmentState := s.GetState(unit)
			if segmentState != UnitPending {
				continue
			}
			if segmentIdx < s.stages[stageIdx].segmenter.FirstIndex() {
				// Don't process stages where all modules's initial blocks are only later
				continue
			}
			if !s.dependenciesCompleted(unit) {
				continue
			}

			s.markSegmentScheduled(unit)
			return &unit
		}
		segmentIdx++
	}
	return nil
}

func (s *Stages) growSegments() {
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
		if s.GetState(Unit{Segment: u.Segment - 1, Stage: i}) != UnitCompleted {
			return false
		}
	}
	return true
}

func (s *Stages) previousUnitComplete(u Unit) bool {
	if u.Segment-s.segmentOffset <= 0 {
		return true
	}
	return s.GetState(Unit{Segment: u.Segment - 1, Stage: u.Stage}) == UnitCompleted
}

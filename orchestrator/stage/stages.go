package stage

import (
	"github.com/streamingfast/substreams/block"
	"github.com/streamingfast/substreams/pipeline/outputmodules"
)

type Stages struct {
	*block.Segmenter

	stages []*Stage

	// segmentStates is a matrix of segment and stages
	segmentStates []stageStates // segmentStates[SegmentIndex][StageIndex]
}
type stageStates []UnitState

func NewStages(
	outputGraph *outputmodules.Graph,
	segmenter *block.Segmenter,
) (out *Stages) {
	stagedModules := outputGraph.StagedUsedModules()
	lastIndex := len(stagedModules) - 1
	out = &Stages{
		Segmenter: segmenter,
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

func (s *Stages) NextJob() *Unit {
	// TODO: before calling NextJob, keep a small reserve (10% ?) of workers
	//  so that when a job finishes, it can start immediately a potentially
	//  higher priority one (we'll go do all those first-level jobs
	//  but we want to keep the diagonal balanced).
	// TODO: Another option is to have an algorithm that doesn't return a job
	//  right away when there are too much jobs scheduled before others
	//  in a given stage.

	// FIXME: eventually, we can start from s.completedSegments, and push `completedSegments`
	// each time contiguous segments are completed for all stages.
	segmentIdx := 0
	for {
		if len(s.segmentStates) <= segmentIdx {
			s.growSegments()
		}
		if segmentIdx > s.LastIndex() {
			break
		}
		for stageIdx := len(s.stages) - 1; stageIdx >= 0; stageIdx-- {
			segmentState := s.segmentStates[segmentIdx][stageIdx]
			if segmentState != UnitPending {
				continue
			}
			if segmentIdx < s.stages[stageIdx].firstSegment {
				// Don't process stages where all modules's initial blocks are only later
				continue
			}
			if !s.dependenciesCompleted(segmentIdx, stageIdx) {
				continue
			}

			id := &Unit{
				Stage:   stageIdx,
				Segment: segmentIdx,
				Range:   s.Range(segmentIdx),
			}
			s.markSegmentScheduled(*id)
			return id
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

func (s *Stages) dependenciesCompleted(segmentIdx int, stageIdx int) bool {
	if segmentIdx == 0 {
		return true
	}
	if stageIdx == 0 {
		return true
	}
	for i := stageIdx - 1; i >= 0; i-- {
		if s.segmentStates[segmentIdx-1][i] != UnitCompleted {
			return false
		}
	}
	return true
}

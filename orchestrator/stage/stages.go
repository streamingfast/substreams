package stage

import (
	"github.com/streamingfast/substreams/block"
	"github.com/streamingfast/substreams/pipeline/outputmodules"
)

type Stages struct {
	*block.Segmenter

	stages []*Stage
	state  [][]SegmentState // state[SegmentIndex][StageIndex]

	completedSegments int
}

func NewStages(
	outputGraph *outputmodules.Graph,
	interval uint64,
	upToBlock uint64,
) (out *Stages) {
	lowestGraphInitBlock := outputGraph.LowestInitBlock()
	allStages := outputGraph.StagedUsedModules()
	lastIndex := len(allStages) - 1
	seg := block.NewSegmenter(interval, lowestGraphInitBlock, upToBlock)
	out = &Stages{
		Segmenter: seg,
	}
	for idx, stage := range allStages {
		isLastStage := idx == lastIndex
		kind := stageKind(stage)
		if kind == KindMap && !isLastStage {
			continue
		}
		stageState := &Stage{
			kind: kind,
		}
		lowestStageInitBlock := stage[0].InitialBlock
		for _, mod := range stage {
			stageState.modules = append(stageState.modules, &ModuleState{
				name: mod.Name,
			})
			if lowestStageInitBlock > mod.InitialBlock {
				lowestStageInitBlock = mod.InitialBlock
			}
		}

		stageState.firstSegment = seg.IndexForBlock(lowestStageInitBlock)

		out.stages = append(out.stages, stageState)
	}
	return out
}

func (s *Stages) NextJob() *SegmentID {
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
		if len(s.state) <= segmentIdx {
			s.growSegments()
		}
		if segmentIdx >= s.Count() {
			break
		}
		for stageIdx := len(s.stages) - 1; stageIdx >= 0; stageIdx-- {
			segmentState := s.state[segmentIdx][stageIdx]
			if segmentState != SegmentPending {
				continue
			}
			if segmentIdx < s.stages[stageIdx].firstSegment {
				// Don't process stages where all modules's initial blocks are only later
				continue
			}
			if !s.dependenciesCompleted(segmentIdx, stageIdx) {
				continue
			}

			s.state[segmentIdx][stageIdx] = SegmentScheduled
			return &SegmentID{
				Stage:   stageIdx,
				Segment: segmentIdx,
				Range:   s.Range(segmentIdx),
			}
		}
		segmentIdx++
	}
	return nil
}

func (s *Stages) MarkJobCompleted(segment int, stage int) {
	if s.state[segment][stage] != SegmentScheduled {
		panic("cannot mark job completed if it was not scheduled")
	}
	s.state[segment][stage] = SegmentCompleted
}

func (s *Stages) growSegments() {
	by := len(s.state)
	if by == 0 {
		by = 2
	}
	for i := 0; i < by; i++ {
		s.state = append(s.state, make([]SegmentState, len(s.stages)))
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
		if s.state[segmentIdx-1][i] != SegmentCompleted {
			return false
		}
	}
	return true
}

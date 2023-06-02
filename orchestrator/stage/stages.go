package stage

import (
	"github.com/streamingfast/substreams/block"
	"github.com/streamingfast/substreams/pipeline/outputmodules"
	"github.com/streamingfast/substreams/storage"
)

type Stages struct {
	*block.Segmenter

	stages []*Stage
	state  [][]SegmentState // state[SegmentIndex][StageIndex]

	completedSegments int
}

func NewStages(
	outputGraph *outputmodules.Graph,
	storageMap storage.ModuleStorageStateMap,
	interval uint64,
	upToBlock uint64,
) (out *Stages) {
	lowestGraphInitBlock := outputGraph.LowestInitBlock()
	allStages := outputGraph.StagedUsedModules()
	lastIndex := len(allStages) - 1
	out = &Stages{
		Segmenter: block.NewSegmenter(interval, lowestGraphInitBlock, lowestGraphInitBlock, upToBlock),
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
			//store := storageMap[mod.Name]
			stageState.modules = append(stageState.modules, &ModuleState{
				name:      mod.Name,
				Segmenter: block.NewSegmenter(interval, lowestGraphInitBlock, mod.InitialBlock, upToBlock),
			})
			if lowestStageInitBlock > mod.InitialBlock {
				lowestStageInitBlock = mod.InitialBlock
			}
		}

		stageState.Segmenter = block.NewSegmenter(interval, lowestGraphInitBlock, lowestStageInitBlock, upToBlock)

		out.stages = append(out.stages, stageState)
	}
	return out
}

// Algorithm for planning the Next Jobs:
// We need to start from the last stage, first segment.

func (s *Stages) NextJob() *SegmentID {
	// FIXME: eventually, we can start from s.completedSegments, and push `completedSegments`
	// each time contiguous segments are completed for all stages.
	segmentIdx := 0
	for {
		if segmentIdx > s.CountFromBegin() {
			break
		}
		for stageIdx := len(s.stages) - 1; stageIdx >= 0; stageIdx-- {
			segmentState := s.state[segmentIdx][stageIdx]
			if segmentState != SegmentPending {
				continue
			}
			if s.stages[stageIdx].FirstModuleSegment() > segmentIdx {
				// TODO: FirstModuleSegment() takes this functionality out of the Segmenter
				// the Segmenter will only consider things from the graphInitBlock
				// and this stage's lowest module init block will be set as a
				// property of the `Stage` struct, and this condition will use that
				// variable.
				// We'll query the Segmenter for `IndexForBlock(module.InitialBlock)` and use that as the "FirstModuleSegment"

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
		if len(s.state) <= segmentIdx {
			s.growSegments(32)
		}
		segmentIdx++
	}

	return nil
}

func (s *Stages) MarkJobCompleted(segment SegmentID) {
	s.state[segment.Segment][segment.Stage] = SegmentCompleted
}

func (s *Stages) growSegments(by int) {
	for i := 0; i < by; i++ {
		s.state = append(s.state, make([]SegmentState, len(s.stages)))
	}
}

func (s *Stages) dependenciesCompleted(segmentIdx int, stageIdx int) bool {
	if segmentIdx == 0 {
		return true
	}
	for i := stageIdx; i >= 0; i-- {
		if s.state[segmentIdx-1][i] != SegmentCompleted {
			return false
		}
	}
	return true
}

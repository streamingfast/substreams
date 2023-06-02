package stage

import (
	"github.com/streamingfast/substreams/block"
	"github.com/streamingfast/substreams/orchestrator/work"
	"github.com/streamingfast/substreams/pipeline/outputmodules"
	"github.com/streamingfast/substreams/storage"
)

type Stages struct {
	*block.Segmenter

	stages   []*Stage
	segments [][]SegmentState // segments[SegmentIndex][StageIndex]

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

func (s Stages) NextJob() *work.Job {
	nextSegment := s.completedSegments
	if len(s.segments) < nextSegment-1 {

	}
	for stageIndex, stage := range s.stages {
		for segmentIndex, segment := range stage.segments {
			if segment != SegmentPending {
				continue
			}
			// check if all dependencies are resolved
			// mark it Reserved
			// and MarkJobScheduled(segment SegmentID)
			// and MarkJobCompleted(segment SegmentID)
		}
	}
}

func (s *Stages) growSegments(by int) {
	for i := 0; i < by; i++ {
		s.segments = append(s.segments, make([]SegmentState, len(s.stages)))
	}
}

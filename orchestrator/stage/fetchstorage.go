package stage

import (
	"context"
	"fmt"

	"github.com/streamingfast/bstream"
	"github.com/streamingfast/substreams/block"
	"github.com/streamingfast/substreams/storage/execout"

	"github.com/streamingfast/substreams/storage/store"
	"github.com/streamingfast/substreams/storage/store/state"
)

func (s *Stages) FetchStoresState(
	ctx context.Context,
	segmenter *block.Segmenter,
	storeConfigMap store.ConfigMap,
	execoutConfigs *execout.Configs,
) error {
	completes := make(unitMap)
	partials := make(unitMap)

	upToBlock := segmenter.ExclusiveEndBlock()

	var mapperName string
	var mapperFiles execout.FileInfos
	if lastStage := s.stages[len(s.stages)-1]; lastStage.kind == KindMap {
		if len(lastStage.storeModuleStates) != 1 {
			panic("assertion: mapper stage should contain a single module")
		}

		mapperName = lastStage.storeModuleStates[0].name
		conf := execoutConfigs.ConfigMap[mapperName]
		// TODO: OPTIMIZATION: get the actual needed range for execOutputs to optimize lookup

		if upToBlock != 0 {
			files, err := conf.ListSnapshotFiles(ctx, bstream.NewInclusiveRange(0, upToBlock))
			if err != nil {
				return fmt.Errorf("fetching mapper storage state: %w", err)
			}
			mapperFiles = files
		}
	}

	// TODO: OPTIMIZATION: why load stores if there could be ExecOut data present
	// on disk already, which avoid the need to do _any_ processing whatsoever?
	state, err := state.FetchState(ctx, storeConfigMap, upToBlock)
	if err != nil {
		return fmt.Errorf("fetching stores storage state: %w", err)
	}
	for stageIdx, stage := range s.stages {
		moduleCount := len(stage.storeModuleStates)

		if stage.kind == KindMap {
			if mapperFiles == nil {
				continue
			}
			if stageIdx != len(s.stages)-1 {
				panic("assertion: mapper stage is not the last stage")
			}
			for _, outputFile := range mapperFiles {
				segmentIdx := s.mapSegmenter.IndexForEndBlock(outputFile.BlockRange.ExclusiveEndBlock)
				rng := s.mapSegmenter.Range(segmentIdx)
				if rng == nil || rng.ExclusiveEndBlock != outputFile.BlockRange.ExclusiveEndBlock {
					continue
				}
				unit := Unit{Stage: stageIdx, Segment: segmentIdx}
				if allDone := markFound(completes, unit, mapperName, moduleCount); allDone {
					s.markSegmentCompleted(unit)
				}
			}

			continue
		}

		for _, mod := range stage.storeModuleStates {
			files := state.Snapshots[mod.name]
			modSegmenter := mod.segmenter

			// TODO: what happens to the Unit's state if we don't have
			// complete sores for all modules within?
			// We'll need to do the same alignment of Complete stores
			for _, fullKV := range files.FullKVFiles {
				segmentIdx := modSegmenter.IndexForEndBlock(fullKV.Range.ExclusiveEndBlock)
				rng := segmenter.Range(segmentIdx)
				if rng == nil || rng.ExclusiveEndBlock != fullKV.Range.ExclusiveEndBlock {
					continue
				}
				unit := Unit{Stage: stageIdx, Segment: segmentIdx}
				if allDone := markFound(completes, unit, mod.name, moduleCount); allDone {
					s.markSegmentCompleted(unit)
				}
			}

			for _, partial := range files.Partials {
				segmentIdx := modSegmenter.IndexForStartBlock(partial.Range.StartBlock)
				rng := segmenter.Range(segmentIdx)
				if rng == nil {
					continue
				}
				if !rng.Equals(partial.Range) {
					continue
				}
				unit := Unit{Stage: stageIdx, Segment: segmentIdx}

				if s.getState(unit) == UnitCompleted {
					// FullKVs take precedence over partial stores' presence.
					continue
				}

				if allDone := markFound(partials, unit, mod.name, moduleCount); allDone {
					s.MarkSegmentPartialPresent(unit)
				}
			}
		}

		s.MoveSegmentCompletedForward(stageIdx)
	}

	// loop all stages
	// loop all segments, check whether they are complete

	return nil
}

type unitMap map[Unit]map[string]struct{}

func markFound(unitMap unitMap, unit Unit, name string, moduleCount int) bool {
	mods := unitMap[unit]
	if mods == nil {
		mods = make(map[string]struct{})
		unitMap[unit] = mods
	}
	mods[name] = struct{}{}
	return len(mods) == moduleCount
}

package stage

import (
	"context"
	"fmt"

	"github.com/streamingfast/substreams/block"
	"github.com/streamingfast/substreams/storage/store"
	"github.com/streamingfast/substreams/storage/store/state"
)

func (s *Stages) FetchStoresState(
	ctx context.Context,
	segmenter *block.Segmenter,
	storeConfigMap store.ConfigMap,
	traceID string,
) error {
	completes := make(unitMap)
	partials := make(unitMap)

	upToBlock := segmenter.ExclusiveEndBlock()

	// OPTIMIZATION: why load stores if there could be ExecOut data present
	// on disk already, which avoid the need to do _any_ processing whatsoever?
	state, err := state.FetchState(ctx, storeConfigMap, upToBlock)
	if err != nil {
		return fmt.Errorf("fetching stores storage state: %w", err)
	}
	for stageIdx, stage := range s.stages {
		moduleCount := len(stage.moduleStates)

		if stage.kind == KindMap {
			continue
		}

		for _, mod := range stage.moduleStates {
			files := state.Snapshots[mod.name]
			modSegmenter := mod.segmenter

			// TODO: what happens to the Unit's state if we don't have
			// complete sores for all modules within?
			// We'll need to do the same alignment of Complete stores
			for _, fullKV := range files.FullKVFiles {
				segmentIdx := modSegmenter.IndexForEndBlock(fullKV.Range.ExclusiveEndBlock)
				rng := segmenter.Range(segmentIdx)
				if rng.ExclusiveEndBlock != fullKV.Range.ExclusiveEndBlock {
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
				if traceID != partial.TraceID {
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

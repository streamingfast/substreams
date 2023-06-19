package storage

import (
	"context"
	"fmt"

	"github.com/streamingfast/substreams/block"
	"github.com/streamingfast/substreams/orchestrator/stage"
	"github.com/streamingfast/substreams/storage/store"
	storeState "github.com/streamingfast/substreams/storage/store/state"
)

type state2 struct {
	stage.Unit
	state stage.UnitState
}

func FetchStoresState(
	ctx context.Context,
	// TODO: Make this a method of `Stages` so we have access to all those internal methods.
	stages *stage.Stages,
	segmenter *block.Segmenter,
	storeConfigMap store.ConfigMap,
) ([]state2, error) {
	// This function needs to be aware of the stages, and the modules
	// included in each stage.
	// When we find files for a given module, we'll add it to a map
	// associated with the stage, and when the length of the map is
	// equal to the length of the modules array, we know we have a
	// complete Stage, and we can emit the message.

	// We need a segmenter for each module's, because the lower boundary
	// of the Store is specific to the module's initial block.

	// How to inject lots of messages initially? We spin this process
	// in a loop.Cmd and have it do some `scheduler.Send()` like crazy?
	// with a final message flipping a switch to kickstart job scheduling
	// and all? We don't want to do some job scheduling until all of the
	// File walkers here are done.

	completes := make(map[stage.Unit]map[string]struct{})
	partials := make(map[stage.Unit]map[string]struct{})

	upToBlock := segmenter.Range(segmenter.LastIndex()).ExclusiveEndBlock

	state, err := storeState.FetchState(ctx, storeConfigMap, upToBlock)
	if err != nil {
		return nil, fmt.Errorf("fetching stores storage state: %w", err)
	}
	for stageIdx, stage := range stages.stages {
		for _, mod := range stage.Modules {
			files := state.Snapshots[mod.Name]
			modSegmenter := mod.segmenter

			for _, complete := range files.Completes {
				segmentIdx := modSegmenter.IndexForEndBlock(complete.Range.ExclusiveEndBlock)
				rng := segmenter.Range(segmentIdx)
				if rng == nil {
					continue
				}
				if rng.ExclusiveEndBlock != complete.Range.ExclusiveEndBlock {
					continue
				}
				unit := stage.Unit{Stage: stageIdx, Segment: segmentIdx}
				if allDone := markFound(completes, unit, mod.Name, modules); allDone {
					stages.markSegmentCompleted(unit)
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
				unit := stage.Unit{Stage: stageIdx, Segment: segmentIdx}

				if allDone := markFound(partials, unit, mod.Name, modules); allDone {
					stages.MarkSegmentPartialPresent(unit)
				}
			}
		}
	}

	return nil, fmt.Errorf("not implemented")
}

func markFound(
	unitMap map[stage.Unit]map[string]struct{},
	unit stage.Unit,
	name string,
	allModules []string,
) bool {
	mods := unitMap[unit]
	if mods == nil {
		mods = make(map[string]struct{})
		unitMap[unit] = mods
	}
	mods[name] = struct{}{}
	return len(mods) == len(allModules)
}

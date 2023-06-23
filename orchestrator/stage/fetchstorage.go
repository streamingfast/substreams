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
) error {
	completes := make(unitMap)
	partials := make(unitMap)

	upToBlock := segmenter.ExclusiveEndBlock()

	// TODO: this method has two goals:
	// load the matrix with complete stores and partials present
	// load the initial blocks, but why not just use the stores
	// at the VERY END? instead of merging all the time?
	// let the stages produce the complete snapshots
	// and don't try to merge them..
	// only when we're near the LinearHandoff should we
	// start loading those stores in preparation for the
	// linear handoff.

	// TODO: make sure the Tier2 jobs write the FULL stores
	// but NO, the tier2 produce partials jobs.. that need to be
	// merged, but right now we have TWO needs for merging:
	// 1) preparing the storeMap for the end
	// 2) taking the previous full and fuse it with a partial
	// in order to produce a new complete store, so that the
	// next job can start.
	//  In this case, we mustn't start a job until the full store
	//  has been properly written to storage.. right now we
	//  ship that in a go routine..
	//  with that `writerErrGroup`.. what's the use of that?

	// FIXME: why load stores if there could be ExecOut data present
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
			// compelte sores for all modules within?
			// We'll need to do the same alignment of Complete stores
			complete := files.LastCompleteSnapshotBefore(upToBlock)
			if complete != nil {
				// HERE WE should actually just load the CLOSEST to the start
				// point
				segmentIdx := modSegmenter.IndexForEndBlock(complete.Range.ExclusiveEndBlock)
				rng := segmenter.Range(segmentIdx)
				if rng.ExclusiveEndBlock != complete.Range.ExclusiveEndBlock {
					continue
				}
				unit := Unit{Stage: stageIdx, Segment: segmentIdx}
				if allDone := markFound(completes, unit, mod.name, moduleCount); allDone {
					// TODO: we should push the `segmentComplete` and LOAD all the stores
					// aligned at this block, but only for the _highest_ of the
					// completed bundles.

					// TODO: do we need another state, for when a CompleteStore is
					// present? or FullKV is present, in which case we can load it
					// altogether instead of merging it. a Full followed by a PartialPresent
					// could do with a `Load()` of the previous `Full`, and then a merge
					// of the partial.
					// But if we have FullKV here and there, we don't need to schedule
					// work to produce them, they are already there.

					// TODO: that might mean that the `moduleState` needs to keep
					// track itself of the state of the advancement of its `store`.
					// Also it should produce a Message when a FullKV is being written
					// and when it written, in which case we can lauch the next job that
					// would consume it. And if we receive notice that a FullKV already
					// exists, we don't schedule work to produce it, and we potentially
					// load it to merge the following stuff.

					// TODO: review the meaning of `UnitCompleted`, perhaps rename to
					// `UnitFullPresent`. And that state should not mean that
					// all stores for a Unit have been merged, or whatever the state
					// of the merging process. That should be kept inside the `state`
					// which are linear view, and always going forward.. to produce
					// whatever is necessary to generate the ExecOu or the final
					//`StoreMap` as a setup-phase for the LinearPipeline
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

				if allDone := markFound(partials, unit, mod.name, moduleCount); allDone {
					// TODO: if we discover both a Partial and a Complete, we need to make
					// sure that the Complete is the thing that wins in the state, because
					// we don't want to lose time merging a partial
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

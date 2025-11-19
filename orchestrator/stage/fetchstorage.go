package stage

import (
	"context"
	"fmt"

	"github.com/streamingfast/substreams/block"
	"github.com/streamingfast/substreams/reqctx"
	"github.com/streamingfast/substreams/storage/execout"

	"github.com/streamingfast/substreams/storage/store/state"
)

var firstPassStoresWalkMaxSegments = 50 // arbitrary "small" number of segments to scan for the first pass. Scan time is slow when listing thousands of files in object storage

// FetchCachesState will look at the cache for:
// 1. the output mapper (if we are in production mode and producing ExecOuts)
// 2. each store (either:
//   - if we need to prepare the stores after reading the execouts (for the LIVE segment) or
//   - if we don't have the ExecOuts on the requested range or
//   - if we are in development mode and need to prepare the stores at the beginning of the range
//
// )
// Then, the internal "s.segmentStates" will be updated.
// It tries to fetch the minimal number of files, because with object-storage (gcs, s3, ...), listing files can be slow and costly
func (s *Stages) FetchCachesState(
	ctx context.Context,
) error {

	mapperName, mapperFiles, err := s.fetchOutputMapperState(ctx)
	if err != nil {
		return err
	}

	hasStores := s.storeSegmenter != nil
	if mapperFiles != nil {
		stageIdx := len(s.stages) - 1
		for _, outputFile := range mapperFiles {
			segmentIdx := s.mapSegmenter.IndexForEndBlock(outputFile.BlockRange.ExclusiveEndBlock)
			rng := s.mapSegmenter.Range(segmentIdx)
			if rng == nil || rng.ExclusiveEndBlock != outputFile.BlockRange.ExclusiveEndBlock {
				continue
			}
			unit := Unit{Stage: stageIdx, Segment: segmentIdx}
			s.markSegmentCompleted(unit)
		}

		// attempt early exit: no store
		if !hasStores {
			return nil
		}

		// attempt early exit: if we have all the execoutputs, and don't launch a pipeline, we don't need to fetch the stores
		if !s.hasLinearPipeline {
			mapperUnits := s.getUnits(s.mapSegmenter.FirstIndex(), s.mapSegmenter.LastIndex(), len(s.stages)-1)
			allMappersDone := true
			for _, unit := range mapperUnits {
				if s.getState(unit) != UnitCompleted {
					allMappersDone = false
					break
				}
			}
			if allMappersDone {
				for _, stage := range s.stages[0 : len(s.stages)-1] {
					for _, unit := range s.getUnits(stage.segmenter.FirstIndex(), stage.segmenter.LastIndex(), stage.idx) {
						s.transition(unit, UnitNoOp, UnitPending)
					}
				}
				return nil
			}
		}
	}
	if !hasStores {
		return nil
	}

	completes := make(unitMap)
	partials := make(unitMap)

	// We make a first pass to fetch the state of the stores, trying to read no more than a handful of files per store, for performance considerations
	// The idea is that object storage only allows scanning forward when listing files, and it is quite slow on some backends.
	// Since we know that the presence of a fullKV file for block `x` implies that all the fullKV files for blocks <x are present, we can backfill the states without actually scanning all the files
	firstPassStoresWalk := s.firstPassStoresWalk()
	cacheState, err := state.FetchState(ctx, s.storeConfigs, firstPassStoresWalk.StartBlock, firstPassStoresWalk.ExclusiveEndBlock)
	if err != nil {
		return fmt.Errorf("fetching stores storage state: %w", err)
	}

	modulesFetchedOverAllRange := make(map[string]bool)
	var someStoresAreMissingFullKVs bool
stageLoop:
	for _, stage := range s.stages {
		for _, mod := range stage.storeModuleStates {
			modulesFetchedOverAllRange[mod.name] = false
			if firstPassStoresWalk.StartBlock > mod.segmenter.InitialBlock() && (cacheState.Snapshots[mod.name] == nil || len(cacheState.Snapshots[mod.name].FullKVFiles) == 0) {
				someStoresAreMissingFullKVs = true
				break stageLoop
			}
		}
	}

	// Second pass: we if we didn't see any expected fullKV for a store in our first pass, we scan again, with the full range
	// Note that we don't reuse the snapshots already fetched: listing a few files twice same few files twice is negligible.
	if someStoresAreMissingFullKVs {
		cacheState, err = state.FetchState(ctx, s.storeConfigs, s.storeSegmenter.InitialBlock(), s.storeSegmenter.ExclusiveEndBlock())
		if err != nil {
			return fmt.Errorf("fetching stores storage state: %w", err)
		}
	}

	segmenter := s.globalSegmenter

	for stageIdx, stage := range s.stages {
		firstIndexes := make(map[string]int)
		for _, mod := range stage.storeModuleStates {
			firstIndexes[mod.name] = mod.segmenter.FirstIndex()
		}
		moduleCount := func(unit Unit) (out int) {
			for _, mod := range stage.storeModuleStates {
				if unit.Segment >= firstIndexes[mod.name] {
					out++
				}
			}
			return
		}

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
				if allDone := markFound(completes, unit, mapperName, moduleCount(unit)); allDone {
					s.markSegmentCompleted(unit)
				}
			}

			continue
		}

		for _, mod := range stage.storeModuleStates {
			files := cacheState.Snapshots[mod.name]
			modSegmenter := mod.segmenter

			for _, fullKV := range files.FullKVFiles {
				segmentIdx := modSegmenter.IndexForEndBlock(fullKV.Range.ExclusiveEndBlock)
				rng := segmenter.Range(segmentIdx)
				if rng == nil || rng.ExclusiveEndBlock != fullKV.Range.ExclusiveEndBlock {
					continue
				}
				if !modulesFetchedOverAllRange[mod.name] {
					// here we backfill the previous segments for modules that were not fetched completely
					for backfillIdx := modSegmenter.FirstIndex(); backfillIdx < segmentIdx; backfillIdx++ {
						unit := Unit{Stage: stageIdx, Segment: backfillIdx}
						if allDone := markFound(completes, unit, mod.name, moduleCount(unit)); allDone {
							s.markSegmentCompleted(unit)
						}
					}
					modulesFetchedOverAllRange[mod.name] = true
				}
				unit := Unit{Stage: stageIdx, Segment: segmentIdx}
				if allDone := markFound(completes, unit, mod.name, moduleCount(unit)); allDone {
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

				if allDone := markFound(partials, unit, mod.name, moduleCount(unit)); allDone {
					s.MarkSegmentPartialPresent(unit)
				}
			}
		}

		s.MoveSegmentCompletedForward(stageIdx)
	}
	// right after we fetch the state on disk, we can determine the shadowable segments
	s.setShadowableSegment(segmenter.IndexForStartBlock(reqctx.Details(ctx).ResolvedStartBlockNum))
	return nil
}

// firstPassStoresWalk will give you an optimistic range for scanning the presence of files in a store
func (s *Stages) firstPassStoresWalk() *block.Range {
	lastIndex := s.storeSegmenter.LastIndex()
	firstIndex := s.storeSegmenter.FirstIndex()

	idx := firstIndex
	if lastIndex > firstPassStoresWalkMaxSegments {
		idx = lastIndex - firstPassStoresWalkMaxSegments
		if idx < firstIndex {
			idx = firstIndex
		}
	}
	return block.NewRange(s.storeSegmenter.Range(idx).StartBlock, s.storeSegmenter.ExclusiveEndBlock())
}

func (s *Stages) fetchOutputMapperState(ctx context.Context) (mapperName string, mapperFiles execout.FileInfos, err error) {
	if s.mapSegmenter == nil {
		return
	}

	mapperName = s.stages[len(s.stages)-1].storeModuleStates[0].name
	conf := s.execoutConfigs.ConfigMap[mapperName]

	execOutFirst := s.mapSegmenter.InitialBlock()
	execOutLast := s.mapSegmenter.ExclusiveEndBlock()
	if execOutLast < execOutFirst+1 {
		return
	}
	execOutLast -= 1

	mapperFiles, err = conf.ListSnapshotFiles(ctx, execOutFirst, execOutLast)
	return
}

func (s *Stages) getUnits(firstIndex, lastIndex, stage int) []Unit {
	if firstIndex >= lastIndex {
		return nil
	}
	out := make([]Unit, lastIndex-firstIndex+1)
	i := 0
	for segIdx := firstIndex; segIdx <= lastIndex; segIdx++ {
		out[i] = Unit{Segment: segIdx, Stage: stage}
		i++
	}
	return out
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

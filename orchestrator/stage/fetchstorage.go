package stage

import (
	"context"

	"github.com/streamingfast/bstream"
	"github.com/streamingfast/substreams/block"
	"github.com/streamingfast/substreams/reqctx"
	"github.com/streamingfast/substreams/storage/execout"

	"github.com/streamingfast/substreams/storage/store"
	"github.com/streamingfast/substreams/storage/store/state"
)

/*

1) mappers-only (do not call fetchStoresStates) -> get mappers on the outputMapper range only

2) stores, no linear segment (obviously bound)
   a. check if we have the full mapper outputs cached on the segment
   (if we do, break)
   b. if not, check if we have the full stores at the beginning of each of our mapper output segments
   (if we do, check the mappers on that range too and break)
   c. if not, walk through all the stores (do we have to walk through the mappers anyway?)

3) stores, with linear segment
   a. check if we have the full mapper outputs cached on the segment AND the full store at the beginning of the linear segment
    (if we do, break)
   b. if not, check if we have the full stores at the beginning of each of our mapper output segments that needs to be reprocessed
    (if we do, check the mappers on that range too and break)
   c. if not, walk through all the stores (do we have to walk through the mappers anyway?)

*/

func (s *Stages) applyState(
	mapperFiles execout.FileInfos,
	state map[string]*state.StoreSnapshots,
	resolvedStartBlock uint64,
) (complete bool) {
	var mapperName string

	segmenter := s.storeSegmenter
	if segmenter == nil {
		segmenter = s.mapSegmenter
	}

	completes := make(unitMap)
	partials := make(unitMap)

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
			files := state[mod.name]
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

	// here: must determine if we have enough or if we need to go back up
	return true

}

func (s *Stages) FetchCacheState(
	ctx context.Context,
	storeConfigMap store.ConfigMap,
	execoutConfigs *execout.Configs,
	withLinearSegment bool,
	resolvedStartBlock uint64,
) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	outputMapper := s.getOutputMapperCache(ctx, s.mapSegmenter, execoutConfigs)

	var nearStartBlock, nearEndBlock uint64
	if s.mapSegmenter != nil {
		nearStartBlock = s.mapSegmenter.InitialBlock()
		nearEndBlock = s.mapSegmenter.ExclusiveEndBlock()
	} else {
		nearStartBlock = resolvedStartBlock / s.storeSegmenter.SegmentSize() * s.storeSegmenter.SegmentSize()
		nearEndBlock = s.storeSegmenter.ExclusiveEndBlock()
	}

	stateFilesInNearRange := s.getStoreStates(ctx, storeConfigMap, nearStartBlock, nearEndBlock)

	var outputMapperRes execout.FileInfos

	select {
	case <-ctx.Done():
		return ctx.Err()
	case res := <-outputMapper:
		if res.Err != nil {
			return res.Err
		}
		outputMapperRes = res.Ok
	}
	if outputMapperRes != nil {
		start, end := consecutiveFilesRange(outputMapperRes)
		if start <= s.mapSegmenter.InitialBlock() && end >= s.mapSegmenter.ExclusiveEndBlock() {
			s.markAllOutputMapperSegmentsCompleted()
			if !withLinearSegment {
				return nil
			}
		}
	}

	var stateFilesInNearRangeRes map[string]*state.StoreSnapshots
	select {
	case <-ctx.Done():
		return ctx.Err()
	case res := <-stateFilesInNearRange:
		if res.Err != nil {
			return res.Err
		}
		stateFilesInNearRangeRes = res.Ok
	}
	if s.applyState(outputMapperRes, stateFilesInNearRangeRes, resolvedStartBlock) {
		s.setShadowableSegment(s.globalSegmenter.IndexForStartBlock(resolvedStartBlock))
		return nil
	}

	stateFilesBelowNearRange := s.getStoreStates(ctx, storeConfigMap, 0, nearStartBlock)

	var stateFilesBelowNearRangeRes map[string]*state.StoreSnapshots
	select {
	case <-ctx.Done():
		return ctx.Err()
	case res := <-stateFilesBelowNearRange:
		if res.Err != nil {
			return res.Err
		}
		stateFilesBelowNearRangeRes = res.Ok
	}
	allStateFiles := appendStateFiles(stateFilesBelowNearRangeRes, stateFilesInNearRangeRes)
	_ = s.applyState(outputMapperRes, allStateFiles, resolvedStartBlock)
	s.setShadowableSegment(s.globalSegmenter.IndexForStartBlock(resolvedStartBlock))
	return nil

}

func (s *Stages) getStoreStates(
	ctx context.Context,
	storeConfigMap store.ConfigMap,
	from uint64,
	to uint64,
) <-chan ChanResult[map[string]*state.StoreSnapshots] {

	var mapperName string
	var mapperFiles execout.FileInfos

	segmenter := s.storeSegmenter
	if segmenter == nil {
		segmenter = s.mapSegmenter
	}

	completes := make(unitMap)
	partials := make(unitMap)

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

type ChanResult[T any] struct {
	Ok  T
	Err error
}

func (s *Stages) getOutputMapperCache(
	ctx context.Context,
	segmenter *block.Segmenter,
	execoutConfigs *execout.Configs,
) <-chan ChanResult[execout.FileInfos] {
	out := make(chan ChanResult[execout.FileInfos], 1)

	if segmenter == nil {
		out <- ChanResult[execout.FileInfos]{Ok: nil, Err: nil}
		return out
	}

	fromBlock := segmenter.InitialBlock()
	upToBlock := segmenter.ExclusiveEndBlock()
	if upToBlock == fromBlock {
		out <- ChanResult[execout.FileInfos]{Ok: nil, Err: nil}
		return out
	}

	lastStage := s.stages[len(s.stages)-1]
	conf := execoutConfigs.ConfigMap[lastStage.storeModuleStates[0].name]

	go func() {
		files, err := conf.ListSnapshotFiles(ctx, bstream.NewInclusiveRange(fromBlock, upToBlock))
		if err != nil {
			out <- ChanResult[execout.FileInfos]{Ok: files, Err: nil}
		} else {
			out <- ChanResult[execout.FileInfos]{Ok: files, Err: nil}
		}
	}()

	return out
}

func consecutiveFilesRange(files execout.FileInfos) (start uint64, exclusiveEnd uint64) {
	if len(files) == 0 {
		return 0, 0
	}

	start = files[0].BlockRange.StartBlock
	last := files[0].BlockRange.ExclusiveEndBlock
	for _, file := range files[1:] {
		if last != file.BlockRange.StartBlock {
			return
		}
		last = file.BlockRange.ExclusiveEndBlock
	}
	return
}

// caution: this modifies left and returns it
func appendStateFiles(
	left map[string]*state.StoreSnapshots,
	right map[string]*state.StoreSnapshots,
) map[string]*state.StoreSnapshots {
	for name, files := range right {
		if _, found := left[name]; !found {
			left[name] = files
		} else {
			left[name].FullKVFiles = append(left[name].FullKVFiles, files.FullKVFiles...)
			left[name].Partials = append(left[name].Partials, files.Partials...)
			// left[name].Sort() already sorted in input
		}
	}
	return left
}

func (s *Stages) markAllOutputMapperSegmentsCompleted() {
	stageIdx := len(s.stages) - 1
	for segmentIdx := s.mapSegmenter.FirstIndex(); segmentIdx <= s.mapSegmenter.LastIndex(); segmentIdx++ {
		unit := Unit{Stage: stageIdx, Segment: segmentIdx}
		s.markSegmentCompleted(unit)
	}
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

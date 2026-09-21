package stage

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/streamingfast/substreams/reqctx"
	"github.com/streamingfast/substreams/storage/execout"
	"github.com/streamingfast/substreams/storage/store/state"
)

// storesWalkWindowSegments is the number of segments covered by the first listing window
// when looking for store snapshots. Each following window is storesWalkWindowGrowth times
// larger, so a store whose nearest snapshot is far behind costs O(log n) bounded listings
// instead of one listing of its whole history.
var storesWalkWindowSegments = 50

const storesWalkWindowGrowth = 4

// FetchCachesState will look at the cache for:
// 1. the output mapper (if we are in production mode and producing ExecOuts)
// 2. each store (either:
//   - if we need to prepare the stores after reading the execouts (for the LIVE segment) or
//   - if we don't have the ExecOuts on the requested range or
//   - if we are in development mode and need to prepare the stores at the beginning of the range
//
// )
// Then, the internal "s.segmentStates" will be updated.
//
// Store snapshots may have been pruned: a fullKV existing at block `x` does NOT imply that
// the fullKVs for blocks <x still exist. A unit is only marked Completed when its file was
// actually seen. The resume segment is the highest segment at or below the first segment
// needing work for which every store module has a fullKV; every segment below it is
// marked NoOp since nothing will ever read those files.
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

	target := s.storesTargetSegment()
	cacheState, resume, err := s.scanStoreSnapshots(ctx, target)
	if err != nil {
		return fmt.Errorf("fetching stores storage state: %w", err)
	}
	reqctx.Logger(ctx).Info("stores resume point",
		zap.Int("target_segment", target),
		zap.Int("resume_segment", resume),
		zap.String("snapshots", cacheState.Summary()),
	)

	completes := make(unitMap)
	partials := make(unitMap)
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

		for segmentIdx := stage.segmenter.FirstIndex(); segmentIdx < resume; segmentIdx++ {
			s.markSegmentNoOp(Unit{Stage: stageIdx, Segment: segmentIdx})
		}

		for _, mod := range stage.storeModuleStates {
			files := cacheState.Snapshots[mod.name]
			if files == nil {
				continue
			}
			modSegmenter := mod.segmenter

			for _, fullKV := range files.FullKVFiles {
				segmentIdx := modSegmenter.IndexForEndBlock(fullKV.Range.ExclusiveEndBlock)
				if segmentIdx < resume {
					continue
				}
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
				if segmentIdx <= resume {
					continue
				}
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

// storesTargetSegment is the last store segment that must be complete before any job can
// run: the segment right before the first mapper segment still needing to be produced, or
// the last store segment when no mapper output is missing (the linear pipeline then needs
// the stores at the handoff block).
func (s *Stages) storesTargetSegment() int {
	target := s.storeSegmenter.LastIndex()
	if s.mapSegmenter == nil {
		return target
	}
	mapStage := len(s.stages) - 1
	for segmentIdx := s.mapSegmenter.FirstIndex(); segmentIdx <= s.mapSegmenter.LastIndex(); segmentIdx++ {
		if s.getState(Unit{Stage: mapStage, Segment: segmentIdx}) == UnitPending {
			return min(target, segmentIdx-1)
		}
	}
	return target
}

// scanStoreSnapshots lists store snapshot files in windows walking backwards from the
// target segment, until a resume segment is found or the lowest module initial block is
// reached. The first window also covers everything above the target, so that snapshots
// left by a previous run are reused. Returns every file seen and the resume segment (-1
// when no common fullKV exists at or below the target).
func (s *Stages) scanStoreSnapshots(ctx context.Context, target int) (*state.StoreSnapshotsMap, int, error) {
	lowest := s.storeSegmenter.FirstIndex()
	all := state.NewStoreSnapshotsMap()

	windowLow := max(lowest, target-storesWalkWindowSegments+1)
	windowSize := storesWalkWindowSegments
	inclusiveTo := s.storeSegmenter.ExclusiveEndBlock()

	for {
		from := s.storeSegmenter.Range(windowLow).StartBlock
		fetched, err := state.FetchState(ctx, s.storeConfigs, from, inclusiveTo)
		if err != nil {
			return nil, 0, err
		}
		all.Merge(fetched)

		if resume, found := s.resumeSegment(all, target, windowLow); found {
			return all, resume, nil
		}
		if windowLow <= lowest {
			return all, -1, nil
		}

		inclusiveTo = from - 1
		windowSize *= storesWalkWindowGrowth
		windowLow = max(lowest, windowLow-windowSize)
	}
}

// resumeSegment returns the highest segment in [floor, target] for which every store
// module already started has a fullKV file in `snapshots`.
func (s *Stages) resumeSegment(snapshots *state.StoreSnapshotsMap, target, floor int) (int, bool) {
	seen := make(map[string]map[uint64]struct{}, len(snapshots.Snapshots))
	for name, files := range snapshots.Snapshots {
		ends := make(map[uint64]struct{}, len(files.FullKVFiles))
		for _, file := range files.FullKVFiles {
			ends[file.Range.ExclusiveEndBlock] = struct{}{}
		}
		seen[name] = ends
	}

	for segmentIdx := target; segmentIdx >= floor; segmentIdx-- {
		complete := true
		for _, stage := range s.stages {
			if stage.kind != KindStore {
				continue
			}
			for _, mod := range stage.storeModuleStates {
				if segmentIdx < mod.segmenter.FirstIndex() {
					continue
				}
				rng := mod.segmenter.Range(segmentIdx)
				if rng == nil {
					continue
				}
				if _, ok := seen[mod.name][rng.ExclusiveEndBlock]; !ok {
					complete = false
					break
				}
			}
			if !complete {
				break
			}
		}
		if complete {
			return segmentIdx, true
		}
	}
	return 0, false
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

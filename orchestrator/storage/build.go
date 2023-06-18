package storage

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/streamingfast/substreams/block"
	"github.com/streamingfast/substreams/orchestrator/stage"
	"github.com/streamingfast/substreams/reqctx"
	"github.com/streamingfast/substreams/storage/execout"
	execoutState "github.com/streamingfast/substreams/storage/execout/state"
	"github.com/streamingfast/substreams/storage/store"
	storeState "github.com/streamingfast/substreams/storage/store/state"
)

type state2 struct {
	stage.Unit
	state stage.UnitState
}

// TODO: another method on `Stages`, upon receiving the end of FetchStoresState()
// piping messages in the `loop`... when all of the
// completes units and partials present units are updated in the
// states matrix, we can look for the earliest store to load.
func (s *Stages) LoadInitialStores() error {

}

func FetchStoresState(
	ctx context.Context,
	// TODO: Make this a method of `Stages` so we have access to all those internal methods.
	stages *stage.Stages,
	storeConfigMap store.ConfigMap,
	segmenter *block.Segmenter,
	storeLinearHandoff uint64,
) ([]state2, error) {
	// we walk all the Complete stores we can find,
	// so we can mark those stage.Unit as StageCompleted
	//
	// we walk all the Partial stores we can find,
	//  bundle them in the proper Stage, and call AddPartial()
	//  on that stage's Unit, and schedule them for merging
	//  also set the stage's Unit as StageMerging

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

	state, err := storeState.FetchState(ctx, storeConfigMap, storeLinearHandoff)
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
					stages.MarkSegmentCompleted(unit)
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

func BuildModuleStorageStateMap(ctx context.Context, storeConfigMap store.ConfigMap, cacheSaveInterval uint64, mapConfigs *execout.Configs, requestStartBlock, linearHandoffBlock, storeLinearHandoffBlock uint64) (ModuleStorageStateMap, error) {
	out := make(ModuleStorageStateMap)
	if err := buildStoresStorageState(ctx, storeConfigMap, cacheSaveInterval, storeLinearHandoffBlock, out); err != nil {
		return nil, err
	}
	// dev mode does not manage mappers states (output caches)
	if details := reqctx.Details(ctx); details.ProductionMode {
		if err := buildMappersStorageState(ctx, mapConfigs, cacheSaveInterval, requestStartBlock, linearHandoffBlock, details.OutputModule, out); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func buildStoresStorageState(ctx context.Context, storeConfigMap store.ConfigMap, storeSnapshotsSaveInterval, storeLinearHandoff uint64, out ModuleStorageStateMap) error {
	logger := reqctx.Logger(ctx)

	state, err := storeState.FetchState(ctx, storeConfigMap, storeLinearHandoff)
	if err != nil {
		return fmt.Errorf("fetching stores states: %w", err)
	}

	for _, config := range storeConfigMap {
		name := config.Name()
		snapshot, ok := state.Snapshots[name]
		if !ok {
			return fmt.Errorf("fatal: storage state not reported for module name %q", name)
		}

		moduleStorageState, err := storeState.NewStoreStorageState(name, storeSnapshotsSaveInterval, config.ModuleInitialBlock(), storeLinearHandoff, snapshot)
		if err != nil {
			return fmt.Errorf("new file units %q: %w", name, err)
		}

		out[name] = moduleStorageState

		logger.Info("work plan for store module", zap.Object("work", moduleStorageState))
	}
	return nil
}

func buildMappersStorageState(ctx context.Context, execoutConfigs *execout.Configs, execOutputSaveInterval, requestStartBlock, linearHandoffBlock uint64, outputModule string, out ModuleStorageStateMap) error {
	stateMap, err := execoutState.FetchMappersState(ctx, execoutConfigs, outputModule)
	if err != nil {
		return fmt.Errorf("fetching execout states: %w", err)
	}

	// note: there is only a single state in this map
	for modName, ranges := range stateMap.Snapshots {
		if out[modName] != nil {
			return fmt.Errorf("attempting to overwrite storage state for module %q", modName)
		}
		config := execoutConfigs.ConfigMap[modName]
		storageState, err := execoutState.NewExecOutputStorageState(config, execOutputSaveInterval, requestStartBlock, linearHandoffBlock, ranges)
		if err != nil {
			return fmt.Errorf("new map storageState: %w", err)
		}
		out[modName] = storageState
	}
	return nil
}

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

type state struct {
	stage.Unit
	state stage.UnitState
}

func FetchStoresState(ctx context.Context, storeConfigMap store.ConfigMap, segmenter *block.Segmenter) ([]state, error) {
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

	// How to inject lots of messages initially? We spin this process
	// in a loop.Cmd and have it do some `scheduler.Send()` like crazy?
	// with a final message flipping a switch to kickstart job scheduling
	// and all? We don't want to do some job scheduling until all of the
	// File walkers here are done.
	return nil, fmt.Errorf("not implemented")
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

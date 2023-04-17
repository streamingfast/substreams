package storage

import (
	"context"
	"fmt"

	"github.com/streamingfast/substreams/reqctx"
	"github.com/streamingfast/substreams/storage/execout"
	"github.com/streamingfast/substreams/storage/execout/state"
	execoutState "github.com/streamingfast/substreams/storage/execout/state"
	"github.com/streamingfast/substreams/storage/store"
	storeState "github.com/streamingfast/substreams/storage/store/state"
	"go.uber.org/zap"
)

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
		storageState, err := state.NewExecOutputStorageState(config, execOutputSaveInterval, requestStartBlock, linearHandoffBlock, ranges)
		if err != nil {
			return fmt.Errorf("new map storageState: %w", err)
		}
		out[modName] = storageState
	}
	return nil
}

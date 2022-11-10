package storage

import (
	"context"
	"fmt"

	"github.com/streamingfast/substreams/storage/execout/state"
	"github.com/streamingfast/substreams/storage/store"
	store2 "github.com/streamingfast/substreams/storage/store/state"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/reqctx"
	"go.uber.org/zap"
)

func BuildModuleStorageStateMap(ctx context.Context, storeConfigMap store.ConfigMap, storeSnapshotsSaveInterval uint64, mapModules []*pbsubstreams.Module, execOutputSaveInterval, upToBlock uint64) (ModuleStorageStateMap, error) {
	out := make(ModuleStorageStateMap)
	if err := buildStoresStorageState(ctx, storeConfigMap, storeSnapshotsSaveInterval, upToBlock, out); err != nil {
		return nil, err
	}
	if err := buildMappersStorageState(ctx, mapModules, execOutputSaveInterval, upToBlock, out); err != nil {
		return nil, err
	}
	return out, nil
}

func buildMappersStorageState(ctx context.Context, mapModules []*pbsubstreams.Module, execOutputSaveInterval, upToBlock uint64, out ModuleStorageStateMap) error {
	// TODO(abourget): fetch execout states

	for _, mod := range mapModules {
		snapshot := "TODO: hmmm.. need a snapshot fetcher here!"
		state, err := state.NewExecOutputStorageState(mod.Name, mod.InitialBlock, upToBlock, snapshot)
		if err != nil {
			return fmt.Errorf("new map state: %w", err)
		}
		out[mod.Name] = state
	}
	return nil
}
func buildStoresStorageState(ctx context.Context, storeConfigMap store.ConfigMap, storeSnapshotsSaveInterval, upToBlock uint64, out ModuleStorageStateMap) error {
	logger := reqctx.Logger(ctx)

	state, err := store2.FetchStoresState(ctx, storeConfigMap)
	if err != nil {
		return fmt.Errorf("fetching stores states: %w", err)
	}

	for _, config := range storeConfigMap {
		name := config.Name()
		snapshot, ok := state.Snapshots[name]
		if !ok {
			return fmt.Errorf("fatal: storage state not reported for module name %q", name)
		}

		moduleStorageState, err := store2.NewStoreStorageState(name, storeSnapshotsSaveInterval, config.ModuleInitialBlock(), upToBlock, snapshot)
		if err != nil {
			return fmt.Errorf("new file units %q: %w", name, err)
		}

		out[name] = moduleStorageState

		logger.Info("work plan for store module", zap.Object("work", moduleStorageState))
	}
	return nil
}

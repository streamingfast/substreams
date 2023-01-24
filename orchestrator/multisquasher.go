package orchestrator

import "C"
import (
	"context"
	"fmt"
	"strings"

	"github.com/streamingfast/substreams/block"
	"github.com/streamingfast/substreams/reqctx"
	"github.com/streamingfast/substreams/service/config"
	"github.com/streamingfast/substreams/storage"
	execoutState "github.com/streamingfast/substreams/storage/execout/state"
	"github.com/streamingfast/substreams/storage/store"
	storeState "github.com/streamingfast/substreams/storage/store/state"
	"go.uber.org/zap"
)

// MultiSquasher produces _complete_ stores, by merging backing partial stores.
type MultiSquasher struct {
	storeSquashers       map[string]squashable
	targetExclusiveBlock uint64
}

type squashable interface {
	launch(ctx context.Context)
	waitForCompletion(ctx context.Context) error
	squash(ctx context.Context, partialsChunks block.Ranges) error
	moduleName() string
}

// NewMultiSquasher receives stores, initializes them and fetches them from
// the existing storage. It prepares itself to receive Squash()
// requests that should correspond to what is missing for those stores
// to reach `targetExclusiveEndBlock`.  This is managed externally by the
// Scheduler/Strategy. Eventually, ideally, all components are
// synchronizes around the actual data: the state of storages
// present, the requests needed to fill in those stores up to the
// target block, etc..
func NewMultiSquasher(
	ctx context.Context,
	runtimeConfig config.RuntimeConfig,
	modulesStorageStateMap storage.ModuleStorageStateMap,
	storeConfigs store.ConfigMap,
	upToBlock uint64,
	onStoreCompletedUntilBlock func(storeName string, blockNum uint64),
) (*MultiSquasher, error) {
	logger := reqctx.Logger(ctx)
	storeSquashers := map[string]squashable{}

	for storeModuleName, moduleStorageState := range modulesStorageStateMap {
		switch storageState := moduleStorageState.(type) {
		case *storeState.StoreStorageState:
			storeConfig, found := storeConfigs[storeModuleName]
			if !found {
				return nil, fmt.Errorf("store %q not found", storeModuleName)
			}

			storeSquasher, err := buildStoreSquasher(ctx, runtimeConfig.CacheSaveInterval, storeConfig, logger, storageState, upToBlock, onStoreCompletedUntilBlock)
			if err != nil {
				return nil, err
			}

			storeSquashers[storeModuleName] = storeSquasher
			logger.Debug("store squasher initialized", zap.String("module_name", storeModuleName))

		case *execoutState.ExecOutputStorageState:
			storeSquashers[storeModuleName] = &NoopMapSquasher{name: storeModuleName}
			logger.Debug("noop squasher initialized", zap.String("module_name", storeModuleName))
		}
	}

	return &MultiSquasher{
		storeSquashers:       storeSquashers,
		targetExclusiveBlock: upToBlock,
	}, nil
}

func buildStoreSquasher(ctx context.Context, storeSnapshotsSaveInterval uint64, storeConfig *store.Config, logger *zap.Logger, storeStorageState *storeState.StoreStorageState, upToBlock uint64, onStoreCompletedUntilBlock func(storeName string, blockNum uint64)) (*StoreSquasher, error) {
	storeModuleName := storeConfig.Name()
	startingStore := storeConfig.NewFullKV(logger)

	// TODO(abourget): can we use the Factory here? Can we not rely on the fact it was created apriori?
	//  can we derive it from a prior store? Did we REALLY need to initialize the store from which this
	//  one is derived?
	var storeSquasher *StoreSquasher
	if storeStorageState.InitialCompleteRange == nil {
		logger.Debug("setting up initial store",
			zap.String("store", storeModuleName),
			zap.Object("initial_store_file", storeStorageState.InitialCompleteRange),
		)
		storeSquasher = NewStoreSquasher(startingStore, upToBlock, startingStore.InitialBlock(), storeSnapshotsSaveInterval, onStoreCompletedUntilBlock)
	} else {
		logger.Debug("loading initial store",
			zap.String("store", storeModuleName),
			zap.Object("initial_store_file", storeStorageState.InitialCompleteRange),
		)
		if err := startingStore.Load(ctx, storeStorageState.InitialCompleteRange.ExclusiveEndBlock); err != nil {
			return nil, fmt.Errorf("load store %q: range %s: %w", storeModuleName, storeStorageState.InitialCompleteRange, err)
		}
		storeSquasher = NewStoreSquasher(startingStore, upToBlock, storeStorageState.InitialCompleteRange.ExclusiveEndBlock, storeSnapshotsSaveInterval, onStoreCompletedUntilBlock)

		onStoreCompletedUntilBlock(storeModuleName, storeStorageState.InitialCompleteRange.ExclusiveEndBlock)
	}

	if len(storeStorageState.PartialsMissing) == 0 {
		storeSquasher.targetExclusiveEndBlockReach = true
	}
	if len(storeStorageState.PartialsPresent) != 0 {
		if err := storeSquasher.squash(ctx, storeStorageState.PartialsPresent); err != nil {
			return nil, fmt.Errorf("first squash: %w", err)
		}
	}
	return storeSquasher, nil
}

func (s *MultiSquasher) Launch(ctx context.Context) {
	for _, squasher := range s.storeSquashers {
		go squasher.launch(ctx)
	}
}

func (s *MultiSquasher) Squash(ctx context.Context, moduleName string, partialsRanges block.Ranges) error {
	squashableStore, ok := s.storeSquashers[moduleName]
	if !ok {
		return fmt.Errorf("module %q was not found in storeSquashers module registry", moduleName)
	}

	return squashableStore.squash(ctx, partialsRanges)
}

func (s *MultiSquasher) Wait(ctx context.Context) (out store.Map, err error) {
	if err := s.waitUntilCompleted(ctx); err != nil {
		return nil, fmt.Errorf("waiting for squashers to complete: %w", err)
	}

	out, err = s.getFinalStores()
	if err != nil {
		return nil, fmt.Errorf("get final stores: %w", err)
	}

	return
}

func (s *MultiSquasher) waitUntilCompleted(ctx context.Context) error {
	logger := reqctx.Logger(ctx)
	logger.Info("squasher waiting until all are completed",
		zap.Int("store_count", len(s.storeSquashers)),
	)
	for _, squashableStore := range s.storeSquashers {
		logger.Info("shutting down squasher",
			zap.String("module", squashableStore.moduleName()),
		)
		if err := squashableStore.waitForCompletion(ctx); err != nil {
			return fmt.Errorf("%q completed with err: %w", squashableStore.moduleName(), err)
		}
	}
	return nil
}

func (s *MultiSquasher) getFinalStores() (out store.Map, err error) {
	out = store.NewMap()
	var errs []string
	for _, squashable := range s.storeSquashers {
		if storeSquasher, ok := squashable.(*StoreSquasher); ok {
			if !storeSquasher.targetExclusiveEndBlockReach {
				errs = append(errs, fmt.Sprintf("module %s: target %d not reached (next expected: %d)", storeSquasher.moduleName(), s.targetExclusiveBlock, storeSquasher.nextExpectedStartBlock))
			}
			if !storeSquasher.IsEmpty() {
				errs = append(errs, fmt.Sprintf("module %s: missing ranges %s", storeSquasher.moduleName(), storeSquasher.ranges))
			}

			out.Set(storeSquasher.store)
		}
	}
	if len(errs) != 0 {
		return nil, fmt.Errorf("%d errors: %s", len(errs), strings.Join(errs, "; "))
	}
	return out, nil
}

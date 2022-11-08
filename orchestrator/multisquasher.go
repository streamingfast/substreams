package orchestrator

import "C"
import (
	"context"
	"fmt"
	"github.com/streamingfast/substreams/orchestrator/storagestate"
	"github.com/streamingfast/substreams/reqctx"
	"strings"

	"github.com/streamingfast/substreams/block"
	"github.com/streamingfast/substreams/service/config"
	"github.com/streamingfast/substreams/store"
	"go.uber.org/zap"
)

// MultiSquasher produces _complete_ stores, by merging backing partial stores.
type MultiSquasher struct {
	storeSquashers       map[string]*StoreSquasher
	targetExclusiveBlock uint64
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
	modulesStorageStateMap storagestate.ModuleStorageStateMap,
	storeConfigs store.ConfigMap,
	upToBlock uint64,
	onStoreCompletedUntilBlock func(storeName string, blockNum uint64),
) (*MultiSquasher, error) {
	logger := reqctx.Logger(ctx)
	storeSquashers := map[string]*StoreSquasher{}
	for storeModuleName, moduleStorageState := range modulesStorageStateMap {
		storeStorageState, ok := moduleStorageState.(*storagestate.StoreStorageState)
		if !ok {
			continue
		}
		// TODO(abourget): type check the ModuleState here, and continue with the StoreModuleState only
		storeConfig, found := storeConfigs[storeModuleName]
		if !found {
			return nil, fmt.Errorf("store %q not found", storeModuleName)
		}

		startingStore := storeConfig.NewFullKV(logger)

		// TODO(abourget): can we use the Factory here? Can we not rely on the fact it was created apriori?
		// can we derive it from a prior store? Did we REALLY need to initialize the store from which this
		// one is derived?
		var storeSquasher *StoreSquasher
		if storeStorageState.InitialCompleteRange == nil {
			logger.Debug("setting up initial store",
				zap.String("store", storeModuleName),
				zap.Object("initial_store_file", storeStorageState.InitialCompleteRange),
			)
			storeSquasher = NewStoreSquasher(startingStore, upToBlock, startingStore.InitialBlock(), uint64(runtimeConfig.StoreSnapshotsSaveInterval), onStoreCompletedUntilBlock)
		} else {
			logger.Debug("loading initial store",
				zap.String("store", storeModuleName),
				zap.Object("initial_store_file", storeStorageState.InitialCompleteRange),
			)
			if err := startingStore.Load(ctx, storeStorageState.InitialCompleteRange.ExclusiveEndBlock); err != nil {
				return nil, fmt.Errorf("load store %q: range %s: %w", storeModuleName, storeStorageState.InitialCompleteRange, err)
			}
			storeSquasher = NewStoreSquasher(startingStore, upToBlock, storeStorageState.InitialCompleteRange.ExclusiveEndBlock, uint64(runtimeConfig.StoreSnapshotsSaveInterval), onStoreCompletedUntilBlock)

			onStoreCompletedUntilBlock(storeModuleName, storeStorageState.InitialCompleteRange.ExclusiveEndBlock)
		}

		if len(storeStorageState.PartialsMissing) == 0 {
			storeSquasher.targetExclusiveEndBlockReach = true
		}

		if len(storeStorageState.PartialsPresent) != 0 {
			storeSquasher.squash(storeStorageState.PartialsPresent)
		}

		storeSquashers[storeModuleName] = storeSquasher

		logger.Info("store squasher initialized",
			zap.String("module_name", storeModuleName),
		)
	}

	squasher := &MultiSquasher{
		storeSquashers:       storeSquashers,
		targetExclusiveBlock: upToBlock,
	}

	return squasher, nil
}

func (s *MultiSquasher) Launch(ctx context.Context) {
	for _, squasher := range s.storeSquashers {
		go squasher.launch(ctx)
	}
}

func (s *MultiSquasher) Squash(moduleName string, partialsRanges block.Ranges) error {
	squashable, ok := s.storeSquashers[moduleName]
	if !ok {
		return fmt.Errorf("module %q was not found in storeSquashers module registry", moduleName)
	}

	return squashable.squash(partialsRanges)
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
	logger.Info("squasher waiting till squasher stores are completed",
		zap.Int("store_count", len(s.storeSquashers)),
	)
	for _, squashable := range s.storeSquashers {
		logger.Info("shutting down store squasher",
			zap.String("store", squashable.name),
		)
		if err := squashable.WaitForCompletion(ctx); err != nil {
			return fmt.Errorf("%q completed with err: %w", squashable.name, err)
		}
	}
	return nil
}

func (s *MultiSquasher) getFinalStores() (out store.Map, err error) {
	out = store.NewMap()
	var errs []string
	for _, squashable := range s.storeSquashers {
		if !squashable.targetExclusiveEndBlockReach {
			errs = append(errs, fmt.Sprintf("module %s: target %d not reached (next expected: %d)", squashable.store.String(), s.targetExclusiveBlock, squashable.nextExpectedStartBlock))
		}
		if !squashable.IsEmpty() {
			errs = append(errs, fmt.Sprintf("module %s: missing ranges %s", squashable.store.String(), squashable.ranges))
		}

		out.Set(squashable.store)
	}
	if len(errs) != 0 {
		return nil, fmt.Errorf("%d errors: %s", len(errs), strings.Join(errs, "; "))
	}
	return out, nil
}

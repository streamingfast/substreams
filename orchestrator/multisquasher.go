package orchestrator

import "C"
import (
	"context"
	"fmt"
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
	workPlan *WorkPlan,
	storeConfigs store.ConfigMap,
	upToBlock uint64,
	onStoreCompletedUntilBlock func(storeName string, blockNum uint64),
) (*MultiSquasher, error) {
	storeSquashers := map[string]*StoreSquasher{}
	zlog.Info("creating a new squasher", zap.Int("work_plan_count", workPlan.StoreCount()))

	for storeModuleName, workUnit := range workPlan.workUnitsMap {
		storeConfig, found := storeConfigs[storeModuleName]
		if !found {
			return nil, fmt.Errorf("store %q not found", storeModuleName)
		}

		startingStore := storeConfig.NewFullKV(zlog)

		// TODO(abourget): can we use the Factory here? Can we not rely on the fact it was created apriori?
		// can we derive it from a prior store? Did we REALLY need to initialize the store from which this
		// one is derived?
		var storeSquasher *StoreSquasher
		if workUnit.initialCompleteRange == nil {
			zlog.Info("setting up initial store",
				zap.String("store", storeModuleName),
				zap.Object("initial_store_file", workUnit.initialCompleteRange),
			)
			storeSquasher = NewStoreSquasher(startingStore, upToBlock, startingStore.InitialBlock(), runtimeConfig.StoreSnapshotsSaveInterval, onStoreCompletedUntilBlock)
		} else {
			zlog.Info("loading initial store",
				zap.String("store", storeModuleName),
				zap.Object("initial_store_file", workUnit.initialCompleteRange),
			)
			if err := startingStore.Load(ctx, workUnit.initialCompleteRange.ExclusiveEndBlock); err != nil {
				return nil, fmt.Errorf("load store %q: range %s: %w", storeModuleName, workUnit.initialCompleteRange, err)
			}
			storeSquasher = NewStoreSquasher(startingStore, upToBlock, workUnit.initialCompleteRange.ExclusiveEndBlock, runtimeConfig.StoreSnapshotsSaveInterval, onStoreCompletedUntilBlock)

			onStoreCompletedUntilBlock(storeModuleName, workUnit.initialCompleteRange.ExclusiveEndBlock)
		}

		if len(workUnit.partialsMissing) == 0 {
			storeSquasher.targetExclusiveEndBlockReach = true
		}

		if len(workUnit.partialsPresent) != 0 {
			storeSquasher.squash(workUnit.partialsPresent)
		}

		storeSquashers[storeModuleName] = storeSquasher
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
	zlog.Info("squasher waiting till squasher stores are completed",
		zap.Int("store_count", len(s.storeSquashers)),
	)
	for _, squashable := range s.storeSquashers {
		zlog.Info("shutting down store squasher",
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

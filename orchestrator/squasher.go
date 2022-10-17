package orchestrator

import "C"
import (
	"context"
	"fmt"
	"strings"

	"github.com/streamingfast/substreams/block"
	"github.com/streamingfast/substreams/store"
	"go.uber.org/zap"
)

// TODO(abourget): Rename to MultiSquasher

// Squasher produces _complete_ stores, by merging backing partial stores.
type Squasher struct {
	storeSquashers       map[string]*StoreSquasher
	storeSaveInterval    uint64
	targetExclusiveBlock uint64
}

// NewSquasher receives stores, initializes them and fetches them from
// the existing storage. It prepares itself to receive Squash()
// requests that should correspond to what is missing for those stores
// to reach `targetExclusiveEndBlock`.  This is managed externally by the
// Scheduler/Strategy. Eventually, ideally, all components are
// synchronizes around the actual data: the state of storages
// present, the requests needed to fill in those stores up to the
// target block, etc..
func NewSquasher(
	ctx context.Context,
	workPlan WorkPlan,
	storeMap *store.Map,
	reqEffectiveStartBlock uint64,
	storeSaveInterval uint64,
	jobsPlanner *JobsPlanner) (*Squasher, error) {
	storeSquashers := map[string]*StoreSquasher{}
	zlog.Info("creating a new squasher", zap.Int("work_plan_count", len(workPlan)))

	for storeModuleName, workUnit := range workPlan {
		genericStore, found := storeMap.Get(storeModuleName)
		if !found {
			return nil, fmt.Errorf("store %q not found", storeModuleName)
		}

		s, ok := genericStore.(store.Cloneable)
		if !ok {
			return nil, fmt.Errorf("can only run squasher on kv stores and not kv partial stores")
		}
		clonedStore := s.Clone()

		// TODO(abourget): can we use the Factory here? Can we not rely on the fact it was created apriori?
		// can we derive it from a prior store? Did we REALLY need to initialize the store from which this
		// one is derived?
		var storeSquasher *StoreSquasher
		if workUnit.initialCompleteRange == nil {
			zlog.Info("setting up initial store",
				zap.String("store", storeModuleName),
				zap.Object("initial_store_file", workUnit.initialCompleteRange),
			)
			storeSquasher = NewStoreSquasher(clonedStore, reqEffectiveStartBlock, clonedStore.InitialBlock(), storeSaveInterval, jobsPlanner)
		} else {
			zlog.Info("loading initial store",
				zap.String("store", storeModuleName),
				zap.Object("initial_store_file", workUnit.initialCompleteRange),
			)
			if err := clonedStore.Load(ctx, workUnit.initialCompleteRange.ExclusiveEndBlock); err != nil {
				return nil, fmt.Errorf("load store %q: range %s: %w", storeModuleName, workUnit.initialCompleteRange, err)
			}
			storeSquasher = NewStoreSquasher(clonedStore, reqEffectiveStartBlock, workUnit.initialCompleteRange.ExclusiveEndBlock, storeSaveInterval, jobsPlanner)

			jobsPlanner.SignalCompletionUpUntil(storeModuleName, workUnit.initialCompleteRange.ExclusiveEndBlock)
		}

		if len(workUnit.partialsMissing) == 0 {
			storeSquasher.targetExclusiveEndBlockReach = true
		}

		go storeSquasher.launch(ctx)
		storeSquashers[storeModuleName] = storeSquasher
	}

	squasher := &Squasher{
		storeSquashers:       storeSquashers,
		targetExclusiveBlock: reqEffectiveStartBlock,
	}
	return squasher, nil
}

func (s *Squasher) WaitUntilCompleted(ctx context.Context) error {
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

func (s *Squasher) Squash(moduleName string, partialsRanges block.Ranges) error {
	squashable, ok := s.storeSquashers[moduleName]
	if !ok {
		return fmt.Errorf("module %q was not found in storeSquashers module registry", moduleName)
	}

	return squashable.squash(partialsRanges)
}

func (s *Squasher) ValidateStoresReady() (out map[string]store.Store, err error) {
	out = map[string]store.Store{}
	var errs []string
	for name, squashable := range s.storeSquashers {
		if !squashable.targetExclusiveEndBlockReach {
			errs = append(errs, fmt.Sprintf("module %s: target %d not reached (next expected: %d)", squashable.store.String(), s.targetExclusiveBlock, squashable.nextExpectedStartBlock))
		}
		if !squashable.IsEmpty() {
			errs = append(errs, fmt.Sprintf("module %s: missing ranges %s", squashable.store.String(), squashable.ranges))
		}

		out[name] = squashable.store
	}
	if len(errs) != 0 {
		return nil, fmt.Errorf("%d errors: %s", len(errs), strings.Join(errs, "; "))
	}
	return out, nil
}

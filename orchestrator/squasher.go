package orchestrator

import (
	"context"
	"fmt"
	"strings"

	"go.uber.org/zap"

	"github.com/streamingfast/shutter"
	"github.com/streamingfast/substreams/block"
	"github.com/streamingfast/substreams/state"
)

// Squasher produces _complete_ stores, by merging backing partial stores.
type Squasher struct {
	*shutter.Shutter
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
func NewSquasher(ctx context.Context, workPlan WorkPlan, stores map[string]*state.Store, reqStartBlock uint64, jobsPlanner *JobsPlanner) (*Squasher, error) {
	storeSquashers := map[string]*StoreSquasher{}
	for modName, workUnit := range workPlan {
		store := stores[modName]
		var storeSquasher *StoreSquasher
		if workUnit.initialStoreFile == nil {
			zlog.Info("setting up initial store", zap.String("store", store.Name), zap.Object("initial_store_fiel", workUnit.initialStoreFile))
			storeSquasher = NewStoreSquasher(store.CloneStructure(store.ModuleInitialBlock), reqStartBlock, store.ModuleInitialBlock, jobsPlanner)
		} else {
			zlog.Info("loading initial store", zap.String("store", store.Name), zap.Object("initial_store_fiel", workUnit.initialStoreFile))
			squish, err := store.LoadFrom(ctx, workUnit.initialStoreFile)
			if err != nil {
				return nil, fmt.Errorf("loading store %q: range %s: %w", store.Name, workUnit.initialStoreFile, err)
			}
			storeSquasher = NewStoreSquasher(squish, reqStartBlock, workUnit.initialStoreFile.ExclusiveEndBlock, jobsPlanner)

			jobsPlanner.SignalCompletionUpUntil(modName, workUnit.initialStoreFile.ExclusiveEndBlock)
		}

		if len(workUnit.partialsMissing) == 0 {
			storeSquasher.targetExclusiveEndBlockReach = true
		}

		go storeSquasher.launch(ctx)
		storeSquashers[store.Name] = storeSquasher
	}

	squasher := &Squasher{
		Shutter:              shutter.New(),
		storeSquashers:       storeSquashers,
		targetExclusiveBlock: reqStartBlock,
	}

	squasher.OnTerminating(func(err error) {
		zlog.Info("squasher terminating", zap.Error(err))
		for _, squashable := range storeSquashers {
			zlog.Info("shutting down store squasher", zap.String("store", squashable.name))
			squashable.Shutter.Shutdown(err)
		}
	})

	return squasher, nil
}

func (s *Squasher) Squash(moduleName string, partialsRanges block.Ranges) error {
	squashable, ok := s.storeSquashers[moduleName]
	if !ok {
		return fmt.Errorf("module %q was not found in storeSquashers module registry", moduleName)
	}

	return squashable.squash(partialsRanges)
}

func (s *Squasher) ValidateStoresReady() (out map[string]*state.Store, err error) {
	out = map[string]*state.Store{}
	var errs []string
	for _, squashable := range s.storeSquashers {
		if !squashable.targetExclusiveEndBlockReach {
			errs = append(errs, fmt.Sprintf("module %q: target %d not reached (next expected: %d)", squashable.name, s.targetExclusiveBlock, squashable.nextExpectedStartBlock))
		}
		if !squashable.IsEmpty() {
			errs = append(errs, fmt.Sprintf("module %q: missing ranges %s", squashable.name, squashable.ranges))
		}

		out[squashable.store.Name] = squashable.store
	}
	if len(errs) != 0 {
		return nil, fmt.Errorf("%d errors: %s", len(errs), strings.Join(errs, "; "))
	}
	return out, nil
}

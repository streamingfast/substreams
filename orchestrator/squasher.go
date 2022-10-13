package orchestrator

import "C"
import (
	"context"
	"fmt"
	"strings"

	"github.com/streamingfast/shutter"
	"github.com/streamingfast/substreams/block"
	"github.com/streamingfast/substreams/store"
	"go.uber.org/zap"
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
func NewSquasher(
	ctx context.Context,
	workPlan WorkPlan,
	storeMap *store.Map,
	reqStartBlock uint64,
	storeSaveInterval uint64,
	jobsPlanner *JobsPlanner) (*Squasher, error) {
	storeSquashers := map[string]*StoreSquasher{}
	for modName, workUnit := range workPlan {
		genericStore, found := storeMap.Get(modName)
		if !found {
			return nil, fmt.Errorf("store %q not found", modName)
		}

		s, ok := genericStore.(store.Cloneable)
		if !ok {
			return nil, fmt.Errorf("can only run sqausher on kv stores and not kv partial stores")
		}
		clonedStore := s.Clone()

		var storeSquasher *StoreSquasher
		if workUnit.initialStoreFile == nil {
			zlog.Info("setting up initial store",
				zap.String("store", modName),
				zap.Object("initial_store_file", workUnit.initialStoreFile),
			)
			storeSquasher = NewStoreSquasher(clonedStore, reqStartBlock, clonedStore.InitialBlock(), storeSaveInterval, jobsPlanner)
		} else {
			zlog.Info("loading initial store",
				zap.String("store", modName),
				zap.Object("initial_store_file", workUnit.initialStoreFile),
			)
			if err := clonedStore.Load(ctx, workUnit.initialStoreFile.ExclusiveEndBlock); err != nil {
				return nil, fmt.Errorf("load store %q: range %s: %w", modName, workUnit.initialStoreFile, err)
			}
			storeSquasher = NewStoreSquasher(clonedStore, reqStartBlock, workUnit.initialStoreFile.ExclusiveEndBlock, storeSaveInterval, jobsPlanner)

			jobsPlanner.SignalCompletionUpUntil(modName, workUnit.initialStoreFile.ExclusiveEndBlock)
		}

		if len(workUnit.partialsMissing) == 0 {
			storeSquasher.targetExclusiveEndBlockReach = true
		}

		go storeSquasher.launch(ctx)
		storeSquashers[modName] = storeSquasher
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

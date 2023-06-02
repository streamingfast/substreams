package squasher

import "C"
import (
	"context"
	"fmt"
	"strings"

	"go.uber.org/zap"

	"github.com/streamingfast/substreams/orchestrator/loop"
	"github.com/streamingfast/substreams/reqctx"
	"github.com/streamingfast/substreams/service/config"
	"github.com/streamingfast/substreams/storage"
	execoutState "github.com/streamingfast/substreams/storage/execout/state"
	"github.com/streamingfast/substreams/storage/store"
	storeState "github.com/streamingfast/substreams/storage/store/state"
)

// Multi produces _complete_ stores, by merging backing partial stores.
//
// It also produces complete store snapshots, which are dependencies
// for the segment following the snapshot.
type Multi struct {
	Modules              map[string]squashable
	targetExclusiveBlock uint64
}

// NewMulti receives stores, initializes them and fetches them from
// the existing storage. It prepares itself to receive Squash()
// requests that should correspond to what is missing for those stores
// to reach `targetExclusiveEndBlock`.  This is managed externally by the
// Scheduler/Strategy. Eventually, ideally, all components are
// synchronizes around the actual data: the state of storages
// present, the requests needed to fill in those stores up to the
// target block, etc..
func NewMulti(
	ctx context.Context,
	runtimeConfig config.RuntimeConfig,
	modulesStorageStateMap storage.ModuleStorageStateMap,
	storeConfigs store.ConfigMap,
	upToBlock uint64,
	onStoreCompletedUntilBlock func(storeName string, blockNum uint64),
) (*Multi, error) {
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

	return &Multi{
		Modules:              storeSquashers,
		targetExclusiveBlock: upToBlock,
	}, nil
}

func buildStoreSquasher(
	ctx context.Context,
	storeSnapshotsSaveInterval uint64,
	storeConfig *store.Config,
	logger *zap.Logger,
	storeStorageState *storeState.StoreStorageState,
	upToBlock uint64,
	onStoreCompletedUntilBlock func(storeName string, blockNum uint64),
) (storeSquasher *Single, err error) {

	storeModuleName := storeConfig.Name()
	startingStore := storeConfig.NewFullKV(logger)

	// TODO(abourget): can we use the Factory here? Can we not rely on the fact it was created apriori?
	//  can we derive it from a prior store? Did we REALLY need to initialize the store from which this
	//  one is derived?
	if storeStorageState.InitialCompleteFile == nil {
		logger.Debug("setting up initial store",
			zap.String("store", storeModuleName),
			zap.String("initial_store_range", "None"),
		)
		storeSquasher = NewSingle(startingStore, upToBlock, startingStore.InitialBlock(), storeSnapshotsSaveInterval, onStoreCompletedUntilBlock)
	} else {
		initialRange := storeStorageState.InitialCompleteFile.Range
		logger.Debug("loading initial store", zap.String("store", storeModuleName), zap.Stringer("initial_store_range", initialRange))
		if err := startingStore.Load(ctx, storeStorageState.InitialCompleteFile); err != nil {
			return nil, fmt.Errorf("load store %q with initial complete range %q: %w", storeModuleName, initialRange, err)
		}

		storeSquasher = NewSingle(startingStore, upToBlock, initialRange.ExclusiveEndBlock, storeSnapshotsSaveInterval, onStoreCompletedUntilBlock)

		onStoreCompletedUntilBlock(storeModuleName, initialRange.ExclusiveEndBlock)
	}

	if len(storeStorageState.PartialsMissing) == 0 {
		storeSquasher.targetExclusiveEndBlockReach = true
	}

	return storeSquasher, nil
}

//func (s *Multi) Launch(ctx context.Context) {
//	for _, squasher := range s.Modules {
//		go squasher.launch(ctx)
//	}
//}

func (s *Multi) MergeNextRange(modName string) loop.Cmd {
	single := s.Modules[modName]
	return single.MergeNextRange()
}

func (s *Multi) AddPartials(modName string, files ...*store.File) loop.Cmd {
	single := s.Modules[modName]
	return single.AddPartials(files...)
}

//func (s *Multi) Squash(ctx context.Context, moduleName string, partialsFiles store.FileInfos) error {
//	squashableStore, ok := s.Modules[moduleName]
//	if !ok {
//		return fmt.Errorf("module %q was not found in storeSquashers module registry", moduleName)
//	}
//
//	return squashableStore.squash(ctx, partialsFiles)
//}

func (s *Multi) Wait(ctx context.Context) (out store.Map, err error) {
	if err := s.waitUntilCompleted(ctx); err != nil {
		return nil, fmt.Errorf("waiting for squashers to complete: %w", err)
	}

	out, err = s.getFinalStores()
	if err != nil {
		return nil, fmt.Errorf("get final stores: %w", err)
	}

	return
}

func (s *Multi) waitUntilCompleted(ctx context.Context) error {
	logger := reqctx.Logger(ctx)
	logger.Info("squasher waiting until all are completed",
		zap.Int("store_count", len(s.Modules)),
	)
	for _, squashableStore := range s.Modules {
		logger.Info("shutting down squasher",
			zap.String("module", squashableStore.moduleName()),
		)
		if err := squashableStore.waitForCompletion(ctx); err != nil {
			return fmt.Errorf("%q completed with err: %w", squashableStore.moduleName(), err)
		}
	}
	return nil
}

func (s *Multi) FinalStoreMap() store.Map {
	out := store.NewMap()
	for _, squashable := range s.Modules {
		if storeSquasher, ok := squashable.(*Single); ok {
			out[storeSquasher.moduleName()] = storeSquasher.store
		}
	}
	return out
}

func (s *Multi) getFinalStores() (out store.Map, err error) {
	// TODO: those here are conditions to interrupt the scheduler, as
	//  they are the final goal of the Scheduler: produce such a map.
	//  The event loop would constantly check for such conditions
	//  and terminate its loop with the final map.
	out = store.NewMap()
	var errs []string
	for _, squashable := range s.Modules {
		if storeSquasher, ok := squashable.(*Single); ok {
			if !storeSquasher.targetExclusiveEndBlockReach {
				errs = append(errs, fmt.Sprintf("module %s: target %d not reached (next expected: %d)", storeSquasher.moduleName(), s.targetExclusiveBlock, storeSquasher.nextExpectedStartBlock))
			}
			if !storeSquasher.IsEmpty() {
				errs = append(errs, fmt.Sprintf("module %s: missing ranges %s", storeSquasher.moduleName(), storeSquasher.files))
			}

			out.Set(storeSquasher.store)
		}
	}
	if len(errs) != 0 {
		return nil, fmt.Errorf("%d errors: %s", len(errs), strings.Join(errs, "; "))
	}
	return out, nil
}

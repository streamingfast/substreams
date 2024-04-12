package stage

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/hashicorp/go-multierror"
	"github.com/streamingfast/substreams/block"
	"github.com/streamingfast/substreams/reqctx"
	"github.com/streamingfast/substreams/storage/store"
)

// TODO: unify Complete (deprecate word) and FullKV, partial files and PartialKV

// multiSquash is called only when we know that the given mergeUnit has a PartialKV present
// and we know that there is a FullKV store that exists on the previous segment.
// We can therefore, for each module: either use the `store` we have in cache (which was
// perhaps used to _produce_ that prior FullKV), or load it from storage.
// This allows for both initialization of the store, and skipping of FullKV if we
// happen to have some that were deleted.
func (s *Stages) multiSquash(stage *Stage, mergeUnit Unit) error {
	if stage.kind != KindStore {
		panic("multiSquash called on non-store stage")
	}

	// Launch parallel jobs to merge all stages' stores.
	for _, modState := range stage.storeModuleStates {
		if mergeUnit.Segment < modState.segmenter.FirstIndex() {
			continue
		}

		modState := modState // capture in loop
		stage.syncWork.Go(func() error {
			stats := reqctx.ReqStats(s.ctx)
			stats.RecordModuleMerging(modState.name)
			defer stats.RecordModuleMergeComplete(modState.name)
			err := s.singleSquash(stage, modState, mergeUnit)
			if err != nil {
				return fmt.Errorf("squash stage %d module %q: %w", stage.idx, modState.name, err)
			}
			return nil
		})
	}

	return stage.syncWork.Wait()
}

type Result struct {
	partialKVStore *store.PartialKV
	partialFile    *store.FileInfo
	fullKVStore    *store.FullKV
	error          error
}

func getPartialOrFullKV(ctx context.Context, modState *StoreModuleState, rng *block.Range) (*store.PartialKV, *store.FileInfo, *store.FullKV, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	results := make(chan Result, 2)
	go func() {
		partialFile := store.NewPartialFileInfo(modState.name, rng.StartBlock, rng.ExclusiveEndBlock)
		partial := modState.derivePartialKV(rng.StartBlock)
		err := partial.Load(ctx, partialFile)
		results <- Result{partialKVStore: partial, partialFile: partialFile, error: err}
	}()

	go func() {
		nextFull, err := modState.getStore(ctx, rng.ExclusiveEndBlock)
		results <- Result{fullKVStore: nextFull, error: err}
	}()

	var err error
	for i := 0; i < 2; i++ {
		select {
		case <-ctx.Done():
			err = multierror.Append(err, ctx.Err())
			return nil, nil, nil, err

		case result := <-results:
			if result.error != nil {
				err = multierror.Append(err, result.error)
				break // from select
			}
			if result.fullKVStore != nil {
				return nil, nil, result.fullKVStore, nil
			}
			if result.partialKVStore != nil {
				return result.partialKVStore, result.partialFile, nil, nil
			}
		}
	}
	return nil, nil, nil, fmt.Errorf("getting partial or full kv: %w", err)
}

// singleSquash gets the current fullKV and merges the next partialKV into it.
// If there is an existing fullKV at the destination (next segment), it will be loaded instead (whichever file is seen first)
func (s *Stages) singleSquash(stage *Stage, modState *StoreModuleState, mergeUnit Unit) error {
	metrics := mergeMetrics{}
	metrics.start = time.Now()
	metrics.stage = stage.idx
	metrics.moduleName = modState.name
	metrics.moduleHash = modState.storeConfig.ModuleHash()

	rng := modState.segmenter.Range(mergeUnit.Segment)
	metrics.blockRange = rng
	segmentEndsOnInterval := modState.segmenter.EndsOnInterval(mergeUnit.Segment)

	// Retrieve store to merge, from cache or load from storage. Allows skipping of segments
	// for handling partials interspearsed with full KVs.
	fullKV, err := modState.getStore(s.ctx, rng.StartBlock) // loads+caches or uses cached store
	if err != nil {
		return fmt.Errorf("getting store: %w", err)
	}

	// Load
	metrics.loadStart = time.Now()
	partialKV, partialFile, newFullKV, err := getPartialOrFullKV(s.ctx, modState, rng)
	if err != nil {
		return err
	}
	metrics.loadEnd = time.Now()
	if s.ctx.Err() != nil {
		return s.ctx.Err()
	}

	if newFullKV != nil {
		modState.cachedStore = newFullKV
		modState.lastBlockInStore = rng.ExclusiveEndBlock
		s.logger.Info("squashing time metrics (skipped, loaded from full kv)", metrics.logFields()...)
		return nil
	}

	// Merge
	metrics.mergeStart = time.Now()
	if err := fullKV.Merge(partialKV); err != nil {
		return fmt.Errorf("merging: %w", err)
	}

	modState.lastBlockInStore = rng.ExclusiveEndBlock
	metrics.mergeEnd = time.Now()

	s.logger.Info("deleting partial store", zap.Stringer("store", partialKV))
	stage.asyncWork.Go(func() error {
		return partialKV.DeleteStore(s.ctx, partialFile)
	})

	// Flush full store
	if segmentEndsOnInterval {
		metrics.saveStart = time.Now()
		_, writer, err := fullKV.Save(rng.ExclusiveEndBlock)
		if err != nil {
			return fmt.Errorf("save full store: %w", err)
		}
		metrics.saveEnd = time.Now()

		stage.asyncWork.Go(func() error {
			return writer.Write(context.Background()) // always write files here even if the request was cancelled.
		})
	}

	s.logger.Info("squashing time metrics", metrics.logFields()...)

	return nil
}

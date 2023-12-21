package stage

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"

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
	for _, modState := range stage.moduleStates {
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

// The singleSquash operation's goal is to take the up-most contiguous unit
// tha is compete, and take the very next partial, squash it and produce a FullKV
// store.
// If we happen to have some FullKV stores in the middle, then our goal is
// to load that compete store, and squash the next partial segment.
// We keep the cache of the latest FullKV store, to speed up things
// if they are linear
func (s *Stages) singleSquash(stage *Stage, modState *ModuleState, mergeUnit Unit) error {
	metrics := mergeMetrics{}
	metrics.start = time.Now()

	rng := modState.segmenter.Range(mergeUnit.Segment)
	partialFile := store.NewPartialFileInfo(modState.name, rng.StartBlock, rng.ExclusiveEndBlock, s.traceID)
	partialKV := modState.derivePartialKV(rng.StartBlock)
	segmentEndsOnInterval := modState.segmenter.EndsOnInterval(mergeUnit.Segment)

	// Retrieve store to merge, from cache or load from storage. Allows skipping of segments
	// for handling partials interspearsed with full KVs.
	fullKV, err := modState.getStore(s.ctx, rng.StartBlock) // loads+caches or uses cached store
	if err != nil {
		return fmt.Errorf("getting store: %w", err)
	}

	// Load
	metrics.loadStart = time.Now()
	if err := partialKV.Load(s.ctx, partialFile); err != nil {
		return fmt.Errorf("loading partial: %q: %w", partialFile.Filename, err)
	}
	metrics.loadEnd = time.Now()

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

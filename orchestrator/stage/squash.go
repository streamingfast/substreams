package stage

import (
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/streamingfast/substreams/reqctx"
	"github.com/streamingfast/substreams/storage/store"
)

func (s *Stages) multiSquash(stage *Stage, mergeUnit Unit) error {
	if stage.kind != KindStore {
		panic("multiSquash called on non-store stage")
	}

	// Launch parallel jobs to merge all stages' stores.
	for _, modState := range stage.moduleStates {
		if modState.segmenter.FirstIndex() < mergeUnit.Segment {
			continue
		}

		modState := modState
		stage.writerErrGroup.Go(func() error {
			err := s.singleSquash(stage, modState, mergeUnit)
			if err != nil {
				return fmt.Errorf("squash stage %d module %q: %w", stage.idx, modState.name, err)
			}
			return nil
		})
	}

	return stage.writerErrGroup.Wait()
}

func (s *Stages) singleSquash(stage *Stage, modState *ModuleState, mergeUnit Unit) error {
	metrics := mergeMetrics{}
	metrics.start = time.Now()

	rng := modState.segmenter.Range(mergeUnit.Segment)
	partialSegment := modState.segmenter.IsPartial(mergeUnit.Segment)
	nextStore := modState.store.DerivePartialStore(rng.StartBlock)

	partialFile := store.NewPartialFileInfo(modState.name, rng.StartBlock, rng.ExclusiveEndBlock, s.traceID)

	// Load
	metrics.loadStart = time.Now()
	if err := nextStore.Load(s.ctx, partialFile); err != nil {
		return fmt.Errorf("loading partial: %w: %w", err)
	}
	metrics.loadEnd = time.Now()

	// Merge
	metrics.mergeStart = time.Now()
	if err := modState.store.Merge(nextStore); err != nil {
		return fmt.Errorf("merging: %w", err)
	}
	metrics.mergeEnd = time.Now()

	// Delete partial store
	if reqctx.Details(s.ctx).ProductionMode || !partialSegment { /* FIXME: compute this elsewhere */
		s.logger.Info("deleting store", zap.Stringer("store", nextStore))
		stage.writerErrGroup.Go(func() error {
			return nextStore.DeleteStore(s.ctx, partialFile)
		})
	}

	// Flush full store
	if !partialSegment {
		metrics.saveStart = time.Now()
		_, writer, err := modState.store.Save(rng.ExclusiveEndBlock)
		if err != nil {
			return fmt.Errorf("save full store: %w", err)
		}
		metrics.saveEnd = time.Now()

		stage.writerErrGroup.Go(func() error {
			// TODO: could this cause an issue if the writing takes more time than when trying to opening the file??
			return writer.Write(s.ctx)
		})
	}

	s.logger.Info("squashing time metrics", metrics.logFields()...)

	return nil
}

func deletePartial() bool {
	if "in production mode" {
		return true
	}
	if "squashableFile.Range.ExclusiveEndBlock%s.storeSaveInterval == 0" {
		return true
	}
	// only leave it in dev mode, when we won't have a correspopnding
	// full store.
	return false
}

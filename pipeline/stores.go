package pipeline

import (
	"context"
	"fmt"
	"github.com/streamingfast/substreams/block"
	"time"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/reqctx"
	"github.com/streamingfast/substreams/store"
	"go.opentelemetry.io/otel/attribute"
	"go.uber.org/zap"
)

// TODO(abourget): eat away all these methods on the Pipeline here
// and turn them into `Stores` methods.
// Make THAT the Return value for the backprocessor and the
type Stores struct {
	bounder         *storeBoundary
	StoreMap        store.Map
	partialsWritten block.Ranges // when backprocessing, to report back to orchestrator
}

func NewStores(storeSnapshotSaveInterval, stopBlockNum uint64) *Stores {
	bounder := NewStoreBoundary(storeSnapshotSaveInterval, stopBlockNum)
	return &Stores{
		bounder: bounder,
	}
}

func (s *Stores) SetStoreMap(storeMap *store.StoreMap, isSubRequest bool, outputModules map[string]bool) {
	// TODO(abourget): assign vars
}

func (s *Stores) resetStores() {
	for _, s := range s.StoreMap.All() {
		if resetableStore, ok := s.(store.Resettable); ok {
			resetableStore.Reset()
		}
	}
}

func (s *Stores) flushStores(ctx context.Context, blockNum uint64) (err error) {
	reqDetails := reqctx.Details(ctx)
	logger := reqctx.Logger(ctx)
	reqStats := reqctx.ReqStats(ctx)
	boundaryIntervals := s.bounder.GetStoreFlushRanges(reqDetails.IsSubRequest, reqDetails.Request.StopBlockNum, blockNum)
	if len(boundaryIntervals) > 0 {
		logger.Info("flushing boundaries", zap.Uint64s("boundaries", boundaryIntervals))
	}
	reqctx.Span(ctx).SetAttributes(attribute.Int("pipeline.stores.boundary_reached", len(boundaryIntervals)))
	for _, boundaryBlock := range boundaryIntervals {
		t0 := time.Now()
		if err := s.saveStoresSnapshots(ctx, boundaryBlock); err != nil {
			return fmt.Errorf("saving stores snapshot at bound %d: %w", boundaryBlock, err)
		}

		reqStats.RecordFlush(time.Since(t0))
	}
	return nil
}
func (p *Pipeline) storesHandleUndo(moduleOutput *pbsubstreams.ModuleOutput) {
	if s, found := p.StoreMap.Get(moduleOutput.Name); found {
		if deltaStore, ok := s.(store.DeltaAccessor); ok {
			deltaStore.ApplyDeltasReverse(moduleOutput.GetStoreDeltas().GetDeltas())
		}
	}
}

func (s *Stores) saveStoresSnapshots(ctx context.Context, boundaryBlock uint64) (err error) {
	reqDetails := reqctx.Details(ctx)

	for name, oneStore := range s.StoreMap.All() {
		if reqDetails.SkipSnapshotSave(name) {
			continue
		}
		if err := s.saveStoreSnapshot(ctx, oneStore, boundaryBlock); err != nil {
			return fmt.Errorf("save store snapshot: %w", err)
		}
	}
	return nil
}

func (s *Stores) saveStoreSnapshot(ctx context.Context, saveStore store.Store, boundaryBlock uint64) (err error) {
	ctx, span := reqctx.WithSpan(ctx, "save_store_snapshot")
	span.SetAttributes(attribute.String("store", saveStore.Name()))
	defer span.EndWithErr(&err)

	blockRange, writer, err := saveStore.Save(boundaryBlock)
	if err != nil {
		return fmt.Errorf("saving store %q at boundary %d: %w", saveStore.Name(), boundaryBlock, err)
	}

	if err = writer.Write(ctx); err != nil {
		return fmt.Errorf("failed to write store: %w", err)
	}

	if reqctx.Details(ctx).ShouldReturnWrittenPartialsInTrailer(saveStore.Name()) {
		s.partialsWritten = append(s.partialsWritten, blockRange)
		reqctx.Logger(ctx).Debug("adding partials written", zap.Object("range", blockRange), zap.Stringer("ranges", s.partialsWritten), zap.Uint64("boundary_block", boundaryBlock))

		if v, ok := saveStore.(store.PartialStore); ok {
			reqctx.Span(ctx).AddEvent("store_roll_trigger")
			v.Roll(boundaryBlock)
		}
	}
	return nil
}

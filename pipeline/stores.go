package pipeline

import (
	"context"
	"fmt"
	"time"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/reqctx"
	"github.com/streamingfast/substreams/store"
	"go.opentelemetry.io/otel/attribute"
	"go.uber.org/zap"
)

func (p *Pipeline) resetStores() {
	for _, s := range p.StoreMap.All() {
		if resetableStore, ok := s.(store.Resettable); ok {
			resetableStore.Reset()
		}
	}
}

func (p *Pipeline) flushStores(ctx context.Context, blockNum uint64) (err error) {
	reqDetails := reqctx.Details(ctx)
	logger := reqctx.Logger(ctx)
	reqStats := reqctx.ReqStats(ctx)
	boundaryIntervals := p.bounder.GetStoreFlushRanges(reqDetails.IsSubRequest, reqDetails.Request.StopBlockNum, blockNum)
	if len(boundaryIntervals) > 0 {
		logger.Info("flushing boundaries", zap.Uint64s("boundaries", boundaryIntervals))
	}
	reqctx.Span(ctx).SetAttributes(attribute.Int("pipeline.stores.boundary_reached", len(boundaryIntervals)))
	for _, boundaryBlock := range boundaryIntervals {
		t0 := time.Now()
		if err := p.saveStoresSnapshots(ctx, boundaryBlock); err != nil {
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

func (p *Pipeline) saveStoresSnapshots(ctx context.Context, boundaryBlock uint64) (err error) {
	reqDetails := reqctx.Details(ctx)

	for name, s := range p.StoreMap.All() {
		// optimization because we know that in a subrequest we are only running through the last store (output)
		// all parent stores should have come from moduleOutput cache
		if reqDetails.IsSubRequest && !p.isOutputModule(name) {
			// skip saving snapshot for non-output stores in sub request
			continue
		}

		if err := p.saveStoreSnapshot(ctx, s, boundaryBlock); err != nil {
			return fmt.Errorf("save store snapshot: %w", err)
		}

	}
	return nil
}

func (p *Pipeline) saveStoreSnapshot(ctx context.Context, s store.Store, boundaryBlock uint64) (err error) {
	ctx, span := reqctx.WithSpan(ctx, "save_store_snapshot")
	span.SetAttributes(attribute.String("store", s.Name()))
	defer span.EndWithErr(&err)

	blockRange, writer, err := s.Save(boundaryBlock)
	if err != nil {
		return fmt.Errorf("saving store %q at boundary %d: %w", s.Name(), boundaryBlock, err)
	}

	if err = writer.Write(ctx); err != nil {
		return fmt.Errorf("failed to write store: %w", err)
	}

	if reqctx.Details(ctx).IsSubRequest && p.isOutputModule(s.Name()) {
		p.partialsWritten = append(p.partialsWritten, blockRange)
		reqctx.Logger(ctx).Debug("adding partials written", zap.Object("range", blockRange), zap.Stringer("ranges", p.partialsWritten), zap.Uint64("boundary_block", boundaryBlock))

		if v, ok := s.(store.PartialStore); ok {
			reqctx.Span(ctx).AddEvent("store_roll_trigger")
			v.Roll(boundaryBlock)
		}
	}
	return nil
}

package pipeline

import (
	"context"
	"fmt"
	"time"

	"github.com/streamingfast/substreams/orchestrator/outputmodules"
	store2 "github.com/streamingfast/substreams/storage/store"

	"github.com/streamingfast/dstore"
	"github.com/streamingfast/substreams/block"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/reqctx"
	"go.opentelemetry.io/otel/attribute"
	"go.uber.org/zap"
)

type Stores struct {
	isSubRequest    bool
	bounder         *storeBoundary
	configs         store2.ConfigMap
	StoreMap        store2.Map
	partialsWritten block.Ranges // when backprocessing, to report back to orchestrator
}

func NewStores(storeConfigs store2.ConfigMap, storeSnapshotSaveInterval, requestStartBlockNum, stopBlockNum uint64, isSubRequest bool) *Stores {
	bounder := NewStoreBoundary(storeSnapshotSaveInterval, requestStartBlockNum, stopBlockNum)
	return &Stores{
		configs:      storeConfigs,
		isSubRequest: isSubRequest,
		bounder:      bounder,
	}
}

func (s *Stores) SetStoreMap(storeMap store2.Map) {
	s.StoreMap = storeMap
}

func InitializeStoreConfigs(outputGraph *outputmodules.Graph, baseObjectStore dstore.Store) (out store2.ConfigMap, err error) {
	out = make(store2.ConfigMap)
	for _, storeModule := range outputGraph.Stores() {
		c, err := store2.NewConfig(
			storeModule.Name,
			storeModule.InitialBlock,
			outputGraph.ModuleHashes().Get(storeModule.Name),
			storeModule.GetKindStore().UpdatePolicy,
			storeModule.GetKindStore().ValueType,
			baseObjectStore,
		)
		if err != nil {
			return nil, fmt.Errorf("new config for store %q: %w", storeModule.Name, err)
		}
		out[storeModule.Name] = c
	}
	return out, nil
}
func (s *Stores) resetStores() {
	for _, s := range s.StoreMap.All() {
		if resetableStore, ok := s.(store2.Resettable); ok {
			resetableStore.Reset()
		}
	}
}

func (s *Stores) flushStores(ctx context.Context, blockNum uint64) (err error) {
	logger := reqctx.Logger(ctx)
	reqStats := reqctx.ReqStats(ctx)
	boundaryIntervals := s.bounder.GetStoreFlushRanges(s.isSubRequest, s.bounder.requestStopBlock, blockNum)
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
func (s *Stores) storesHandleUndo(moduleOutput *pbsubstreams.ModuleOutput) {
	if s, found := s.StoreMap.Get(moduleOutput.Name); found {
		if deltaStore, ok := s.(store2.DeltaAccessor); ok {
			deltaStore.ApplyDeltasReverse(moduleOutput.GetDebugStoreDeltas().GetDeltas())
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

func (s *Stores) saveStoreSnapshot(ctx context.Context, saveStore store2.Store, boundaryBlock uint64) (err error) {
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

		if v, ok := saveStore.(store2.PartialStore); ok {
			reqctx.Span(ctx).AddEvent("store_roll_trigger")
			v.Roll(boundaryBlock)
		}
	}
	return nil
}

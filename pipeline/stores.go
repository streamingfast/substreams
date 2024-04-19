package pipeline

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel/attribute"
	"go.uber.org/zap"

	"github.com/streamingfast/substreams/block"
	pbssinternal "github.com/streamingfast/substreams/pb/sf/substreams/intern/v2"
	"github.com/streamingfast/substreams/pipeline/outputmodules"
	"github.com/streamingfast/substreams/reqctx"
	"github.com/streamingfast/substreams/storage/store"
)

type Stores struct {
	logger         *zap.Logger
	isTier2Request bool // means we're processing a tier2 request
	bounder        *storeBoundary
	configs        store.ConfigMap
	StoreMap       store.Map
	// DEPRECATED: we don't need to report back, these file names are now implicitly conveyed from
	// tier1 to tier2.
	partialsWritten block.Ranges // when backprocessing, to report back to orchestrator
	tier            string
}

func NewStores(ctx context.Context, storeConfigs store.ConfigMap, storeSnapshotSaveInterval, requestStartBlockNum, stopBlockNum uint64, isTier2Request bool) *Stores {
	// FIXME(abourget): a StoreBoundary should exist for EACH Store
	//  because the module's Initial Block could change the range of each
	//  store.
	tier := "tier1"
	if isTier2Request {
		tier = "tier2"
	}
	bounder := NewStoreBoundary(storeSnapshotSaveInterval, requestStartBlockNum, stopBlockNum)
	return &Stores{
		configs:        storeConfigs,
		isTier2Request: isTier2Request,
		bounder:        bounder,
		tier:           tier,
		logger:         reqctx.Logger(ctx),
	}
}

func (s *Stores) SetStoreMap(storeMap store.Map) {
	s.StoreMap = storeMap
}

func (s *Stores) resetStores() {
	for _, s := range s.StoreMap.All() {
		if resetableStore, ok := s.(store.Resettable); ok {
			resetableStore.Reset()
		}
	}
}

// flushStores is called only for Tier2 request, as to not save reversible stores.
func (s *Stores) flushStores(ctx context.Context, executionStages outputmodules.ExecutionStages, blockNum uint64) (err error) {
	if s.StoreMap == nil {
		return // fast exit for cases without stores or no linear processing
	}
	lastLayer := executionStages.LastStage().LastLayer()
	if !lastLayer.IsStoreLayer() {
		return nil
	}

	boundaryIntervals := s.bounder.GetStoreFlushRanges(s.isTier2Request, s.bounder.requestStopBlock, blockNum)
	for _, boundaryBlock := range boundaryIntervals {
		if err := s.saveStoresSnapshots(ctx, lastLayer, len(executionStages)-1, boundaryBlock); err != nil {
			return fmt.Errorf("saving stores snapshot at bound %d: %w", boundaryBlock, err)
		}
	}
	return nil
}

func (s *Stores) saveStoresSnapshots(ctx context.Context, lastLayer outputmodules.LayerModules, stage int, boundaryBlock uint64) (err error) {
	for _, mod := range lastLayer {
		store := s.StoreMap[mod.Name]
		s.logger.Info("flushing store at boundary", zap.Uint64("boundary", boundaryBlock), zap.String("store", mod.Name), zap.Int("stage", stage))
		// TODO when partials are generic again, we can also check if PartialKV exists and skip if it does.
		exists, _ := s.configs[mod.Name].ExistsFullKV(ctx, boundaryBlock)
		if exists {
			continue
		}
		if err := s.saveStoreSnapshot(ctx, store, boundaryBlock); err != nil {
			return fmt.Errorf("save store snapshot %q: %w", mod.Name, err)
		}
	}
	return nil
}

func (s *Stores) storesHandleUndo(moduleOutput *pbssinternal.ModuleOutput) {
	if s, found := s.StoreMap.Get(moduleOutput.ModuleName); found {
		if deltaStore, ok := s.(store.DeltaAccessor); ok {
			deltaStore.ApplyDeltasReverse(moduleOutput.GetStoreDeltas().StoreDeltas)
		}
	}
}

func (s *Stores) saveStoreSnapshot(ctx context.Context, saveStore store.Store, boundaryBlock uint64) (err error) {
	ctx, span := reqctx.WithSpan(ctx, fmt.Sprintf("substreams/%s/stores/save_store_snapshot", s.tier))
	span.SetAttributes(attribute.String("subtreams.store", saveStore.Name()))
	defer span.EndWithErr(&err)

	file, writer, err := saveStore.Save(boundaryBlock)
	if err != nil {
		return fmt.Errorf("saving store %q at boundary %d: %w", saveStore.Name(), boundaryBlock, err)
	}

	if err = writer.Write(ctx); err != nil {
		return fmt.Errorf("failed to write store: %w", err)
	}

	if reqctx.Details(ctx).ShouldReturnWrittenPartials(saveStore.Name()) {
		//s.partialsWritten = append(s.partialsWritten, file.Range)
		s.logger.Debug("adding partials written",
			zap.Stringer("range", file.Range),
			zap.Stringer("ranges", s.partialsWritten),
			zap.Uint64("boundary_block", boundaryBlock),
		)

		if v, ok := saveStore.(store.PartialStore); ok {
			reqctx.Span(ctx).AddEvent("store_roll_trigger")
			v.Roll(boundaryBlock)
		}
	}
	return nil
}

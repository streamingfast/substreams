package pipeline

import (
	"fmt"
	"github.com/streamingfast/dstore"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/store"
	"go.uber.org/zap"
)

type StoreFactory struct {
	baseStore    dstore.Store
	saveInterval uint64
}

func NewStoreFactory(baseStore dstore.Store, saveInterval uint64) *StoreFactory {
	return &StoreFactory{
		baseStore:    baseStore,
		saveInterval: saveInterval,
	}
}

func (g *StoreFactory) NewFullKV(hash string, storeModule *pbsubstreams.Module, logger *zap.Logger) (*store.FullKV, error) {
	return store.NewFullKV(
		storeModule.Name,
		storeModule.InitialBlock,
		hash,
		storeModule.GetKindStore().UpdatePolicy,
		storeModule.GetKindStore().ValueType,
		g.baseStore,
		logger,
	)
}

func (g *StoreFactory) NewPartialKV(hash string, storeModule *pbsubstreams.Module, initialBlock uint64, logger *zap.Logger) (*store.PartialKV, error) {
	s, err := store.NewBaseStore(
		storeModule.Name,
		storeModule.InitialBlock,
		hash,
		storeModule.GetKindStore().UpdatePolicy,
		storeModule.GetKindStore().ValueType,
		g.baseStore,
		logger,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create store: %w", err)
	}
	return store.NewPartialKV(s, initialBlock), nil
}

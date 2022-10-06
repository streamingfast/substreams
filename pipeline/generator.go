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

func (g *StoreFactory) NewKVStore(hash string, storeModule *pbsubstreams.Module, logger *zap.Logger) (*store.KVStore, error) {
	return store.NewKVStore(
		storeModule.Name,
		storeModule.InitialBlock,
		hash,
		storeModule.GetKindStore().UpdatePolicy,
		storeModule.GetKindStore().ValueType,
		g.baseStore,
		logger,
	)
}

func (g *StoreFactory) NewKVPartialStore(hash string, storeModule *pbsubstreams.Module, initialBlock uint64, logger *zap.Logger) (*store.KVPartialStore, error) {
	s, err := store.NewKVStore(
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
	return store.NewPartialStore(s, initialBlock), nil
}

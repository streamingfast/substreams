package store

import (
	"fmt"

	"github.com/streamingfast/dstore"
	"github.com/streamingfast/substreams/manifest"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

type ConfigMap map[string]*Config

func NewConfigMap(baseObjectStore dstore.Store, storeModules []*pbsubstreams.Module, moduleHashes *manifest.ModuleHashes) (out ConfigMap, err error) {
	out = make(ConfigMap)
	for _, storeModule := range storeModules {
		c, err := NewConfig(
			storeModule.Name,
			storeModule.InitialBlock,
			moduleHashes.Get(storeModule.Name),
			storeModule.GetKindStore().UpdatePolicy,
			storeModule.GetKindStore().ValueType,
			baseObjectStore,
		)
		if err != nil {
			return nil, fmt.Errorf("new store config for %q: %w", storeModule.Name, err)
		}
		out[storeModule.Name] = c
	}
	return out, nil
}

package store

import (
	"fmt"

	"github.com/streamingfast/dstore"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

type ConfigMap map[string]*Config

// NewConfigMap creates a ConfigMap for the given store modules.
// storeSizeLimit, if non-zero, overrides the default StoreSizeLimit for all stores in this map.
func NewConfigMap(baseObjectStore, quickSaveStore dstore.Store, storeModules []*pbsubstreams.Module, moduleHashes map[string]string, firstStreamableBlock uint64, storeSizeLimit uint64, scratchSpace string, backend string) (out ConfigMap, err error) {
	out = make(ConfigMap)
	for _, storeModule := range storeModules {
		// Store files are named after the module's own initial block, even when it is below the
		// first streamable block (pruned chain), so snapshots taken before the chain was
		// pruned are still found. A module without one starts at the first streamable block.
		initialBlock := storeModule.InitialBlock
		if initialBlock == 0 {
			initialBlock = firstStreamableBlock
		}
		c, err := NewConfig(
			storeModule.Name,
			initialBlock,
			moduleHashes[storeModule.Name],
			storeModule.GetKindStore().UpdatePolicy,
			storeModule.GetKindStore().ValueType,
			baseObjectStore,
			quickSaveStore,
			storeSizeLimit,
			scratchSpace,
			backend,
		)
		if err != nil {
			return nil, fmt.Errorf("new store config for %q: %w", storeModule.Name, err)
		}
		out[storeModule.Name] = c
	}
	return out, nil
}

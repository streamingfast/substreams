package codegen

import (
	"log"
	"sync"

	registry "github.com/pinax-network/graph-networks-libs/packages/golang/lib"
)

var (
	registryNetworks     map[string]*registry.Network
	registryNetworksOnce sync.Once
)

// GetRegistryNetworks returns a map of network ID to *registry.Network, using Pinax's registry as primary source.
// Only networks with a Firehose endpoint are included.
// If fetching fails, it falls back to loading from a local JSON file ("TheGraphNetworksRegistry.json").
func GetRegistryNetworks() map[string]*registry.Network {
	registryNetworksOnce.Do(func() {
		reg, err := registry.FromLatestVersion()
		if err != nil {
			// Fallback: try to load from local file
			// TODO: Validate where to put the registry file
			reg, err = registry.FromFile("TheGraphNetworksRegistry.json")
			if err != nil {
				log.Fatalf("Failed to load registry from both network and file: %v", err)
			}
		}
		m := make(map[string]*registry.Network)
		for i, net := range reg.Networks {
			if len(net.Services.Firehose) > 0 {
				m[net.ID] = &reg.Networks[i]
			}
		}
		registryNetworks = m
	})
	return registryNetworks
}

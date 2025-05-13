package networks

import (
	_ "embed"
	"fmt"
	"slices"
	"sort"
	"sync"

	registry "github.com/pinax-network/graph-networks-libs/packages/golang/lib"
)

//go:embed TheGraphNetworksRegistry.json
var embeddedRegistryJSON []byte

// Package networks provides access to The Graph's network registry and helpers.

// NetworkRegistry is a thin wrapper around a [map[string]*registry.Network] to add some helper methods.
type NetworkRegistry map[string]*registry.Network

var (
	registryNetworks     NetworkRegistry
	registryNetworksOnce sync.Once
)

// GetRegistryNetworks returns a map of network ID to *registry.Network, using The Graph's registry as primary source.
// If fetching fails, it falls back to loading from a local JSON file ("TheGraphNetworksRegistry.json").
func GetRegistryNetworks() NetworkRegistry {
	registryNetworksOnce.Do(func() {
		reg, err := registry.FromLatestVersion()
		if err != nil {
			// Fallback: use embedded JSON
			reg, err = registry.FromJSON(embeddedRegistryJSON)
			if err != nil {
				panic(fmt.Sprintf("Failed to load registry from both network and embedded JSON: %v", err))
			}
		}
		m := make(map[string]*registry.Network)
		for i, net := range reg.Networks {
			if len(net.Services.Substreams) > 0 {
				m[net.ID] = &reg.Networks[i]
			}
		}
		registryNetworks = m

		// Add custom Tron network until it's added to the registry
		registryNetworks.addCustomNetwork(&registry.Network{
			ID:        "tron",
			ShortName: "Tron",
			FullName:  "Tron Mainnet",
			Aliases:   []string{"tron-mainnet"},
			Caip2ID:   "eip155:728126428",
			GraphNode: &registry.GraphNode{
				Protocol: (*registry.Protocol)(ptr("tron")),
			},
			ExplorerUrls: []string{"https://tronscan.org"},
			RPCUrls:      []string{"https://api.trongrid.io/jsonrpc", "https://tron.drpc.org", "https://rpc.ankr.com/tron_jsonrpc"},
			APIUrls: []registry.APIURL{
				{
					URL:  "https://apilist.tronscanapi.com/api/",
					Kind: "etherscan",
				},
			},
			Services: registry.Services{
				Subgraphs:  []string{"mainnet.tron.streamingfast.io:443"},
				Substreams: []string{"mainnet.tron.streamingfast.io:443"},
			},
			NetworkType:     "mainnet",
			IssuanceRewards: true,
			NativeToken:     ptr("TRX"),
			DocsURL:         ptr("https://developers.tron.network/"),
			Genesis: &registry.Genesis{
				Hash:   "0x00000000000000001ebf88508a03865c71d452e25f4d51194196a1d22b6653dc",
				Height: 0,
			},
			Firehose: &registry.Firehose{
				BlockType:        "sf.tron.type.v1.Block",
				EvmExtendedModel: ptr(false),
				BufURL:           "https://buf.build/streamingfast/firehose-tron",
				BytesEncoding:    "hex",
			},
		})
	})
	return registryNetworks
}

// addCustomNetwork can be used to add a custom network to the registry map for testing or development.
func (r NetworkRegistry) addCustomNetwork(network *registry.Network) {
	if network == nil || network.ID == "" {
		return // Ignore invalid input
	}
	r[network.ID] = network
}

// Find returns the network by ID or, if not found, by alias (sorted by network ID).
func (r NetworkRegistry) Find(key string) *registry.Network {
	if n, ok := r[key]; ok {
		return n
	}
	// If not found by ID, search aliases in sorted order
	ids := make([]string, 0, len(r))
	for id := range r {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		net := r[id]
		if slices.Contains(net.Aliases, key) {
			return net
		}
	}
	return nil
}

// Find is a shortcut for GetRegistry().Find(key).
func Find(key string) *registry.Network {
	return GetRegistryNetworks().Find(key)
}

func ptr[T any](v T) *T { return &v }

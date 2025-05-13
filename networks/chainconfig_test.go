package networks

import (
	"testing"

	registry "github.com/pinax-network/graph-networks-libs/packages/golang/lib"
	"github.com/stretchr/testify/assert"
)

func TestNetworkRegistry_Find(t *testing.T) {
	net1 := &registry.Network{ID: "mainnet", Aliases: []string{"eth", "ethereum"}}
	net2 := &registry.Network{ID: "arbitrum", Aliases: []string{"arb", "arbitrum-one"}}
	net3 := &registry.Network{ID: "custom", Aliases: []string{"mychain"}}

	r := NetworkRegistry{
		"mainnet":  net1,
		"arbitrum": net2,
		"custom":   net3,
	}

	t.Run("find by id", func(t *testing.T) {
		assert.Equal(t, net1, r.Find("mainnet"))
		assert.Equal(t, net2, r.Find("arbitrum"))
	})
	t.Run("find by alias", func(t *testing.T) {
		assert.Equal(t, net1, r.Find("eth"))
		assert.Equal(t, net1, r.Find("ethereum"))
		assert.Equal(t, net2, r.Find("arb"))
		assert.Equal(t, net2, r.Find("arbitrum-one"))
		assert.Equal(t, net3, r.Find("mychain"))
	})
	t.Run("not found", func(t *testing.T) {
		assert.Nil(t, r.Find("notfound"))
	})
}

func TestAllLegacyChainConfigKeysPresent(t *testing.T) {
	legacyKeys := []string{
		"mainnet", "bnb", "polygon", "amoy", "arbitrum", "holesky", "sepolia", "optimism", "avalanche", "chapel",
		"injective-mainnet", "injective-testnet", "starknet-mainnet", "starknet-testnet", "solana-mainnet-beta",
		"mantra-testnet", "mantra-mainnet", "stellar-testnet", "stellar", "sei-mainnet",
	}

	networks := GetRegistryNetworks()
	for _, key := range legacyKeys {
		net := networks.Find(key)
		assert.NotNilf(t, net, "Network with key %q should be present in GetRegistryNetworks()", key)
	}
}

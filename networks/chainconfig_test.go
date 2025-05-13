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

	for _, key := range legacyKeys {
		net := Find(key)
		assert.NotNilf(t, net, "Network with key %q should be present in GetRegistryNetworksWithSubstreams()", key)
	}
}

func TestGetRegistryNetworksWithSubstreams(t *testing.T) {
	networks := GetRegistryNetworksWithSubstreams()
	assert.NotEmpty(t, networks, "Should return at least one network with Substreams endpoint")
	for id, net := range networks {
		assert.Greater(t, len(net.Services.Substreams), 0, "Network %q should have at least one Substreams endpoint", id)
	}
	// Known networks with Substreams endpoints (should be present)
	for _, key := range []string{"mainnet", "optimism", "arbitrum", "polygon", "bnb", "avalanche"} {
		assert.NotNilf(t, networks.Find(key), "Network %q should be present in Substreams registry", key)
	}
	// Known networks without Substreams endpoints (should NOT be present)
	for _, key := range []string{"cronos", "clover", "aurora", "celo"} {
		assert.Nilf(t, networks.Find(key), "Network %q should NOT be present in Substreams registry", key)
	}
}

func TestGetRegistryNetworksWithFirehose(t *testing.T) {
	networks := GetRegistryNetworksWithFirehose()
	assert.NotEmpty(t, networks, "Should return at least one network with Firehose endpoint")
	for id, net := range networks {
		assert.Greater(t, len(net.Services.Firehose), 0, "Network %q should have at least one Firehose endpoint", id)
	}
	// Known networks with Firehose endpoints (should be present)
	for _, key := range []string{"mainnet", "optimism", "arbitrum", "polygon", "bnb", "avalanche"} {
		assert.NotNilf(t, networks.Find(key), "Network %q should be present in Firehose registry", key)
	}
	// Known networks without Firehose endpoints (should NOT be present)
	for _, key := range []string{"cronos", "clover", "aurora", "celo"} {
		assert.Nilf(t, networks.Find(key), "Network %q should NOT be present in Firehose registry", key)
	}
}

package networks

import (
	"testing"

	registry "github.com/pinax-network/graph-networks-libs/packages/golang/lib"
	"github.com/stretchr/testify/assert"
)

func TestNetworkRegistry_Find(t *testing.T) {
	net1 := &registry.Network{ID: "mainnet", ShortName: "ETH", FullName: "Ethereum Mainnet", Aliases: []string{"eth", "ethereum"}}
	net2 := &registry.Network{ID: "arbitrum", ShortName: "ARB", FullName: "Arbitrum One", Aliases: []string{"arb", "arbitrum-one"}}
	net3 := &registry.Network{ID: "custom", ShortName: "MYC", FullName: "My Custom Chain", Aliases: []string{"mychain"}}

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
	t.Run("find by FullName", func(t *testing.T) {
		assert.Equal(t, net1, r.Find("Ethereum Mainnet"))
		assert.Equal(t, net2, r.Find("Arbitrum One"))
		assert.Equal(t, net3, r.Find("My Custom Chain"))
	})
	t.Run("find by ShortName", func(t *testing.T) {
		assert.Equal(t, net1, r.Find("ETH"))
		assert.Equal(t, net2, r.Find("ARB"))
		assert.Equal(t, net3, r.Find("MYC"))
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
		assert.NotNilf(t, net, "Network with key %q should be present in GetSubstreamsRegistry()", key)
	}
}

func TestGetSubstreamsRegistry(t *testing.T) {
	networks := GetSubstreamsRegistry()
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

func TestGetFirehoseRegistry(t *testing.T) {
	networks := GetFirehoseRegistry()
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

func TestNetworkRegistry_FindByGenesisBlock(t *testing.T) {
	networks := getRegistryNetworks()
	const moonbeamID = "moonbeam"
	const moonbeamGenesisHash = "0x7e6b3bbed86828a558271c9c9f62354b1d8b5aa15ff85fd6f1e7cbe9af9dde7e"
	const moonbeamGenesisHeight = 0

	net := networks.FindByGenesisBlock(moonbeamGenesisHeight, moonbeamGenesisHash)
	assert.NotNil(t, net, "Should find Moonbeam by genesis block")
	assert.Equal(t, moonbeamID, net.ID)

	// Not found case
	notFound := networks.FindByGenesisBlock(12345, "0xdeadbeef")
	assert.Nil(t, notFound, "Should return nil for unknown genesis block")
}

func TestGetBytesEncoding(t *testing.T) {
	networks := getRegistryNetworks()

	t.Run("returns correct encoding for mainnet", func(t *testing.T) {
		net := networks.Find("mainnet")
		assert.NotNil(t, net)
		assert.Equal(t, registry.Hex, GetBytesEncoding(net))
	})

	t.Run("returns correct encoding for optimism", func(t *testing.T) {
		net := networks.Find("optimism")
		assert.NotNil(t, net)
		assert.Equal(t, registry.Hex, GetBytesEncoding(net))
	})

	t.Run("returns Hex for nil network", func(t *testing.T) {
		assert.Equal(t, registry.Hex, GetBytesEncoding(nil))
	})

	t.Run("returns Hex for network without Firehose", func(t *testing.T) {
		net := &registry.Network{ID: "no-firehose"}
		assert.Equal(t, registry.Hex, GetBytesEncoding(net))
	})
}

func TestFindBySubstreamsEndpoint(t *testing.T) {
	substreamsRegistry := GetSubstreamsRegistry()

	t.Run("finds mainnet by endpoint", func(t *testing.T) {
		mainnetEndpoints := []string{
			"eth.substreams.pinax.network:443",
			"mainnet.eth.streamingfast.io:443",
		}
		for _, ep := range mainnetEndpoints {
			net := substreamsRegistry.FindBySubstreamsEndpoint(ep)
			assert.NotNilf(t, net, "Should find mainnet for endpoint %q", ep)
			assert.Equal(t, "mainnet", net.ID)
		}
	})

	t.Run("finds optimism by endpoint", func(t *testing.T) {
		optimismEndpoints := []string{
			"mainnet.optimism.streamingfast.io:443",
			"optimism.substreams.pinax.network:443",
		}
		for _, ep := range optimismEndpoints {
			net := substreamsRegistry.FindBySubstreamsEndpoint(ep)
			assert.NotNilf(t, net, "Should find optimism for endpoint %q", ep)
			assert.Equal(t, "optimism", net.ID)
		}
	})

	t.Run("returns nil for unknown endpoint", func(t *testing.T) {
		net := substreamsRegistry.FindBySubstreamsEndpoint("unknown.endpoint:1234")
		assert.Nil(t, net)
	})

	t.Run("returns nil for empty endpoint", func(t *testing.T) {
		net := substreamsRegistry.FindBySubstreamsEndpoint("")
		assert.Nil(t, net)
	})

	t.Run("returns nil for network with no Substreams endpoints", func(t *testing.T) {
		// Add a network with no Substreams endpoints
		net := &registry.Network{ID: "no-substreams", Services: registry.Services{Substreams: []string{}}}
		r := NetworkRegistry{"no-substreams": net}
		assert.Nil(t, r.FindBySubstreamsEndpoint("any.endpoint:443"))
	})
}

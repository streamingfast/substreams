package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	pbconvo "github.com/streamingfast/substreams/pb/sf/codegen/conversation/v1"
)

func TestProtocolOrderPreservation(t *testing.T) {
	// Simulate generators received from server in specific order
	generators := []*pbconvo.DiscoveryResponse_Generator{
		{Id: "evm-events-calls", Group: "EVM", Title: "EVM Events/Calls"},
		{Id: "evm-hello-world", Group: "EVM", Title: "EVM Hello World"},
		{Id: "sol-anchor", Group: "Solana", Title: "Solana Anchor"},
		{Id: "sol-hello-world", Group: "Solana", Title: "Solana Hello World"},
		{Id: "starknet-events", Group: "Starknet", Title: "Starknet Events"},
	}

	// Reproduce the grouping logic
	type BlockchainProtocolSelector struct {
		Id         string
		Title      string
		Generators []*pbconvo.DiscoveryResponse_Generator
	}

	var protocolOrder []string
	filteredProtocols := make(map[string]*BlockchainProtocolSelector)
	for _, gen := range generators {
		selector, groupExists := filteredProtocols[gen.Group]
		if groupExists {
			selector.Generators = append(selector.Generators, gen)
		} else {
			protocolOrder = append(protocolOrder, gen.Group)
			filteredProtocols[gen.Group] = &BlockchainProtocolSelector{
				Id:         gen.Group,
				Title:      gen.Group,
				Generators: []*pbconvo.DiscoveryResponse_Generator{gen},
			}
		}
	}

	// Verify order is preserved
	require.Len(t, protocolOrder, 3)
	assert.Equal(t, "EVM", protocolOrder[0])
	assert.Equal(t, "Solana", protocolOrder[1])
	assert.Equal(t, "Starknet", protocolOrder[2])

	// Verify generators within each group maintain order
	evmGens := filteredProtocols["EVM"].Generators
	require.Len(t, evmGens, 2)
	assert.Equal(t, "evm-events-calls", evmGens[0].Id)
	assert.Equal(t, "evm-hello-world", evmGens[1].Id)

	solanaGens := filteredProtocols["Solana"].Generators
	require.Len(t, solanaGens, 2)
	assert.Equal(t, "sol-anchor", solanaGens[0].Id)
	assert.Equal(t, "sol-hello-world", solanaGens[1].Id)
}

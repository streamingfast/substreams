package main

import (
	"testing"

	"github.com/streamingfast/substreams/manifest"
	"github.com/stretchr/testify/assert"
)

func TestRenderNetworks(t *testing.T) {
	networks := map[string]*manifest.NetworkParams{
		"sepolia": {
			InitialBlocks: map[string]uint64{"mod2": 400, "mod1": 400},
			Params:        map[string]string{"mod1": "val=tata"},
		},
		"mainnet": {
			InitialBlocks: map[string]uint64{"mod2": 200, "mod1": 200},
			Params:        map[string]string{"mod1": "val=toto"},
		},
	}

	t.Run("collapsed", func(t *testing.T) {
		expected := `Networks:
  mainnet (default): 2 initial blocks, 1 param
  sepolia: 2 initial blocks, 1 param

  Use --expand-networks to see the per-module values.
`

		assert.Equal(t, expected, renderNetworks("mainnet", networks, false))
	})

	t.Run("expanded", func(t *testing.T) {
		expected := `Networks:
  mainnet (default):
    Initial Blocks:
      - mod1: 200
      - mod2: 200
    Params:
      - mod1: "val=toto"

  sepolia:
    Initial Blocks:
      - mod1: 400
      - mod2: 400
    Params:
      - mod1: "val=tata"

`

		assert.Equal(t, expected, renderNetworks("mainnet", networks, true))
	})
}

func TestRenderNetworks_omitsEmptySections(t *testing.T) {
	networks := map[string]*manifest.NetworkParams{
		"mainnet": {InitialBlocks: map[string]uint64{"mod1": 200}},
	}

	expected := `Networks:
  mainnet (default):
    Initial Blocks:
      - mod1: 200

`

	assert.Equal(t, expected, renderNetworks("mainnet", networks, true))
}

func TestRenderNetworks_pluralizesCounts(t *testing.T) {
	networks := map[string]*manifest.NetworkParams{
		"mainnet": {
			InitialBlocks: map[string]uint64{"mod1": 200},
			Params:        map[string]string{"mod1": "a", "mod2": "b"},
		},
	}

	expected := `Networks:
  mainnet (default): 1 initial block, 2 params

  Use --expand-networks to see the per-module values.
`

	assert.Equal(t, expected, renderNetworks("mainnet", networks, false))
}

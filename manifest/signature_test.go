package manifest

import (
	"testing"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/stretchr/testify/require"
)

func Test_HashModule(t *testing.T) {
	mapPoolsCreatedModule := &pbsubstreams.Module{
		Name:         "map_pools_created",
		InitialBlock: 12369621,
		Kind: &pbsubstreams.Module_KindMap_{
			KindMap: &pbsubstreams.Module_KindMap{
				OutputType: "proto:uniswap.types.v1.Pools",
			},
		},
		Inputs: []*pbsubstreams.Module_Input{
			{
				Input: &pbsubstreams.Module_Input_Params_{
					Params: &pbsubstreams.Module_Input_Params{
						Value: "foo",
					},
				},
			},
			{
				Input: &pbsubstreams.Module_Input_Source_{
					Source: &pbsubstreams.Module_Input_Source{
						Type: "sf.ethereum.type.v1.Block",
					},
				},
			},
		},
	}
	mapPoolsInitializationModule := &pbsubstreams.Module{
		Name:         "map_pools_initialized",
		InitialBlock: 12369621,
		Kind: &pbsubstreams.Module_KindMap_{
			KindMap: &pbsubstreams.Module_KindMap{
				OutputType: "proto:uniswap.types.v1.Pools",
			},
		},
		Inputs: []*pbsubstreams.Module_Input{
			{
				Input: &pbsubstreams.Module_Input_Source_{
					Source: &pbsubstreams.Module_Input_Source{
						Type: "sf.ethereum.type.v1.Block",
					},
				},
			},
		},
	}
	manifest := &pbsubstreams.Modules{
		Modules: []*pbsubstreams.Module{
			mapPoolsCreatedModule, mapPoolsInitializationModule,
		},
		Binaries: []*pbsubstreams.Binary{
			{
				Type:    "wasm/rust-v1",
				Content: []byte("01"),
			},
		},
	}

	graph, _ := NewModuleGraph(manifest.Modules)
	hashes := NewModuleHashes()
	hashMapPoolsCreated := hashes.HashModule(manifest, mapPoolsCreatedModule, graph)
	hashMapPoolsInitialized := hashes.HashModule(manifest, mapPoolsInitializationModule, graph)

	require.NotEqual(t, hashMapPoolsInitialized, hashMapPoolsCreated)
}

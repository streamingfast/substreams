package outputmodules

import (
	"testing"

	pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/stretchr/testify/assert"
)

func TestGraph_computeSchedulableModules(t *testing.T) {
	tests := []struct {
		name           string
		stores         []*pbsubstreams.Module
		outputModule   *pbsubstreams.Module
		productionMode bool
		expect         []*pbsubstreams.Module
	}{

		{
			name:         "dev mode with output module map",
			stores:       []*pbsubstreams.Module{pbsubstreamsrpc.TestNewStoreModule("store_a"), pbsubstreamsrpc.TestNewStoreModule("store_b")},
			outputModule: pbsubstreamsrpc.TestNewMapModule("map_a"),
			expect:       []*pbsubstreams.Module{pbsubstreamsrpc.TestNewStoreModule("store_a"), pbsubstreamsrpc.TestNewStoreModule("store_b")},
		},
		{
			name:         "dev mode with output module store",
			stores:       []*pbsubstreams.Module{pbsubstreamsrpc.TestNewStoreModule("store_a"), pbsubstreamsrpc.TestNewStoreModule("store_b")},
			outputModule: pbsubstreamsrpc.TestNewStoreModule("store_b"),
			expect:       []*pbsubstreams.Module{pbsubstreamsrpc.TestNewStoreModule("store_a"), pbsubstreamsrpc.TestNewStoreModule("store_b")},
		},
		{
			name:           "prod mode with output module map",
			stores:         []*pbsubstreams.Module{pbsubstreamsrpc.TestNewStoreModule("store_a"), pbsubstreamsrpc.TestNewStoreModule("store_b")},
			outputModule:   pbsubstreamsrpc.TestNewMapModule("map_a"),
			productionMode: true,
			expect:         []*pbsubstreams.Module{pbsubstreamsrpc.TestNewStoreModule("store_a"), pbsubstreamsrpc.TestNewStoreModule("store_b"), pbsubstreamsrpc.TestNewMapModule("map_a")},
		},
		{
			name:           "prod mode with output module store",
			stores:         []*pbsubstreams.Module{pbsubstreamsrpc.TestNewStoreModule("store_a"), pbsubstreamsrpc.TestNewStoreModule("store_b")},
			outputModule:   pbsubstreamsrpc.TestNewStoreModule("store_b"),
			productionMode: true,
			expect:         []*pbsubstreams.Module{pbsubstreamsrpc.TestNewStoreModule("store_a"), pbsubstreamsrpc.TestNewStoreModule("store_b")},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			out := computeSchedulableModules(test.stores, test.outputModule, test.productionMode)

			assert.Equal(t, test.expect, out)
		})
	}

}

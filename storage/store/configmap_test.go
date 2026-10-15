package store

import (
	"testing"

	"github.com/streamingfast/dstore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

func TestNewConfigMap_InitialBlock(t *testing.T) {
	storeModule := func(name string, initialBlock uint64) *pbsubstreams.Module {
		return &pbsubstreams.Module{
			Name:         name,
			InitialBlock: initialBlock,
			Kind:         &pbsubstreams.Module_KindStore_{KindStore: &pbsubstreams.Module_KindStore{UpdatePolicy: pbsubstreams.Module_KindStore_UPDATE_POLICY_SET, ValueType: "string"}},
		}
	}

	configs, err := NewConfigMap(dstore.NewMockStore(nil), nil, []*pbsubstreams.Module{
		storeModule("no_initial_block", 0),
		storeModule("before_first_streamable_block", 346),
		storeModule("after_first_streamable_block", 450),
	}, map[string]string{
		"no_initial_block":              "hash1",
		"before_first_streamable_block": "hash2",
		"after_first_streamable_block":  "hash3",
	}, 400, 0, "", "")
	require.NoError(t, err)

	assert.Equal(t, uint64(400), configs["no_initial_block"].ModuleInitialBlock())
	assert.Equal(t, uint64(346), configs["before_first_streamable_block"].ModuleInitialBlock())
	assert.Equal(t, uint64(450), configs["after_first_streamable_block"].ModuleInitialBlock())
}

package store

import (
	"github.com/streamingfast/substreams/storage/store/marshaller"
	"go.uber.org/zap"

	"github.com/streamingfast/dstore"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/stretchr/testify/require"
)

func newTestBaseStore(
	t require.TestingT,
	updatePolicy pbsubstreams.Module_KindStore_UpdatePolicy,
	valueType string,
	store dstore.Store,
) *baseStore {
	if store == nil {
		store = dstore.NewMockStore(nil)
	}

	var appendLimit uint64 = 8_388_608 // 8kb = 8 * 1024 * 1024,
	if updatePolicy == pbsubstreams.Module_KindStore_UPDATE_POLICY_APPEND {
		appendLimit = 10
	}

	config, err := NewConfig("test", 0, "test.module.hash", updatePolicy, valueType, store)
	config.appendLimit = appendLimit
	config.totalSizeLimit = 9999
	config.itemSizeLimit = 10_485_760
	require.NoError(t, err)
	return &baseStore{
		Config:     config,
		kv:         make(map[string][]byte),
		logger:     zap.NewNop(),
		marshaller: &marshaller.Binary{},
	}
}

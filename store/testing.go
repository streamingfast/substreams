package store

import (
	"github.com/streamingfast/dstore"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/stretchr/testify/require"
	"testing"
)

func NewTestKVStore(
	t *testing.T,
	updatePolicy pbsubstreams.Module_KindStore_UpdatePolicy,
	valueType string,
	store dstore.Store,
) *KVStore {
	t.Helper()
	if store == nil {
		store = dstore.NewMockStore(nil)
	}

	stateStore, err := NewKVStore("test", 0, "test.module.hash", updatePolicy, valueType, store, zlog)
	require.NoError(t, err)

	return stateStore
}

func NewTestKVPartialStore(
	t *testing.T,
	updatePolicy pbsubstreams.Module_KindStore_UpdatePolicy,
	valueType string,
	store dstore.Store,
	initialPartialBlock uint64,
) *KVPartialStore {
	s := NewTestKVStore(t, updatePolicy, valueType, store)
	return NewPartialStore(s, initialPartialBlock)
}

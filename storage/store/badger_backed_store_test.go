package store

import (
	"math/big"
	"net"
	"testing"

	fsgrpc "github.com/streamingfast/substreams-foundational-store/grpc"
	pbmodel "github.com/streamingfast/substreams-foundational-store/pb/sf/substreams/foundational-store/model/v2"
	pbfoundationalservice "github.com/streamingfast/substreams-foundational-store/pb/sf/substreams/foundational-store/service/v2"
	fsstore "github.com/streamingfast/substreams-foundational-store/store"
	fsForkAware "github.com/streamingfast/substreams-foundational-store/store/ForkAware"
	fsBadger "github.com/streamingfast/substreams-foundational-store/store/badger"
	pbssinternal "github.com/streamingfast/substreams/pb/sf/substreams/intern/v2"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"google.golang.org/grpc"
)

// startTestFoundationalStore starts an in-process foundational store backed by a real
// Badger instance (on-disk in tmpDir) and returns the gRPC address and backend for inspection.
func startTestFoundationalStore(t *testing.T, tmpDir string) (addr string, backend *fsBadger.Store) {
	t.Helper()
	dsn, err := fsstore.ParseDSN("badger://" + tmpDir + "/fstore")
	require.NoError(t, err)
	backend, err = fsBadger.NewStore(dsn, "type.googleapis.com/google.protobuf.BytesValue", 1, zap.NewNop())
	require.NoError(t, err)
	forkAware := fsForkAware.NewStore(backend)
	lis, err := net.Listen("tcp", "localhost:0")
	require.NoError(t, err)
	srv := grpc.NewServer()
	pbfoundationalservice.RegisterStoreServer(srv, fsgrpc.NewStoreServer(forkAware, func() uint64 { return ^uint64(0) }, zap.NewNop()))
	go srv.Serve(lis) //nolint:errcheck
	t.Cleanup(func() { srv.Stop() })
	return lis.Addr().String(), backend
}

// newTestBadgerStore creates a BadgerBackedStore wired to the given gRPC addr.
func newTestBadgerStore(t *testing.T, addr string, policy pbsubstreams.Module_KindStore_UpdatePolicy, valueType string) *BadgerBackedStore {
	t.Helper()
	conn, err := grpc.NewClient(addr, grpc.WithInsecure()) //nolint:staticcheck
	require.NoError(t, err)
	client := pbfoundationalservice.NewStoreClient(conn)
	s, err := NewBadgerBackedKV(zap.NewNop(), nil, &Config{
		updatePolicy:   policy,
		valueType:      valueType,
		moduleHash:     "testhash",
		name:           "test_store",
		itemSizeLimit:  10_485_760,     // 10 MiB, matches NewConfig default
		appendLimit:    8_388_608,      // 8 MiB
		totalSizeLimit: StoreSizeLimit, // 1 GiB
	}, client)
	require.NoError(t, err)
	t.Cleanup(func() { conn.Close() })
	return s
}

// TestBadgerBackedStore_GetFirst_CacheMissHitsGRPC verifies that GetFirst on a
// cache-miss calls the foundational store over gRPC and populates readCache.
func TestBadgerBackedStore_GetFirst_CacheMissHitsGRPC(t *testing.T) {
	addr, _ := startTestFoundationalStore(t, t.TempDir())
	store := newTestBadgerStore(t, addr, pbsubstreams.Module_KindStore_UPDATE_POLICY_SET, "bytes")

	// Write a key through the store pipeline so it ends up in ForkAware
	store.SetBlockNum(10)
	store.SetBytes(0, "mykey", []byte("myvalue"))
	require.NoError(t, store.Flush())

	// Clear local state so the next GetFirst must hit gRPC
	store.baseStore.kv = make(map[string][]byte)

	val, found := store.GetFirst("mykey")
	require.True(t, found)
	assert.Equal(t, []byte("myvalue"), val)

	// kv (lazy Badger cache) must now be populated
	assert.NotEmpty(t, store.kv)
	cached, ok := store.kv["mykey"]
	require.True(t, ok)
	assert.Equal(t, []byte("myvalue"), cached)
}

// TestBadgerBackedStore_GetFirst_CacheHitSkipsGRPC verifies that GetFirst returns
// the cached value (from b.kv) without making a gRPC call.
func TestBadgerBackedStore_GetFirst_CacheHitSkipsGRPC(t *testing.T) {
	addr, _ := startTestFoundationalStore(t, t.TempDir())
	store := newTestBadgerStore(t, addr, pbsubstreams.Module_KindStore_UPDATE_POLICY_SET, "bytes")

	// Seed kv cache directly — if gRPC were called it would return not-found
	store.kv["prefix/key"] = []byte("cached")

	val, found := store.GetFirst("prefix/key")
	require.True(t, found)
	assert.Equal(t, []byte("cached"), val)
}

// TestBadgerBackedStore_GetAt_IntraBlockFromLocalKV verifies GetAt is served
// from the local kv (baseStore deltas) without going to gRPC.
// A key written via a SET delta at ordinal 3 must be visible at GetAt(3) and GetAt(5),
// but NOT via GetFirst (which would hit gRPC for a miss).
func TestBadgerBackedStore_GetAt_IntraBlockFromLocalKV(t *testing.T) {
	addr, _ := startTestFoundationalStore(t, t.TempDir())
	store := newTestBadgerStore(t, addr, pbsubstreams.Module_KindStore_UPDATE_POLICY_SET, "bytes")

	// Simulate a delta at ordinal 3 (as would be produced by baseStore.Flush)
	store.baseStore.deltas = []*pbsubstreams.StoreDelta{
		{
			Operation: pbsubstreams.StoreDelta_CREATE,
			Ordinal:   3,
			Key:       "ordkey",
			NewValue:  []byte("intra-block-val"),
		},
	}
	store.baseStore.kv["ordkey"] = []byte("intra-block-val")

	val, found := store.GetAt(3, "ordkey")
	require.True(t, found)
	assert.Equal(t, []byte("intra-block-val"), val)

	// At ordinal 2 (before the write) it should not be found
	_, foundBefore := store.GetAt(2, "ordkey")
	assert.False(t, foundBefore, "GetAt at ordinal before write should return not-found")
}

// TestBadgerBackedStore_AddAccumulation verifies that ADD bigint operations
// accumulate correctly across two Flush calls via the ForkAware → Badger pipeline.
func TestBadgerBackedStore_AddAccumulation(t *testing.T) {
	addr, backend := startTestFoundationalStore(t, t.TempDir())
	store := newTestBadgerStore(t, addr, pbsubstreams.Module_KindStore_UPDATE_POLICY_ADD, "bigint")

	// Simulate block 10: add 5 to "counter" using the proper store API
	store.SetBlockNum(10)
	store.SumBigInt(0, "counter", big.NewInt(5))
	require.NoError(t, store.Flush())
	store.Reset()

	// Simulate block 11: add 3 to "counter"
	store.SetBlockNum(11)
	store.SumBigInt(0, "counter", big.NewInt(3))
	require.NoError(t, store.Flush())
	store.Reset()

	// Flush the ForkAware cache to Badger (simulates FlushToBadger at segment boundary)
	require.NoError(t, store.FlushToBadger(11))

	// Verify Badger has accumulated value: 5+3=8
	resp, err := backend.GetFirst(&pbfoundationalservice.GetRequest{BlockNumber: ^uint64(0),
		Keys: []*pbmodel.Key{{Bytes: []byte("counter")}},
	})
	require.NoError(t, err)
	require.NotEmpty(t, resp.Entries.Entries)
	require.Equal(t, pbmodel.ResponseCode_RESPONSE_CODE_FOUND, resp.Entries.Entries[0].Code)
	// The value is stored as a decimal string by substreams bigint encoding
	gotStr := string(resp.Entries.Entries[0].Entry.Value.Value)
	assert.Equal(t, "8", gotStr, "ADD accumulation across blocks should produce 5+3=8")
}

// TestBadgerBackedStore_FlushToBadger_PersistsToBackend verifies that FlushToBadger
// triggers a ForkAware → Badger drain and the backend has the data.
func TestBadgerBackedStore_FlushToBadger_PersistsToBackend(t *testing.T) {
	addr, backend := startTestFoundationalStore(t, t.TempDir())
	store := newTestBadgerStore(t, addr, pbsubstreams.Module_KindStore_UPDATE_POLICY_SET, "bytes")

	store.SetBlockNum(100)
	store.kvOps.Operations = []*pbssinternal.Operation{
		{Type: pbssinternal.Operation_SET, Key: "persist-me", Value: []byte("hello")},
	}
	require.NoError(t, store.Flush())

	// Before FlushToBadger — Badger backend has nothing (still in ForkAware cache)
	resp, err := backend.GetFirst(&pbfoundationalservice.GetRequest{BlockNumber: ^uint64(0),
		Keys: []*pbmodel.Key{{Bytes: []byte("persist-me")}},
	})
	require.NoError(t, err)
	// Either empty or not-found
	notFoundBefore := len(resp.Entries.Entries) == 0 ||
		resp.Entries.Entries[0].Code == pbmodel.ResponseCode_RESPONSE_CODE_NOT_FOUND
	assert.True(t, notFoundBefore, "entry should not be in Badger backend before FlushToBadger")

	// FlushToBadger signals ForkAware to drain to Badger
	require.NoError(t, store.FlushToBadger(100))

	// Now Badger should have it
	resp, err = backend.GetFirst(&pbfoundationalservice.GetRequest{BlockNumber: ^uint64(0),
		Keys: []*pbmodel.Key{{Bytes: []byte("persist-me")}},
	})
	require.NoError(t, err)
	require.NotEmpty(t, resp.Entries.Entries)
	assert.Equal(t, pbmodel.ResponseCode_RESPONSE_CODE_FOUND, resp.Entries.Entries[0].Code)
}

// TestBadgerBackedStore_EvictFromBadger_RemovesSpeculativeEntries verifies that
// EvictFromBadger removes speculative ForkAware cache entries above the evict block.
func TestBadgerBackedStore_EvictFromBadger_RemovesSpeculativeEntries(t *testing.T) {
	addr, _ := startTestFoundationalStore(t, t.TempDir())
	store := newTestBadgerStore(t, addr, pbsubstreams.Module_KindStore_UPDATE_POLICY_SET, "bytes")

	// Write at block 50 (speculative — not yet flushed to Badger)
	store.SetBlockNum(50)
	store.kvOps.Operations = []*pbssinternal.Operation{
		{Type: pbssinternal.Operation_SET, Key: "speculative", Value: []byte("reorg-me")},
	}
	require.NoError(t, store.Flush())

	// Evict block 50 (simulates reorg)
	require.NoError(t, store.EvictFromBadger(50))

	// GetFirst should now return not-found (evicted from ForkAware cache)
	store.kv = make(map[string][]byte) // clear lazy kv cache too
	_, found := store.GetFirst("speculative")
	assert.False(t, found, "entry should be evicted after EvictFromBadger")
}

// Ensure BadgerBackedStore satisfies the Store interface at compile time.
var _ Store = (*BadgerBackedStore)(nil)

// TestBadgerBackedStore_Reset_PreservesKV verifies that Reset does NOT clear kv.
// kv is authoritative accumulated state (same contract as FullKV) and must
// survive across blocks so cross-block deltas have correct OldValue.
func TestBadgerBackedStore_Reset_PreservesKV(t *testing.T) {
	s := &BadgerBackedStore{
		baseStore: &baseStore{
			kvOps:  &pbssinternal.Operations{},
			Config: &Config{valueType: "bytes"},
			logger: zap.NewNop(),
			kv:     map[string][]byte{"key": []byte("value")},
		},
	}
	s.Reset()
	assert.Equal(t, []byte("value"), s.kv["key"],
		"Reset must preserve kv — it is authoritative accumulated state")
}

// TestBadgerBackedStore_Flush_SendsOpsToGRPC verifies that Flush sends kvOps
// to the foundational store and clears them.
func TestBadgerBackedStore_Flush_SendsOpsToGRPC(t *testing.T) {
	addr, backend := startTestFoundationalStore(t, t.TempDir())
	store := newTestBadgerStore(t, addr, pbsubstreams.Module_KindStore_UPDATE_POLICY_SET, "bytes")

	store.SetBlockNum(200)
	store.kvOps.Operations = []*pbssinternal.Operation{
		{Type: pbssinternal.Operation_SET, Key: "flush-key", Value: []byte("flush-val")},
	}
	require.NoError(t, store.Flush())

	// FlushToBadger to drain from ForkAware into Badger
	require.NoError(t, store.FlushToBadger(200))

	resp, err := backend.GetFirst(&pbfoundationalservice.GetRequest{BlockNumber: ^uint64(0),
		Keys: []*pbmodel.Key{{Bytes: []byte("flush-key")}},
	})
	require.NoError(t, err)
	require.Len(t, resp.Entries.Entries, 1)
	assert.Equal(t, pbmodel.ResponseCode_RESPONSE_CODE_FOUND, resp.Entries.Entries[0].Code)
	assert.Equal(t, []byte("flush-val"), resp.Entries.Entries[0].Entry.Value.Value)
}

// TestBadgerBackedStore_GetFirst_NotFound verifies GetFirst returns false when key absent.
func TestBadgerBackedStore_GetFirst_NotFound(t *testing.T) {
	addr, _ := startTestFoundationalStore(t, t.TempDir())
	store := newTestBadgerStore(t, addr, pbsubstreams.Module_KindStore_UPDATE_POLICY_SET, "bytes")

	_, found := store.GetFirst("no-such-key")
	assert.False(t, found)
}

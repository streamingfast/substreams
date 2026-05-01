package tests_e2e

import (
	"context"
	"fmt"
	"io"
	"math/big"
	"net"
	"path/filepath"
	"testing"

	fsgrpc "github.com/streamingfast/substreams-foundational-store/grpc"
	pbmodel "github.com/streamingfast/substreams-foundational-store/pb/sf/substreams/foundational-store/model/v2"
	pbfoundationalservice "github.com/streamingfast/substreams-foundational-store/pb/sf/substreams/foundational-store/service/v2"
	fsstore "github.com/streamingfast/substreams-foundational-store/store"
	fsForkAware "github.com/streamingfast/substreams-foundational-store/store/ForkAware"
	fsBadger "github.com/streamingfast/substreams-foundational-store/store/badger"
	"github.com/streamingfast/substreams/manifest"
	pbsubstreamsrpcv3 "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/anypb"
)

// startE2EFoundationalStoreWithBadger starts a real Badger-backed foundational store server
// using a temp dir for persistence, and returns the gRPC address and the backend store
// so the test can inspect persisted values.
func startE2EFoundationalStoreWithBadger(t *testing.T, tmpDir string) (addr string, backend *fsBadger.Store) {
	t.Helper()
	dsn, err := fsstore.ParseDSN("badger://" + filepath.Join(tmpDir, "fstore-badger"))
	require.NoError(t, err)

	badgerBackend, err := fsBadger.NewStore(dsn, "type.googleapis.com/google.protobuf.BytesValue", 1, zap.NewNop())
	require.NoError(t, err)

	forkAware := fsForkAware.NewStore(badgerBackend)
	lis, err := net.Listen("tcp", "localhost:0")
	require.NoError(t, err)

	srv := grpc.NewServer()
	// headBlockFetcher always says we're at max height so BlockReached is always true
	pbfoundationalservice.RegisterStoreServer(srv, fsgrpc.NewStoreServer(forkAware, func() uint64 { return ^uint64(0) }, zap.NewNop()))
	go srv.Serve(lis) //nolint:errcheck
	t.Cleanup(func() { srv.Stop() })
	return lis.Addr().String(), badgerBackend
}

// TestBadgerBackedStoreE2E is the main end-to-end test for BadgerBackedStore.
// It runs the full Tier1+Tier2 pipeline with store_stats backed by a real Badger store,
// then asserts:
//  1. All 100 requested blocks are returned.
//  2. map_stats produces non-empty output (read path works).
//  3. The "transactions" key in Badger has the correct accumulated bigint value
//     matching what the WASM ADD operations should have produced.
//  4. DeletePrefix correctly removes keys from Badger (tested in-process via the same server).
func TestBadgerBackedStoreE2E(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	container, err := newDummyBlockchainContainer(ctx, tmpDir, latestDummyBlockchainImage, "", 1000)
	require.NoError(t, err)
	defer container.Terminate(ctx, testcontainers.StopTimeout(0))

	// Start a real Badger-backed foundational store for "store_stats"
	fstoreAddr, badgerBackend := startE2EFoundationalStoreWithBadger(t, tmpDir)

	app2, t2Endpoint := startTier2App(t, ctx, tmpDir, zlog)
	app, substreamsEndpoint := startTier1AppWithBadgerBackedStores(t, ctx, tmpDir, container, t2Endpoint, zlog, map[string]string{
		"store_stats": fstoreAddr,
	})

	pkg, err := manifest.MustNewReader("./dummy/e2e-v0.1.0.spkg").Read()
	require.NoError(t, err)

	// store_stats has initialBlock: 400, updatePolicy: add, valueType: bigint
	// It adds len(blk.transactions) to key "transactions" on every block.
	// The dummy blockchain generates 1000 blocks with a fixed number of transactions per block.
	// We request blocks 400–499 (100 blocks).
	request := &pbsubstreamsrpcv3.Request{
		StartBlockNum:   400,
		StopBlockNum:    500,
		FinalBlocksOnly: false,
		ProductionMode:  true,
		OutputModule:    "map_stats",
		Package:         pkg.Package,
	}

	blockScopedDataSlice, session, err := RunRequest(t, request, substreamsEndpoint)
	if err != nil && err != io.EOF {
		t.Fatalf("unexpected error: %s", err)
	}

	// ── 1. Basic pipeline assertions ─────────────────────────────────────────

	require.NotNil(t, session)
	require.Equal(t, 100, len(blockScopedDataSlice), "expected 100 BlockScopedData responses")
	require.Equal(t, uint64(499), uint64(blockScopedDataSlice[len(blockScopedDataSlice)-1].Clock.Number))

	// ── 2. Read path: map_stats must receive non-empty output ────────────────
	// map_stats reads store_stats and emits events. If the store was never written
	// or the read path is broken, every block would return an empty proto.
	nonEmptyOutputs := 0
	for _, bsd := range blockScopedDataSlice {
		if bsd.Output != nil && bsd.Output.GetMapOutput() != nil && len(bsd.Output.GetMapOutput().GetValue()) > 0 {
			nonEmptyOutputs++
		}
	}
	assert.Greater(t, nonEmptyOutputs, 0, "map_stats should have produced non-empty output for at least one block")

	// ── 3. Accumulated bigint value in Badger must be correct ────────────────
	// After the pipeline finishes, FlushToBadger should have been called.
	// Query the Badger backend directly for the "transactions" key.
	// The store_stats module calls: store.add(0, "transactions", BigInt::from(blk.transactions.len()))
	// on every block 400–499. The ForkAware accumulates across blocks then flushes to Badger.
	// We flush explicitly here to ensure everything is drained.
	forkAwareClient, fstoreConn, err := connectToFStore(t, fstoreAddr)
	require.NoError(t, err)
	defer fstoreConn.Close()

	_, err = forkAwareClient.FlushUpToBlock(ctx, &pbfoundationalservice.FlushRequest{BlockNumber: ^uint64(0)})
	require.NoError(t, err, "explicit FlushUpToBlock should succeed")

	getResp, err := badgerBackend.GetFirst(&pbfoundationalservice.GetRequest{
		Keys:        []*pbmodel.Key{{Bytes: []byte("transactions")}},
		BlockNumber: ^uint64(0),
	})
	require.NoError(t, err, "should be able to query badger backend directly")
	require.NotNil(t, getResp)
	require.NotEmpty(t, getResp.Entries.Entries, "Badger backend should have at least one entry for 'transactions' key after FlushToBadger")
	require.Equal(t, pbmodel.ResponseCode_RESPONSE_CODE_FOUND, getResp.Entries.Entries[0].Code)

	// Parse the bigint value stored as a decimal string.
	storedVal := new(big.Int)
	storedStr := string(getResp.Entries.Entries[0].Entry.Value.Value)
	_, ok := storedVal.SetString(storedStr, 10)
	require.True(t, ok, "stored value %q should be a valid decimal bigint", storedStr)
	assert.True(t, storedVal.Sign() > 0, "accumulated transactions count must be > 0, got %s", storedStr)
	// The dummy blockchain generates exactly 13 transactions per block.
	// store_stats runs from block 400, we request 400–499 = 100 blocks.
	// Expected: 13 * 100 = 1300
	assert.Equal(t, "1300", storedStr, "accumulated transactions count should be 13 tx/block * 100 blocks")
	t.Logf("accumulated transactions count after 100 blocks: %s", storedStr)

	// ── 4. DeletePrefix: verify keys are removed from Badger ─────────────────
	// The store_stats module also adds a key per block hash:
	//   store.add(0, blk.header.hash, BigInt::from(blk.transactions.len()))
	// Flush so block-hash keys are in Badger, then call DeletePrefix to remove them,
	// then verify they are gone.
	//
	// We test this at the ForkAware gRPC layer (in-process) rather than through WASM
	// to avoid rebuilding the spkg.

	// Write a few test keys with a shared prefix to the foundational store
	testEntries := []*pbmodel.Entry{
		{Key: &pbmodel.Key{Bytes: []byte("pfx:key1")}, Value: &anypb.Any{Value: []byte("v1")}, UpdatePolicy: pbmodel.UpdatePolicy_UPDATE_POLICY_SET},
		{Key: &pbmodel.Key{Bytes: []byte("pfx:key2")}, Value: &anypb.Any{Value: []byte("v2")}, UpdatePolicy: pbmodel.UpdatePolicy_UPDATE_POLICY_SET},
		{Key: &pbmodel.Key{Bytes: []byte("other:key")}, Value: &anypb.Any{Value: []byte("keep")}, UpdatePolicy: pbmodel.UpdatePolicy_UPDATE_POLICY_SET},
	}
	_, err = forkAwareClient.SetAll(ctx, &pbfoundationalservice.SetRequest{
		SinkEntries: &pbmodel.SinkEntries{Entries: testEntries},
		BlockNumber: 600,
	})
	require.NoError(t, err)

	// Flush to Badger so the keys are persisted
	_, err = forkAwareClient.FlushUpToBlock(ctx, &pbfoundationalservice.FlushRequest{BlockNumber: 600})
	require.NoError(t, err)

	// Now send a SetAll with a delete prefix for "pfx:"
	_, err = forkAwareClient.SetAll(ctx, &pbfoundationalservice.SetRequest{
		SinkEntries: &pbmodel.SinkEntries{
			DeletePrefixes: []string{"pfx:"},
		},
		BlockNumber: 601,
	})
	require.NoError(t, err)

	// Flush again to drain the delete-prefix into Badger
	_, err = forkAwareClient.FlushUpToBlock(ctx, &pbfoundationalservice.FlushRequest{BlockNumber: 601})
	require.NoError(t, err)

	// "pfx:key1" and "pfx:key2" must be gone
	for _, deletedKey := range []string{"pfx:key1", "pfx:key2"} {
		resp, err := badgerBackend.GetFirst(&pbfoundationalservice.GetRequest{
			Keys:        []*pbmodel.Key{{Bytes: []byte(deletedKey)}},
			BlockNumber: ^uint64(0),
		})
		require.NoError(t, err)
		notFound := len(resp.Entries.Entries) == 0 ||
			resp.Entries.Entries[0].Code == pbmodel.ResponseCode_RESPONSE_CODE_NOT_FOUND
		assert.True(t, notFound, "key %q should be deleted after DeletePrefix(\"pfx:\")", deletedKey)
	}

	// "other:key" must still be present
	respOther, err := badgerBackend.GetFirst(&pbfoundationalservice.GetRequest{
		Keys:        []*pbmodel.Key{{Bytes: []byte("other:key")}},
		BlockNumber: ^uint64(0),
	})
	require.NoError(t, err)
	require.NotEmpty(t, respOther.Entries.Entries)
	assert.Equal(t, pbmodel.ResponseCode_RESPONSE_CODE_FOUND, respOther.Entries.Entries[0].Code,
		"key \"other:key\" should survive DeletePrefix(\"pfx:\")")

	app.Shutdown(nil)
	app2.Shutdown(nil)
	<-app.Terminated()
	<-app2.Terminated()
}

// connectToFStore dials the foundational store gRPC server and returns a client.
func connectToFStore(t *testing.T, addr string) (pbfoundationalservice.StoreClient, *grpc.ClientConn, error) {
	t.Helper()
	conn, err := grpc.NewClient(addr, grpc.WithInsecure()) //nolint:staticcheck
	if err != nil {
		return nil, nil, fmt.Errorf("dialing foundational store at %s: %w", addr, err)
	}
	return pbfoundationalservice.NewStoreClient(conn), conn, nil
}

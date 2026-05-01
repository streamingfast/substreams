package integration

import (
	"context"
	"net"
	"path/filepath"
	"sync"
	"testing"

	fsgrpc "github.com/streamingfast/substreams-foundational-store/grpc"
	pbfoundationalmodel "github.com/streamingfast/substreams-foundational-store/pb/sf/substreams/foundational-store/model/v2"
	pbfoundationalservice "github.com/streamingfast/substreams-foundational-store/pb/sf/substreams/foundational-store/service/v2"
	fsstore "github.com/streamingfast/substreams-foundational-store/store"
	fsForkAware "github.com/streamingfast/substreams-foundational-store/store/ForkAware"
	"github.com/streamingfast/substreams/manifest"
	"github.com/streamingfast/substreams/metering"
	"github.com/streamingfast/substreams/metrics"
	"github.com/streamingfast/substreams/orchestrator/work"
	"github.com/streamingfast/substreams/reqctx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"google.golang.org/grpc"
)

// inMemoryBackendStore is a minimal in-memory implementation of the foundational store.Store interface.
// It is used exclusively for integration tests it does NOT implement arithmetic accumulation (that's done
// in the ForkAware layer above it).
type inMemoryBackendStore struct {
	mu   sync.RWMutex
	data map[string]*pbfoundationalmodel.Entry
}

func newInMemoryBackendStore() *inMemoryBackendStore {
	return &inMemoryBackendStore{
		data: make(map[string]*pbfoundationalmodel.Entry),
	}
}

func (s *inMemoryBackendStore) SetAll(entries []*pbfoundationalmodel.Entry, _ []string, _ bool, _ uint64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, e := range entries {
		key := string(e.Key.Bytes)
		s.data[key] = e
	}
	return nil
}

func (s *inMemoryBackendStore) Get(req *pbfoundationalservice.GetRequest) (*pbfoundationalservice.GetResponse, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	resp := &pbfoundationalservice.GetResponse{
		Entries: &pbfoundationalmodel.QueriedEntries{},
	}
	for _, k := range req.Keys {
		key := string(k.Bytes)
		entry, ok := s.data[key]
		if ok {
			resp.Entries.Entries = append(resp.Entries.Entries, &pbfoundationalmodel.QueriedEntry{
				Code:  pbfoundationalmodel.ResponseCode_RESPONSE_CODE_FOUND,
				Entry: entry,
			})
		} else {
			resp.Entries.Entries = append(resp.Entries.Entries, &pbfoundationalmodel.QueriedEntry{
				Code: pbfoundationalmodel.ResponseCode_RESPONSE_CODE_NOT_FOUND,
			})
		}
	}
	return resp, nil
}

func (s *inMemoryBackendStore) GetFirst(req *pbfoundationalservice.GetRequest) (*pbfoundationalservice.GetResponse, error) {
	return s.Get(req)
}

var _ fsstore.Store = (*inMemoryBackendStore)(nil)

// startInProcessFoundationalStore starts an in-process gRPC server using the foundational store
// service backed by a ForkAware store over an in-memory backend. Returns the listener address.
func startInProcessFoundationalStore(t *testing.T) string {
	t.Helper()
	backend := newInMemoryBackendStore()
	forkAware := fsForkAware.NewStore(backend)

	lis, err := net.Listen("tcp", "localhost:0")
	require.NoError(t, err)

	grpcSrv := grpc.NewServer()
	// headBlockFetcher always reports max uint64 so BlockReached is always true.
	storeServer := fsgrpc.NewStoreServer(forkAware, func() uint64 { return ^uint64(0) }, zap.NewNop())
	pbfoundationalservice.RegisterStoreServer(grpcSrv, storeServer)
	go grpcSrv.Serve(lis) //nolint:errcheck
	t.Cleanup(func() { grpcSrv.Stop() })

	return lis.Addr().String()
}

// TestBadgerBackedStoreIntegration verifies that a store module declared as BadgerBacked
// accumulates values correctly across blocks via the foundational store service, and that
// downstream map modules that assert the store values pass.
func TestBadgerBackedStoreIntegration(t *testing.T) {
	manifest.TestUseSimpleHash = true

	// Start in-process foundational store server for the "setup_test_store_add_i64" module.
	addr := startInProcessFoundationalStore(t)

	ctx := context.Background()
	ctx = metering.WithMetricsSender(ctx)
	ctx = reqctx.WithReqStats(ctx, metrics.NewReqStats(&metrics.Config{}, nil, nil, zlog))

	testTempDir := t.TempDir()

	ctx = reqctx.WithTier2RequestParameters(ctx, reqctx.Tier2RequestParameters{
		BlockType:            "sf.substreams.v1.test.Block",
		FirstStreamableBlock: 0,
		StateBundleSize:      10,
		StateStoreURL:        filepath.Join(testTempDir, "test.store"),
		MeteringConfig:       "some_metering_config",
		MergedBlockStoreURL:  "some_merged_block_store_url",
		StateStoreDefaultTag: "tag",
		// Wire "setup_test_store_add_i64" to our in-process foundational store server.
		BadgerBackedStoreEndpoints: map[string]string{
			"setup_test_store_add_i64": addr,
		},
	})

	manifestPath := "./testdata/simple_substreams_init0/substreams-test-init0-v0.1.0.spkg"
	pkg := manifest.TestReadManifest(t, manifestPath)
	moduleName := "assert_test_store_add_i64"

	ctx = reqctx.WithRequest(ctx, &reqctx.RequestDetails{Modules: pkg.Modules, OutputModule: moduleName})

	responseCollector := newResponseCollector(ctx, moduleName, 1, 11)

	newBlockGenerator := func(startBlock uint64, inclusiveStopBlock uint64) TestBlockGenerator {
		return &LinearBlockGenerator{
			startBlock:         startBlock,
			inclusiveStopBlock: inclusiveStopBlock,
		}
	}

	// Stage 1 runs both setup_test_store_add_i64 (store) and assert_test_store_add_i64 (map).
	request := work.NewRequest(ctx, reqctx.Details(ctx), 1 /* stage */, 1 /* startBlock */, true /* streamOutput */)
	require.NoError(t, request.Validate())

	err := processInternalRequest(t, ctx, request, nil, newBlockGenerator, responseCollector, nil, testTempDir)
	require.NoError(t, err)

	// The assert_test_store_add_i64 map module outputs proto:Boolean{value:true} = 0x0801
	// when the store values are as expected.
	assert.NotEmpty(t, responseCollector.responses, "expected at least one response")

	anyTrue := false
	for _, resp := range responseCollector.responses {
		if bsd := resp.GetBlockScopedData(); bsd != nil {
			for _, output := range bsd.AllModuleOutputs() {
				if output.Name() == moduleName {
					mapout := output.MapOutput.GetMapOutput()
					if mapout != nil && len(mapout.Value) > 0 {
						// 0x0801 = field 1, varint 1 = Boolean{value: true}
						if len(mapout.Value) >= 2 && mapout.Value[0] == 0x08 && mapout.Value[1] == 0x01 {
							anyTrue = true
						}
					}
				}
			}
		}
	}
	assert.True(t, anyTrue, "expected at least one block where assert_test_store_add_i64 returned true (0x0801)")
}

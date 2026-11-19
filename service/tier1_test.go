package service

import (
	"context"
	"errors"
	"io"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/streamingfast/derr"
	"github.com/streamingfast/dstore"
	"github.com/streamingfast/shutter"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/pipeline/exec"
	"github.com/streamingfast/substreams/reqctx"
	"github.com/streamingfast/substreams/storage/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func createTestStoreConfig(name string, initialBlock uint64, mockStore dstore.Store) (*store.Config, error) {
	return store.NewConfig(
		name,
		initialBlock,
		"test-hash",
		pbsubstreams.Module_KindStore_UPDATE_POLICY_SET,
		"string",
		mockStore,
		nil, // quickSaveStore
		0,
		"",
		"",
	)
}

func TestConfigureLiveBackFillerFromQuickload(t *testing.T) {
	logger := zap.NewNop()

	tests := []struct {
		name                   string
		segmentSize            uint64
		linearHandoffBlockNum  uint64
		firstStreamableBlock   uint64
		usedStores             []*pbsubstreams.Module
		setupStoreFiles        func(dstore.Store)
		expectedCurrentSegment uint64
		expectedError          string
	}{
		{
			name:                  "single module with no snapshots - rewinds to lowest index",
			segmentSize:           1000,
			linearHandoffBlockNum: 5000,
			usedStores: []*pbsubstreams.Module{
				{Name: "store1", InitialBlock: 0},
			},
			setupStoreFiles: func(ms dstore.Store) {
				// No files to add - empty store
			},
			expectedCurrentSegment: 0, // Should rewind to 0 (lowestIndex)
		},
		{
			name:                  "single module with snapshots - rewinds to latest snapshot segment",
			segmentSize:           1000,
			linearHandoffBlockNum: 5123,
			usedStores: []*pbsubstreams.Module{
				{Name: "store1", InitialBlock: 2000},
			},
			setupStoreFiles: func(ms dstore.Store) {
				mockStore := ms.(*dstore.MockStore)
				mockStore.WalkFunc = func(ctx context.Context, prefix string, f func(filename string) error) error {
					files := []string{
						"0000003000-0000002000.kv", // state at block 3000
						"0000004000-0000002000.kv", // state at block 4000
					}
					for _, file := range files {
						if err := f(file); err != nil {
							return err
						}
					}
					return nil
				}
			},
			expectedCurrentSegment: 4, // 4000 / 1000 = 4
		},
		{
			name:                  "store starting before the first streamable block, no snapshot",
			segmentSize:           1000,
			linearHandoffBlockNum: 5123,
			firstStreamableBlock:  2500,
			usedStores: []*pbsubstreams.Module{
				{Name: "store1", InitialBlock: 2000},
			},
			setupStoreFiles:        func(ms dstore.Store) {},
			expectedCurrentSegment: 3, // first segment holding no block below 2500
		},
		{
			name:                  "snapshot named after another initial block",
			segmentSize:           1000,
			linearHandoffBlockNum: 5123,
			usedStores: []*pbsubstreams.Module{
				{Name: "store1", InitialBlock: 2000},
			},
			setupStoreFiles: func(ms dstore.Store) {
				mockStore := ms.(*dstore.MockStore)
				mockStore.WalkFunc = func(ctx context.Context, prefix string, f func(filename string) error) error {
					for _, file := range []string{
						"0000003000-0000002000.kv",
						"0000004000-0000001000.kv", // cannot be loaded by a store starting at 2000
					} {
						if err := f(file); err != nil {
							return err
						}
					}
					return nil
				}
			},
			expectedCurrentSegment: 3,
		},
		{
			name:                   "no modules - no change",
			segmentSize:            1000,
			linearHandoffBlockNum:  5123,
			usedStores:             []*pbsubstreams.Module{},
			setupStoreFiles:        func(ms dstore.Store) {},
			expectedCurrentSegment: 5, // Should remain unchanged
		},
		{
			name:                  "error from store",
			segmentSize:           1000,
			linearHandoffBlockNum: 5123,
			usedStores: []*pbsubstreams.Module{
				{Name: "store1", InitialBlock: 0},
			},
			setupStoreFiles: func(ms dstore.Store) {
				mockStore := ms.(*dstore.MockStore)
				mockStore.WalkFunc = func(ctx context.Context, prefix string, f func(filename string) error) error {
					return derr.NewFatalError(errors.New("storage error"))
				}
			},
			expectedError: "storage error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()

			// Create mock store and configure it
			mockStore := dstore.NewMockStore(nil)
			tt.setupStoreFiles(mockStore)

			// Create store configs
			storeConfigs := make(store.ConfigMap)
			for _, mod := range tt.usedStores {
				config, err := createTestStoreConfig(mod.Name, mod.InitialBlock, mockStore)
				require.NoError(t, err)
				storeConfigs[mod.Name] = config
			}

			// Create a LiveBackFiller with the initial segment
			backFiller := NewLiveBackFiller(ctx, nil, logger, 0, tt.segmentSize, tt.linearHandoffBlockNum, nil, nil)

			err := configureLiveBackFillerFromQuickload(
				ctx,
				tt.segmentSize,
				tt.linearHandoffBlockNum,
				tt.firstStreamableBlock,
				tt.usedStores,
				storeConfigs,
				backFiller,
			)

			if tt.expectedError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expectedCurrentSegment, backFiller.CurrentSegment())
		})
	}
}

func TestConfigureLiveBackFillerFromQuickload_EdgeCases(t *testing.T) {
	logger := zap.NewNop()

	t.Run("context cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		mockStore := dstore.NewMockStore(nil)
		mockStore.WalkFunc = func(ctx context.Context, prefix string, f func(filename string) error) error {
			return context.Canceled
		}

		config, err := createTestStoreConfig("store1", 0, mockStore)
		require.NoError(t, err)

		storeConfigs := store.ConfigMap{"store1": config}
		backFiller := NewLiveBackFiller(context.Background(), nil, logger, 0, 1000, 5000, nil, nil)

		err = configureLiveBackFillerFromQuickload(
			ctx,
			1000,
			5000,
			0,
			[]*pbsubstreams.Module{{Name: "store1", InitialBlock: 0}},
			storeConfigs,
			backFiller,
		)

		assert.Error(t, err)
		assert.True(t, errors.Is(err, context.Canceled))
	})

	t.Run("very large numbers", func(t *testing.T) {
		ctx := context.Background()

		mockStore := dstore.NewMockStore(nil)
		config, err := createTestStoreConfig("store1", 0, mockStore)
		require.NoError(t, err)

		storeConfigs := store.ConfigMap{"store1": config}
		backFiller := NewLiveBackFiller(ctx, nil, logger, 0, 1000000, 10000000, nil, nil)

		err = configureLiveBackFillerFromQuickload(
			ctx,
			1000000,
			1000000000,
			0,
			[]*pbsubstreams.Module{{Name: "store1", InitialBlock: 0}},
			storeConfigs,
			backFiller,
		)

		require.NoError(t, err)
		// Should rewind to 0 since no snapshots and lowest index is 0
		assert.Equal(t, uint64(0), backFiller.CurrentSegment())
	})

	t.Run("currentSegment already at minimum - rewind only lowers", func(t *testing.T) {
		ctx := context.Background()

		mockStore := dstore.NewMockStore(nil)
		config, err := createTestStoreConfig("store1", 3000, mockStore)
		require.NoError(t, err)

		storeConfigs := store.ConfigMap{"store1": config}

		// Start with currentSegment at 2
		backFiller := NewLiveBackFiller(ctx, nil, logger, 0, 1000, 2000, nil, nil)
		initialSegment := backFiller.CurrentSegment()

		err = configureLiveBackFillerFromQuickload(
			ctx,
			1000,
			5000,
			0,
			[]*pbsubstreams.Module{{Name: "store1", InitialBlock: 3000}}, // lowestIndex = 3, higher than backfillFromIndex
			storeConfigs,
			backFiller,
		)

		require.NoError(t, err)
		// The Rewind should be called with 5 (backfillFromIndex), but since 5 > initialSegment (2),
		// currentSegment should remain at the lower value
		assert.Equal(t, initialSegment, backFiller.CurrentSegment())
	})
}

func TestWithStoresBackend_BothTiersAligned(t *testing.T) {
	scratch := t.TempDir() // WithStoresScratchSpace sweeps this dir; keep it real + empty

	t.Run("tier1", func(t *testing.T) {
		s := &Tier1Service{}
		WithStoresBackend("mmap")(s)
		WithStoresScratchSpace(scratch)(s)

		assert.Equal(t, "mmap", s.runtimeConfig.StoresBackend, "tier1 must read backend from WithStoresBackend")
		assert.Equal(t, scratch, s.runtimeConfig.StoresScratchSpace, "tier1 must read scratch space from WithStoresScratchSpace")
	})

	t.Run("tier2", func(t *testing.T) {
		s := &Tier2Service{}
		WithStoresBackend("mmap")(s)
		WithStoresScratchSpace(scratch)(s)

		assert.Equal(t, "mmap", s.storesBackend, "tier2 must read backend from WithStoresBackend")
		assert.Equal(t, scratch, s.storesScratchSpace, "tier2 must read scratch space from WithStoresScratchSpace")
	})

	// Both tiers must resolve the SAME backend string identically — the whole
	// point of the alignment. A future divergence (e.g. one tier ignoring the
	// option) should fail here.
	t.Run("tiers agree", func(t *testing.T) {
		for _, backend := range []string{"memory", "mmap"} {
			t1 := &Tier1Service{}
			t2 := &Tier2Service{}
			WithStoresBackend(backend)(t1)
			WithStoresBackend(backend)(t2)
			assert.Equal(t, t1.runtimeConfig.StoresBackend, t2.storesBackend,
				"tier1 and tier2 must resolve backend %q to the same value", backend)
		}
	})
}

func TestLastUsedFilename(t *testing.T) {
	assert.Equal(t, "last_used", lastUsedFilename(""))
	assert.Equal(t, "last_used_pro", lastUsedFilename("PRO"))
	assert.Equal(t, "last_used_enterprise", lastUsedFilename("ENTERPRISE"))
}

func TestEvery(t *testing.T) {
	t.Run("calls fn each interval until ctx is done", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			var calls atomic.Int32
			ctx, cancel := context.WithCancel(t.Context())
			go every(ctx, time.Hour, func() { calls.Add(1) })

			time.Sleep(3*time.Hour + time.Minute)
			synctest.Wait()
			assert.Equal(t, int32(3), calls.Load())

			// synctest.Test fails if every is still blocked once the bubble ends.
			cancel()
			time.Sleep(10 * time.Hour)
			assert.Equal(t, int32(3), calls.Load(), "fn called after ctx was canceled")
		})
	})

	t.Run("non-positive interval never calls fn", func(t *testing.T) {
		every(t.Context(), 0, func() { require.Fail(t, "fn called") })
		every(t.Context(), -time.Second, func() { require.Fail(t, "fn called") })
	})
}

func TestWriteLastUsedContinuesPastFailures(t *testing.T) {
	execGraph, err := exec.NewOutputModuleGraph("map_a", true, &pbsubstreams.Modules{
		Modules: []*pbsubstreams.Module{
			{
				Name: "store_a",
				Kind: &pbsubstreams.Module_KindStore_{KindStore: &pbsubstreams.Module_KindStore{
					UpdatePolicy: pbsubstreams.Module_KindStore_UPDATE_POLICY_SET,
					ValueType:    "string",
				}},
				Inputs: []*pbsubstreams.Module_Input{
					{Input: &pbsubstreams.Module_Input_Source_{Source: &pbsubstreams.Module_Input_Source{Type: "test.Block"}}},
				},
			},
			{
				Name: "map_a",
				Kind: &pbsubstreams.Module_KindMap_{},
				Inputs: []*pbsubstreams.Module_Input{
					{Input: &pbsubstreams.Module_Input_Source_{Source: &pbsubstreams.Module_Input_Source{Type: "test.Block"}}},
					{Input: &pbsubstreams.Module_Input_Store_{Store: &pbsubstreams.Module_Input_Store{
						ModuleName: "store_a",
						Mode:       pbsubstreams.Module_Input_Store_GET,
					}}},
				},
			},
		},
		Binaries: []*pbsubstreams.Binary{{Type: "test", Content: []byte("fake")}},
	}, 0, false)
	require.NoError(t, err)
	require.Len(t, execGraph.UsedModules(), 2)

	var writes atomic.Int32
	cacheStore := dstore.NewMockStore(nil)
	cacheStore.WriteObjectFunc = func(_ context.Context, base string, _ io.Reader, _ ...string) error {
		assert.Equal(t, "last_used", base)
		if writes.Add(1) == 1 {
			return errors.New("write failed")
		}
		return nil
	}

	ctx := reqctx.WithRequest(context.Background(), &reqctx.RequestDetails{})
	err = (&Tier1Service{}).writeLastUsed(ctx, execGraph, cacheStore)
	require.ErrorContains(t, err, "write failed")
	assert.Equal(t, int32(2), writes.Load(), "a failed write must not skip the other modules")
}

func TestLastUsedRefreshIntervalOption(t *testing.T) {
	s := &Tier1Service{lastUsedRefreshInterval: defaultLastUsedRefreshInterval}
	WithLastUsedRefreshInterval(2 * time.Hour)(s)
	assert.Equal(t, 2*time.Hour, s.lastUsedRefreshInterval)
	WithLastUsedRefreshInterval(0)(s)
	assert.Zero(t, s.lastUsedRefreshInterval)
}

func TestPendingShutdownCheck(t *testing.T) {
	t.Run("no max request duration follows service termination", func(t *testing.T) {
		s := &Tier1Service{Shutter: shutter.New()}
		check := s.pendingShutdownCheck(time.Time{}, zap.NewNop())
		assert.False(t, check())
		s.Shutdown(nil)
		assert.True(t, check())
	})

	t.Run("max request duration drains once elapsed", func(t *testing.T) {
		s := &Tier1Service{Shutter: shutter.New()}
		check := s.pendingShutdownCheck(time.Now().Add(50*time.Millisecond), zap.NewNop())
		assert.False(t, check())
		time.Sleep(60 * time.Millisecond)
		assert.True(t, check())
	})

	t.Run("max request duration still drains on service termination", func(t *testing.T) {
		s := &Tier1Service{Shutter: shutter.New()}
		check := s.pendingShutdownCheck(time.Now().Add(time.Hour), zap.NewNop())
		assert.False(t, check())
		s.Shutdown(nil)
		assert.True(t, check())
	})
}

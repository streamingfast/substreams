package service

import (
	"context"
	"errors"
	"testing"

	"github.com/streamingfast/derr"
	"github.com/streamingfast/dstore"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
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
						"0000003000-0000002000.kv", // 2000-3000 range
						"0000004000-0000003000.kv", // 3000-4000 range
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

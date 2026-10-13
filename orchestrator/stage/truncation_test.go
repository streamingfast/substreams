package stage

import (
	"context"
	"testing"

	"github.com/streamingfast/bstream/stream"
	"github.com/streamingfast/dstore"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/reqctx"
	"github.com/streamingfast/substreams/storage/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The chain was truncated at block 30: test_store started at block 5, so its files are named
// after block 5, but its stage can only run from block 30 on.
func TestFetchCachesState_TruncatedStore(t *testing.T) {
	tests := []struct {
		name              string
		storeInitialBlock uint64
		files             []string
		wantStates        string
		wantErr           bool
	}{
		{
			name:              "resumes from a snapshot above the first streamable block",
			storeInitialBlock: 5,
			files:             []string{"hash/states/0000000050-0000000005.kv"},
			wantStates:        `S:...NC`,
		},
		{
			name:              "resumes from a snapshot at the first streamable block",
			storeInitialBlock: 5,
			files:             []string{"hash/states/0000000030-0000000005.kv"},
			wantStates:        `S:`,
		},
		{
			name:              "snapshots below the first streamable block cannot be used",
			storeInitialBlock: 5,
			files:             []string{"hash/states/0000000020-0000000005.kv"},
			wantErr:           true,
		},
		{
			name:              "no snapshot",
			storeInitialBlock: 5,
			wantErr:           true,
		},
		{
			name:              "store starting at the first streamable block builds from scratch",
			storeInitialBlock: 30,
			wantStates:        `S:`,
		},
	}

	storesWalkWindowSegments = 2
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			files := map[string][]byte{}
			for _, f := range tt.files {
				files[f] = nil
			}
			stages := &Stages{
				globalSegmenter: segmenter(30, 60),
				storeSegmenter:  segmenter(30, 60),
				stages: []*Stage{
					{
						segmenter: segmenter(30, 60),
						storeModuleStates: []*StoreModuleState{
							{name: "test_store", segmenter: segmenter(30, 60)},
						},
						kind: KindStore,
					},
				},
				storeConfigs: store.ConfigMap{
					"test_store": testStoreConfig(t, "test_store", tt.storeInitialBlock, "hash", pbsubstreams.Module_KindStore_UPDATE_POLICY_SET, "value", &dstore.MockStore{Files: files}),
				},
			}
			ctx := reqctx.WithRequest(context.Background(), &reqctx.RequestDetails{ResolvedStartBlockNum: 60})

			err := stages.FetchCachesState(ctx)
			if tt.wantErr {
				var invalidArg *stream.ErrInvalidArg
				require.ErrorAs(t, err, &invalidArg)
				assert.Contains(t, err.Error(), `store "test_store" starts at block 5`)
				return
			}
			require.NoError(t, err)
			segmentStateEquals(t, stages, tt.wantStates)
		})
	}
}

package orchestrator

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/abourget/llerrgroup"
	"github.com/streamingfast/dstore"
	"github.com/streamingfast/shutter"
	"github.com/streamingfast/substreams/block"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/reqctx"
	"github.com/streamingfast/substreams/storage/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestShouldSaveFullKV(t *testing.T) {
	tests := []struct {
		name              string
		storeSaveInterval uint64
		storeInitialBlock uint64
		squashableRange   *block.Range
		expectValue       bool
	}{
		{
			name:              "first range",
			storeSaveInterval: 10,
			storeInitialBlock: 0,
			squashableRange:   block.ParseRange("0-10"),
			expectValue:       true,
		},
		{
			name:              "first range, doesn't start on store boundary",
			storeSaveInterval: 10,
			storeInitialBlock: 5,
			squashableRange:   block.ParseRange("5-10"),
			expectValue:       true,
		},
		{
			name:              "range does not end on boundary",
			storeSaveInterval: 10,
			storeInitialBlock: 5,
			squashableRange:   block.ParseRange("10-15"),
			expectValue:       false,
		},
		{
			name:              "range end on store boundary",
			storeSaveInterval: 10,
			storeInitialBlock: 0,
			squashableRange:   block.ParseRange("10-20"),
			expectValue:       true,
		},
		{
			name:              "range does not end on store boundary greater",
			storeSaveInterval: 10,
			storeInitialBlock: 0,
			squashableRange:   block.ParseRange("10-45"),
			expectValue:       false,
		},
		{
			name:              "range ends on store boundary wih big range",
			storeSaveInterval: 10,
			storeInitialBlock: 0,
			squashableRange:   block.ParseRange("10-50"),
			expectValue:       false,
		},
	}
	for _, test := range tests {
		squasher := &StoreSquasher{storeSaveInterval: test.storeSaveInterval}
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.expectValue, squasher.shouldSaveFullKV(test.storeInitialBlock, test.squashableRange))
		})
	}

}

func TestStoreSquasher_squash(t *testing.T) {
	tests := []struct {
		name        string
		ranges      store.FileInfos
		expectErr   error
		expectCount int
	}{
		{
			name:        "squash with ranges",
			ranges:      store.PartialFiles("0-10,20-30"),
			expectCount: 1,
		},
		{
			name:      "squash without ranges",
			ranges:    store.FileInfos{},
			expectErr: fmt.Errorf("partialsChunks is empty for module \"mod\""),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			partialsChunks := make(chan store.FileInfos, 10)
			squasher := &StoreSquasher{
				Shutter:        shutter.New(),
				name:           "mod",
				partialsChunks: partialsChunks,
			}
			err := squasher.squash(context.Background(), test.ranges)
			if test.expectErr != nil {
				require.Error(t, err)
				assert.Equal(t, test.expectErr, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, test.expectCount, len(squasher.partialsChunks))
			}

		})
	}
}
func TestStoreSquasher_sortRange(t *testing.T) {
	s := &StoreSquasher{files: store.PartialFiles("10-20,40-50,0-10")}
	s.sortRange()
	assert.Equal(t, store.PartialFiles("0-10,10-20,40-50"), s.files)
}

func TestStoreSquasher_ensureContiguity(t *testing.T) {
	s := &StoreSquasher{files: store.PartialFiles("10-20,40-50,45-48")}
	assert.Error(t, s.ensureNoOverlap())

	s = &StoreSquasher{files: store.PartialFiles("10-20,40-50")}
	assert.NoError(t, s.ensureNoOverlap())

	s = &StoreSquasher{files: store.PartialFiles("10-20,20-50")}
	assert.NoError(t, s.ensureNoOverlap())
}

func TestStoreSquasher_getPartialChunks(t *testing.T) {
	ctx := context.Background()
	s := &StoreSquasher{
		Shutter:        shutter.New(),
		partialsChunks: make(chan store.FileInfos, 10),
		files:          store.FileInfos{},
		store:          newTestStore(t, dstore.NewMockStore(nil), 0),
	}
	go func() {
		s.partialsChunks <- store.PartialFiles("0-10")
	}()
	err := s.accumulateMorePartials(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, len(s.files))

	cacnelCtx, cancelFunc := context.WithCancel(ctx)
	go func() {
		cancelFunc()
	}()
	err = s.accumulateMorePartials(cacnelCtx)
	require.Error(t, err)
	assert.Equal(t, context.Canceled, err)

	go func() {
		close(s.partialsChunks)
	}()
	err = s.accumulateMorePartials(ctx)
	require.Error(t, err)
	assert.Equal(t, PartialsChannelClosed, err)
}

func TestStoreSquasher_processRange(t *testing.T) {

	tests := []struct {
		name                         string
		storeInitialBlock            uint64
		nextExpectedStartBlock       uint64
		storeSaveInterval            uint64
		squashableFile               *store.FileInfo
		expectPartialFilenameLoaded  string
		expectNextExpectedStartBlock uint64
		expectShouldSaveFullStore    bool
		expectError                  error
	}{
		{
			name:                         "expect and received the first partial",
			storeInitialBlock:            0,  // modules starts at 0
			nextExpectedStartBlock:       0,  // we are expecting the first partial to be completed
			storeSaveInterval:            10, // store files contains 10 blocks
			squashableFile:               store.PartialFile("0-10", store.TraceIDParam("testTraceID")),
			expectPartialFilenameLoaded:  "0000000010-0000000000.testTraceID.partial",
			expectNextExpectedStartBlock: 10,
			expectShouldSaveFullStore:    true,
		},
		{
			name:                   "skips range, when it does not receive the expected range",
			storeInitialBlock:      0,  // modules starts at 0
			nextExpectedStartBlock: 0,  // we are expecting the first partial to be completed
			storeSaveInterval:      10, // store files contains 10 blocks
			squashableFile:         store.PartialFile("10-20", store.TraceIDParam("testTraceID")),
			expectError:            SkipFile,
		},
		{
			name:                   "called when ranges out of order",
			storeInitialBlock:      0,  // modules starts at 0
			nextExpectedStartBlock: 50, // we are expecting the first partial to be completed
			storeSaveInterval:      10, // store files contains 10 blocks
			squashableFile:         store.PartialFile("40-50", store.TraceIDParam("testTraceID")),
			expectError:            fmt.Errorf("non contiguous ranges were added to the store squasher, expected 50, got 40, ranges: "),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			eg := llerrgroup.New(250)

			savedFullKCStore := false
			loadedPartialFilename := ""
			testStore := dstore.NewMockStore(nil)
			testStore.OpenObjectFunc = func(ctx context.Context, name string) (out io.ReadCloser, err error) {
				if strings.HasSuffix(name, ".partial") {
					loadedPartialFilename = name
				}
				return newPartialKVContent(t, map[string][]byte{}, &store.FullKV{}), nil
			}
			testStore.WriteObjectFunc = func(ctx context.Context, base string, f io.Reader) error {
				if strings.HasSuffix(base, ".kv") {
					savedFullKCStore = true
				}
				return nil
			}
			squasher := &StoreSquasher{
				store:                  newTestStore(t, testStore, test.storeInitialBlock),
				nextExpectedStartBlock: test.nextExpectedStartBlock,
				storeSaveInterval:      test.storeSaveInterval,
			}
			ctx := reqctx.WithRequest(context.Background(), &reqctx.RequestDetails{
				ProductionMode: false,
			})
			err := squasher.processSquashableFile(ctx, eg, test.squashableFile)
			require.NoError(t, eg.Wait())

			if test.expectError != nil {
				assert.Equal(t, test.expectError, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, test.expectNextExpectedStartBlock, squasher.nextExpectedStartBlock)
				assert.Equal(t, test.expectShouldSaveFullStore, savedFullKCStore)
				assert.Equal(t, test.expectPartialFilenameLoaded, loadedPartialFilename)
			}
		})
	}
}

func newTestStore(t *testing.T, testStore dstore.Store, initialBlock uint64) *store.FullKV {
	c, err := store.NewConfig(
		"mod",
		initialBlock,
		"mod.hash",
		pbsubstreams.Module_KindStore_UPDATE_POLICY_SET,
		"",
		testStore,
		"testTraceID",
	)
	require.NoError(t, err)

	return c.NewFullKV(zap.NewNop())
}

func newPartialKVContent(t *testing.T, data map[string][]byte, kv *store.FullKV) io.ReadCloser {
	//marshaller := kv.Marshaller()
	//content, err := marshaller.Marshal(data)
	//require.NoError(t, err)
	return io.NopCloser(bytes.NewReader([]byte{}))
}

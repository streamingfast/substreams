package orchestrator

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"strings"
	"testing"

	store2 "github.com/streamingfast/substreams/storage/store"

	"github.com/abourget/llerrgroup"
	"github.com/streamingfast/dstore"
	"github.com/streamingfast/substreams/block"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
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
		ranges      block.Ranges
		expectErr   error
		expectCount int
	}{
		{
			name:        "squash with ranges",
			ranges:      block.ParseRanges("0-10,20-30"),
			expectCount: 1,
		},
		{
			name:      "squash without ranges",
			ranges:    []*block.Range{},
			expectErr: fmt.Errorf("partialsChunks is empty for module \"mod\""),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			partialsChunks := make(chan block.Ranges, 10)
			squasher := &StoreSquasher{
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
	s := &StoreSquasher{ranges: block.ParseRanges("10-20,40-50,0-10")}
	s.sortRange()
	assert.Equal(t, block.ParseRanges("0-10,10-20,40-50"), s.ranges)
}

func TestStoreSquasher_getPartialChunks(t *testing.T) {
	ctx := context.Background()
	s := &StoreSquasher{
		partialsChunks: make(chan block.Ranges, 10),
		ranges:         []*block.Range{},
		store:          newTestStore(t, dstore.NewMockStore(nil), 0),
	}
	go func() {
		s.partialsChunks <- block.ParseRanges("0-10")
	}()
	err := s.getPartialChunks(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, len(s.ranges))

	cacnelCtx, cancelFunc := context.WithCancel(ctx)
	go func() {
		cancelFunc()
	}()
	err = s.getPartialChunks(cacnelCtx)
	require.Error(t, err)
	assert.Equal(t, context.Canceled, err)

	go func() {
		close(s.partialsChunks)
	}()
	err = s.getPartialChunks(ctx)
	require.Error(t, err)
	assert.Equal(t, PartialChunksDone, err)
}

func TestStoreSquasher_processRange(t *testing.T) {

	tests := []struct {
		name                         string
		storeInitialBlock            uint64
		nextExpectedStartBlock       uint64
		storeSaveInterval            uint64
		squashableRange              *block.Range
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
			squashableRange:              block.NewRange(0, 10),
			expectPartialFilenameLoaded:  "0000000010-0000000000.partial",
			expectNextExpectedStartBlock: 10,
			expectShouldSaveFullStore:    true,
		},
		{
			name:                   "skips range, when it does not receive the expected range",
			storeInitialBlock:      0,  // modules starts at 0
			nextExpectedStartBlock: 0,  // we are expecting the first partial to be completed
			storeSaveInterval:      10, // store files contains 10 blocks
			squashableRange:        block.NewRange(10, 20),
			expectError:            SkipRange,
		},
		{
			name:                   "called when ranges out of order",
			storeInitialBlock:      0,  // modules starts at 0
			nextExpectedStartBlock: 50, // we are expecting the first partial to be completed
			storeSaveInterval:      10, // store files contains 10 blocks
			squashableRange:        block.NewRange(40, 50),
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
				return newPartialKVContent(t, map[string][]byte{}, &store2.FullKV{}), nil
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
			err := squasher.processRange(context.Background(), eg, test.squashableRange)
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

func newTestStore(t *testing.T, testStore dstore.Store, initialBlock uint64) *store2.FullKV {
	c, err := store2.NewConfig(
		"mod",
		initialBlock,
		"mod.hash",
		pbsubstreams.Module_KindStore_UPDATE_POLICY_SET,
		"",
		testStore,
	)
	require.NoError(t, err)

	return c.NewFullKV(zap.NewNop())
}

func newPartialKVContent(t *testing.T, data map[string][]byte, kv *store2.FullKV) io.ReadCloser {
	//marshaller := kv.Marshaller()
	//content, err := marshaller.Marshal(data)
	//require.NoError(t, err)
	return ioutil.NopCloser(bytes.NewReader([]byte{}))
}

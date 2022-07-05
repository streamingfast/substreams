package orchestrator

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"testing"

	"github.com/streamingfast/dstore"
	"github.com/streamingfast/substreams/block"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/state"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type NotifierFunc func()

func (n NotifierFunc) Notify(builder string, blockNum uint64) {
	n()
}

// func TestInitSquasher(t *testing.T) {
// 	ctx := context.Background()
// 	fileStore := dstore.NewMockStore(nil)
// 	fileStore.OpenObjectFunc = func(ctx context.Context, name string) (out io.ReadCloser, err error) {
// 		return io.NopCloser(bytes.NewReader([]byte("{}"))), nil
// 	}

// 	store := testStateBuilder(fileStore)
// 	storageState := NewStorageState()

// 	storageState.lastBlocks["store"] = 12
// 	storageState.initialBlocks["store"] = 10
// 	s := NewSquasher(ctx, storageState, map[string]*state.Store{"store": store}, 10, 7)
// 	assert.Equal(t, 12, s.targetExclusiveEndBlock)

// }

func TestSquash(t *testing.T) {
	writeCount := 0
	var infoBytes []byte

	store := dstore.NewMockStore(nil)
	store.WriteObjectFunc = func(ctx context.Context, base string, f io.Reader) error {
		if base == state.InfoFileName() {
			infoBytes, _ = io.ReadAll(f)
			return nil
		}
		writeCount++
		return nil
	}

	store.OpenObjectFunc = func(ctx context.Context, name string) (out io.ReadCloser, err error) {
		if name == state.InfoFileName() {
			if infoBytes == nil {
				return nil, dstore.ErrNotFound
			}
			return io.NopCloser(bytes.NewReader(infoBytes)), nil
		}
		if name == "0000020000-0000010000.kv" || name == "0000030000-0000020000.partial" {
			return io.NopCloser(bytes.NewReader([]byte("{}"))), nil
		}
		return nil, fmt.Errorf("file %q not mocked", name)
	}

	planner := &JobsPlanner{AvailableJobs: make(chan *Job, 100)}

	s := testStateBuilder(store)
	squashable := NewStoreSquasher(s, 80_000, 10_000, planner)
	go squashable.launch(context.Background())

	require.NoError(t, squashable.squash([]*block.Range{{20_000, 30_000}}))
	require.Equal(t, 0, writeCount)

	require.NoError(t, squashable.squash([]*block.Range{{70_000, 80_000}}))
	require.Equal(t, 0, writeCount)

	require.NoError(t, squashable.squash([]*block.Range{{10_000, 20_000}}))

	squashable.Shutdown(nil)
	require.Equal(t, 2, writeCount) //both [10_000,20_000) and [20_000,30_000) will be merged and written
	assert.True(t, planner.completed)
}

func testStateBuilder(store dstore.Store) *state.Store {
	s, _ := state.NewBuilder("test", 10_000, 10_000, "abc", pbsubstreams.Module_KindStore_UPDATE_POLICY_SET, state.OutputValueTypeString, store)
	return s
}

// func TestConcurrentSquasherClose(t *testing.T) {
// 	var writeLock sync.RWMutex
// 	var infoBytes []byte
// 	var writeCount int

// 	store := dstore.NewMockStore(nil)
// 	store.WriteObjectFunc = func(ctx context.Context, base string, f io.Reader) error {
// 		writeLock.Lock()
// 		defer writeLock.Unlock()

// 		if base == state.InfoFileName() {
// 			infoBytes, _ = io.ReadAll(f)
// 			return nil
// 		}

// 		writeCount++

// 		return nil
// 	}

// 	store.OpenObjectFunc = func(ctx context.Context, name string) (out io.ReadCloser, err error) {
// 		if name == state.InfoFileName() {
// 			writeLock.RLock()
// 			defer writeLock.RUnlock()
// 			if infoBytes == nil {
// 				return nil, dstore.ErrNotFound
// 			}
// 			return io.NopCloser(bytes.NewReader(infoBytes)), nil
// 		}

// 		return nil, fmt.Errorf("no test my friend")
// 	}

// 	var s1 *Squasher
// 	s1 = &Squasher{
// 		storeSquashers: map[string]*StoreSquasher{
// 			"testBuilder": &StoreSquasher{
// 				builder: testStateBuilder(store),
// 				ranges:  []*block.Range{},
// 			},
// 		},
// 		storeSaveInterval: 10_000,
// 		notifier:          nil,
// 	}

// 	var s2 *Squasher
// 	s2 = &Squasher{
// 		storeSquashers: map[string]*StoreSquasher{
// 			"testBuilder": &StoreSquasher{
// 				builder: testStateBuilder(store),
// 				ranges:  []*block.Range{},
// 			},
// 		},
// 		storeSaveInterval: 10_000,
// 		notifier:          nil,
// 	}

// 	var errClose1 error
// 	var errClose2 error

// 	wg := sync.WaitGroup{}
// 	wg.Add(2)

// 	go func() {
// 		defer wg.Done()
// 		err := s1.Squash(context.Background(), "testBuilder", &block.Range{StartBlock: 10_000, ExclusiveEndBlock: 20_000})
// 		if err != nil {
// 			t.Fail()
// 		}
// 		errClose1 = s1.StoresReady()
// 	}()

// 	go func() {
// 		defer wg.Done()
// 		err := s2.Squash(context.Background(), "testBuilder", &block.Range{StartBlock: 10_000, ExclusiveEndBlock: 20_000})
// 		if err != nil {
// 			t.Fail()
// 		}
// 		errClose2 = s2.StoresReady()
// 	}()

// 	wg.Wait()

// 	require.Nil(t, errClose1)
// 	require.Nil(t, errClose2)
// 	require.Equal(t, 2, writeCount)
// }

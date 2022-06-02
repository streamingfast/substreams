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
// 	assert.Equal(t, 12, s.targetExclusiveBlock)

// }

func TestComputeExclusiveEndBlock(t *testing.T) {
	tests := []struct {
		name        string
		lastSaved   int
		target      int
		expect      int
		expectPanic bool
	}{
		{
			name:      "target equal to last saved, on bound",
			lastSaved: 90,
			target:    90,
			expect:    90,
		},
		{
			name:      "target later than last saved, on bound",
			lastSaved: 100,
			target:    90,
			expect:    90,
		},
		{
			name:      "target later than last saved, off bound",
			lastSaved: 100,
			target:    91,
			expect:    90,
		},
		{
			name:      "target later than last saved, off bound",
			lastSaved: 100,
			target:    91,
			expect:    90,
		},
		{
			name:      "target prior to last saved, on bound",
			lastSaved: 80,
			target:    90,
			expect:    80,
		},
		{
			name:      "target prior to last saved, off bound",
			lastSaved: 80,
			target:    92,
			expect:    80,
		},
		{
			name:      "nothing saved, target off bound",
			lastSaved: 0,
			target:    92,
			expect:    0,
		},
		{
			name:      "nothing saved, target on bound",
			lastSaved: 0,
			target:    80,
			expect:    0,
		},
		{
			name:        "target after last saved, but below init block",
			lastSaved:   20,
			target:      40,
			expectPanic: true,
		},
		{
			name:        "target before last saved, but below init block",
			lastSaved:   30,
			target:      10,
			expectPanic: true,
		},
	}
	moduleInitBlock := 50
	saveInterval := 10

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var res uint64
			f := func() {
				res = ComputeStoreExclusiveEndBlock(uint64(test.lastSaved), uint64(test.target), uint64(saveInterval), uint64(moduleInitBlock))
			}
			if test.expectPanic {
				assert.Panics(t, f)
			} else {
				f()
				assert.Equal(t, test.expect, int(res))
			}
		})
	}
}

func TestSquash(t *testing.T) {
	ctx := context.Background()

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

	s := testStateBuilder(store)
	squashable := NewSquashable(s, 80_000, 10_000, 10_000)

	notificationsSent := 0
	notifierFunc := NotifierFunc(func() {
		notificationsSent++
	})

	require.NoError(t, squashable.squash(ctx, &block.Range{StartBlock: 20_000, ExclusiveEndBlock: 30_000}, notifierFunc))
	require.Equal(t, 0, writeCount)

	require.NoError(t, squashable.squash(ctx, &block.Range{StartBlock: 70_000, ExclusiveEndBlock: 80_000}, notifierFunc))
	require.Equal(t, 0, writeCount)

	require.NoError(t, squashable.squash(ctx, &block.Range{StartBlock: 10_000, ExclusiveEndBlock: 20_000}, notifierFunc))

	require.Equal(t, 2, writeCount) //both [10_000,20_000) and [20_000,30_000) will be merged and written
	require.Equal(t, 2, notificationsSent)
}

func testStateBuilder(store dstore.Store) *state.Store {
	return &state.Store{
		Name:               "testBuilder",
		SaveInterval:       10_000,
		ModuleInitialBlock: 10_000,
		StoreInitialBlock:  10_000,
		Store:              store,
		ModuleHash:         "abc",
		KV:                 map[string][]byte{},
		UpdatePolicy:       pbsubstreams.Module_KindStore_UPDATE_POLICY_SET,
		ValueType:          state.OutputValueTypeString,
	}
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
// 		squashables: map[string]*Squashable{
// 			"testBuilder": &Squashable{
// 				builder: testStateBuilder(store),
// 				ranges:  []*block.Range{},
// 			},
// 		},
// 		storeSaveInterval: 10_000,
// 		notifier:          nil,
// 	}

// 	var s2 *Squasher
// 	s2 = &Squasher{
// 		squashables: map[string]*Squashable{
// 			"testBuilder": &Squashable{
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

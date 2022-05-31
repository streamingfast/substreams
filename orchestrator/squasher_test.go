package orchestrator

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"sync"
	"testing"

	"github.com/streamingfast/dstore"
	"github.com/streamingfast/substreams/block"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/state"
	"github.com/stretchr/testify/require"
)

type NotifierFunc func()

func (n NotifierFunc) Notify(builder string, blockNum uint64) {
	n()
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
		return nil, fmt.Errorf("no")
	}

	squashable := &Squashable{
		builder: testStateBuilder(store),
		ranges:  []*block.Range{},
	}

	notificationsSent := 0
	notifierFunc := NotifierFunc(func() {
		notificationsSent++
	})

	err := squash(ctx, squashable, &block.Range{StartBlock: 20_000, ExclusiveEndBlock: 30_000}, notifierFunc)
	require.Nil(t, err)
	require.Equal(t, 0, writeCount)

	err = squash(ctx, squashable, &block.Range{StartBlock: 70_000, ExclusiveEndBlock: 80_000}, notifierFunc)
	require.Nil(t, err)
	require.Equal(t, 0, writeCount)

	err = squash(ctx, squashable, &block.Range{StartBlock: 10_000, ExclusiveEndBlock: 20_000}, notifierFunc)
	require.Nil(t, err)

	require.Equal(t, 2, writeCount) //both [10_000,20_000) and [20_000,30_000) will be merged and written
	require.Equal(t, 2, notificationsSent)
}

func testStateBuilder(store dstore.Store) *state.Store {
	return &state.Store{
		Name:             "testBuilder",
		SaveInterval:     10_000,
		ModuleInitialBlock: 0,
		Store:            store,
		ModuleHash:       "abc",
		KV:               map[string][]byte{},
		PartialMode:      false,
		BlockRange: &block.Range{
			StartBlock:        0,
			ExclusiveEndBlock: 10_000,
		},
		UpdatePolicy: pbsubstreams.Module_KindStore_UPDATE_POLICY_SET,
		ValueType:    state.OutputValueTypeString,
	}
}

func TestConcurrentSquasherClose(t *testing.T) {
	var writeLock sync.RWMutex
	var infoBytes []byte
	var writeCount int

	store := dstore.NewMockStore(nil)
	store.WriteObjectFunc = func(ctx context.Context, base string, f io.Reader) error {
		writeLock.Lock()
		defer writeLock.Unlock()

		if base == state.InfoFileName() {
			infoBytes, _ = io.ReadAll(f)
			return nil
		}

		writeCount++

		return nil
	}

	store.OpenObjectFunc = func(ctx context.Context, name string) (out io.ReadCloser, err error) {
		if name == state.InfoFileName() {
			writeLock.RLock()
			defer writeLock.RUnlock()
			if infoBytes == nil {
				return nil, dstore.ErrNotFound
			}
			return io.NopCloser(bytes.NewReader(infoBytes)), nil
		}

		return nil, fmt.Errorf("no")
	}

	var s1 *Squasher
	s1 = &Squasher{
		squashables: map[string]*Squashable{
			"testBuilder": &Squashable{
				builder: testStateBuilder(store),
				ranges:  []*block.Range{},
			},
		},
		storeSaveInterval: 10_000,
		notifier:          nil,
	}

	var s2 *Squasher
	s2 = &Squasher{
		squashables: map[string]*Squashable{
			"testBuilder": &Squashable{
				builder: testStateBuilder(store),
				ranges:  []*block.Range{},
			},
		},
		storeSaveInterval: 10_000,
		notifier:          nil,
	}

	var errClose1 error
	var errClose2 error

	wg := sync.WaitGroup{}
	wg.Add(2)

	go func() {
		defer wg.Done()
		err := s1.Squash(context.Background(), "testBuilder", &block.Range{StartBlock: 10_000, ExclusiveEndBlock: 20_000})
		if err != nil {
			t.Fail()
		}
		errClose1 = s1.Close()
	}()

	go func() {
		defer wg.Done()
		err := s2.Squash(context.Background(), "testBuilder", &block.Range{StartBlock: 10_000, ExclusiveEndBlock: 20_000})
		if err != nil {
			t.Fail()
		}
		errClose2 = s2.Close()
	}()

	wg.Wait()

	require.Nil(t, errClose1)
	require.Nil(t, errClose2)
	require.Equal(t, 2, writeCount)
}

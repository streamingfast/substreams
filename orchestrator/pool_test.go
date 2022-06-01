package orchestrator

import (
	"context"
	"io"
	"testing"
	"time"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/stretchr/testify/require"
)

type TestWaiter struct {
	ch      chan interface{}
	counter *int
}

func NewTestWaiter(counter *int) *TestWaiter {
	return &TestWaiter{
		ch:      make(chan interface{}),
		counter: counter,
	}
}

func (tw *TestWaiter) Wait(ctx context.Context) <-chan interface{} {
	return tw.ch
}

func (tw *TestWaiter) Signal(storeName string, blockNum uint64) {
	close(tw.ch)
	*tw.counter++
}

func (tw *TestWaiter) Order() int {
	return 0
}

func (tw *TestWaiter) String() string {
	return ""
}

func TestNotify(t *testing.T) {
	p := NewRequestPool()
	ctx := context.Background()

	signalCounter := new(int)

	_ = p.Add(ctx, &pbsubstreams.Request{}, NewTestWaiter(signalCounter))
	_ = p.Add(ctx, &pbsubstreams.Request{}, NewTestWaiter(signalCounter))

	p.Notify("", 0)
	require.Equal(t, 2, *signalCounter)
}

func TestGet(t *testing.T) {
	p := NewRequestPool()
	ctx := context.Background()

	storageState := &StorageState{lastBlocks: map[string]uint64{
		"test1": 0,
		"test2": 3000,
	}}

	waiter0 := NewWaiter(200, storageState,
		&pbsubstreams.Module{Name: "test1"},
	)
	r0 := &pbsubstreams.Request{
		StartBlockNum: 200,
		StopBlockNum:  300,
		Modules:       &pbsubstreams.Modules{Modules: []*pbsubstreams.Module{{Name: "test1_descendant"}}},
	}
	_ = p.Add(ctx, r0, waiter0)

	waiter1 := NewWaiter(300, storageState,
		&pbsubstreams.Module{Name: "test1"},
		&pbsubstreams.Module{Name: "test2"},
	)
	r1 := &pbsubstreams.Request{
		StartBlockNum: 300,
		StopBlockNum:  400,
		Modules:       &pbsubstreams.Modules{Modules: []*pbsubstreams.Module{{Name: "test2_test3_descendant"}}},
	}
	_ = p.Add(ctx, r1, waiter1)

	p.Notify("test1", 200)

	p.Start(ctx)

	r, err := p.Get(ctx)
	require.Nil(t, err)
	require.NotNil(t, r)
	require.Equal(t, r0, r)

	shortContext, cancel := context.WithTimeout(ctx, 10*time.Millisecond)
	r, err = p.Get(shortContext)
	require.Equal(t, context.DeadlineExceeded, err) //expected for this test
	require.Nil(t, r)
	cancel()

	p.Notify("test1", 300)

	r, err = p.Get(ctx)
	require.Nil(t, err)
	require.NotNil(t, r)
	require.Equal(t, r1, r)

	r, err = p.Get(ctx)
	require.NotNil(t, err)
	require.Equal(t, io.EOF, err)
	require.Nil(t, r)
}

func TestGetOrdered(t *testing.T) {
	p := NewRequestPool()
	ctx := context.Background()

	waiter0 := NewWaiter(100, NewStorageState())
	r0 := &pbsubstreams.Request{
		StartBlockNum: 100,
		StopBlockNum:  200,
		Modules:       &pbsubstreams.Modules{Modules: []*pbsubstreams.Module{{Name: "A"}}},
	}
	_ = p.Add(ctx, r0, waiter0)

	waiter1 := NewWaiter(200, NewStorageState())
	r1 := &pbsubstreams.Request{
		StartBlockNum: 200,
		StopBlockNum:  300,
		Modules:       &pbsubstreams.Modules{Modules: []*pbsubstreams.Module{{Name: "A"}}},
	}
	_ = p.Add(ctx, r1, waiter1)

	waiter2 := NewWaiter(100, NewStorageState(), &pbsubstreams.Module{Name: "A"})
	r2 := &pbsubstreams.Request{
		StartBlockNum: 100,
		StopBlockNum:  200,
		Modules:       &pbsubstreams.Modules{Modules: []*pbsubstreams.Module{{Name: "B"}}},
	}
	_ = p.Add(ctx, r2, waiter2)

	p.Start(ctx)

	// first request will be for A, since they have no dependencies and are ready right away.
	r, err := p.Get(ctx)
	require.Nil(t, err)
	require.NotNil(t, r)
	require.Equal(t, "A", r.Modules.Modules[0].Name)

	// we notify that A is ready up to block 100, which will put the request for B to the front of the queue
	p.Notify("A", 100)
	time.Sleep(100 * time.Microsecond) // give it a teeny bit of time for notification to get processed

	// assert that the request for B got put ahead of the request for A
	r, err = p.Get(ctx)
	require.Nil(t, err)
	require.NotNil(t, r)
	require.Equal(t, "B", r.Modules.Modules[0].Name)

	// assert that the remaining request is there
	r, err = p.Get(ctx)
	require.Nil(t, err)
	require.NotNil(t, r)
	require.Equal(t, "A", r.Modules.Modules[0].Name)

	// asser the end of the stream
	r, err = p.Get(ctx)
	require.NotNil(t, err)
	require.Equal(t, io.EOF, err)
	require.Nil(t, r)
}

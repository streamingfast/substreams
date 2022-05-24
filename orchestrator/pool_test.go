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

func TestNotify(t *testing.T) {
	p := NewPool()
	ctx := context.Background()

	signalCounter := new(int)

	_ = p.Add(ctx, &pbsubstreams.Request{}, NewTestWaiter(signalCounter))
	_ = p.Add(ctx, &pbsubstreams.Request{}, NewTestWaiter(signalCounter))

	p.Notify("", 0)
	require.Equal(t, 2, *signalCounter)
}

func TestGet(t *testing.T) {
	p := NewPool()
	ctx := context.Background()

	lastSavedBlockMap := map[string]uint64{
		"test1": 0,
		"test2": 3000,
	}

	waiter0 := NewWaiter(200, lastSavedBlockMap,
		&pbsubstreams.Module{Name: "test1"},
	)
	r0 := &pbsubstreams.Request{
		StartBlockNum: 200,
		StopBlockNum:  300,
		Modules:       &pbsubstreams.Modules{Modules: []*pbsubstreams.Module{{Name: "test1_descendant"}}},
	}
	_ = p.Add(ctx, r0, waiter0)

	waiter1 := NewWaiter(300, lastSavedBlockMap,
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

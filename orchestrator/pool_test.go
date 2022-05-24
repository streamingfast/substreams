package orchestrator

import (
	"context"
	"io"
	"testing"

	"github.com/streamingfast/substreams/block"
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
	p := &Pool{}
	ctx := context.Background()

	signalCounter := new(int)

	_ = p.Add(ctx, &pbsubstreams.Request{}, NewTestWaiter(signalCounter))
	_ = p.Add(ctx, &pbsubstreams.Request{}, NewTestWaiter(signalCounter))

	p.Notify("", 0)
	require.Equal(t, 2, *signalCounter)
}

func TestGet(t *testing.T) {
	p := &Pool{}
	ctx := context.Background()

	waiter := NewWaiter(&block.Range{
		StartBlock:        100,
		ExclusiveEndBlock: 200,
	}, &pbsubstreams.Module{
		Name: "test1",
	})

	_ = p.Add(ctx, &pbsubstreams.Request{}, waiter)

	p.Notify("popo", 5000)
	p.Notify("test1", 100)

	r, err := p.Get(ctx)
	require.Nil(t, err)
	require.NotNil(t, r)

	r, err = p.Get(ctx)
	require.NotNil(t, err)
	require.Equal(t, io.EOF, err)
	require.Nil(t, r)
}

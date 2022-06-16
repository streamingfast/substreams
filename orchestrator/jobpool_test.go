package orchestrator

import (
	"context"
	"io"
	"testing"
	"time"

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

func (tw *TestWaiter) Size() int {
	return 0
}

func (tw *TestWaiter) String() string {
	return ""
}

func TestNotify(t *testing.T) {
	p := NewJobPool()
	ctx := context.Background()

	signalCounter := new(int)

	_ = p.Add(ctx, 0, &Job{}, NewTestWaiter(signalCounter))
	_ = p.Add(ctx, 0, &Job{}, NewTestWaiter(signalCounter))

	p.Notify("", 0)
	require.Equal(t, 2, *signalCounter)
}

func TestGet(t *testing.T) {
	t.Skip("fixme")
	p := NewJobPool()
	ctx := context.Background()

	waiter0 := NewWaiter(200,
		&pbsubstreams.Module{Name: "test1"},
	)
	r0 := &Job{
		moduleName:   "test1_descendant",
		requestRange: block.NewRange(200, 300),
	}
	_ = p.Add(ctx, 0, r0, waiter0)

	waiter1 := NewWaiter(300,
		&pbsubstreams.Module{Name: "test1"},
		&pbsubstreams.Module{Name: "test2"},
	)
	r1 := &Job{
		moduleName:   "test2_test3_descendant",
		requestRange: block.NewRange(300, 400),
	}
	_ = p.Add(ctx, 0, r1, waiter1)

	p.Notify("test1", 200)

	p.Start(ctx)

	r, err := p.GetNext(ctx)
	require.Nil(t, err)
	require.NotNil(t, r)
	require.Equal(t, r0, r)

	shortContext, cancel := context.WithTimeout(ctx, 10*time.Millisecond)
	r, err = p.GetNext(shortContext)
	require.Equal(t, context.DeadlineExceeded, err) //expected for this test
	require.Nil(t, r)
	cancel()

	p.Notify("test1", 300)

	r, err = p.GetNext(ctx)
	require.Nil(t, err)
	require.NotNil(t, r)
	require.Equal(t, r1, r)

	r, err = p.GetNext(ctx)
	require.NotNil(t, err)
	require.Equal(t, io.EOF, err)
	require.Nil(t, r)
}

func TestGetOrdered(t *testing.T) {
	p := NewJobPool()
	ctx := context.Background()

	waiter0 := NewWaiter(100)
	r0 := &Job{
		moduleName:   "A",
		requestRange: block.NewRange(100, 200),
	}
	_ = p.Add(ctx, 0, r0, waiter0)

	waiter1 := NewWaiter(200)
	r1 := &Job{
		moduleName:   "A",
		requestRange: block.NewRange(200, 300),
	}
	_ = p.Add(ctx, 0, r1, waiter1)

	waiter2 := NewWaiter(100, &pbsubstreams.Module{Name: "A"})
	r2 := &Job{
		moduleName:   "B",
		requestRange: block.NewRange(100, 200),
	}
	_ = p.Add(ctx, 0, r2, waiter2)

	p.Start(ctx)

	// first request will be for A, since they have no dependencies and are ready right away.
	r, err := p.GetNext(ctx)
	require.Nil(t, err)
	require.NotNil(t, r)
	require.Equal(t, "A", r.moduleName)

	// we notify that A is ready up to block 100, which will put the request for B to the front of the queue
	p.Notify("A", 100)

	// NOTE:
	// if this test ever fails, it is almost certainly because some cpu race is happening here, and that the getNext below
	// is getting called before the Notify call above is not done being processed.  we try to give it enough time, without slowing down
	// the testing process too much.
	time.Sleep(500 * time.Millisecond) // give it a teeny bit of time for notification to get processed.

	// assert that the request for B got put ahead of the request for A
	r, err = p.GetNext(ctx)
	require.Nil(t, err)
	require.NotNil(t, r)
	require.Equal(t, "B", r.moduleName)

	// assert that the remaining request is there
	r, err = p.GetNext(ctx)
	require.Nil(t, err)
	require.NotNil(t, r)
	require.Equal(t, "A", r.moduleName)

	// asser the end of the stream
	r, err = p.GetNext(ctx)
	require.NotNil(t, err)
	require.Equal(t, io.EOF, err)
	require.Nil(t, r)
}

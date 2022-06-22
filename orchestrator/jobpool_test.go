package orchestrator

import (
	"context"
	"fmt"
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

func (tw *TestWaiter) BlockNumber() uint64 {
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

func TestGetOrdered(t *testing.T) {
	p := NewJobPool()
	ctx := context.Background()

	waiter0 := NewWaiter(1, "A", 100)
	r0 := &Job{
		moduleName:   "A",
		requestRange: block.NewRange(100, 200),
	}
	_ = p.Add(ctx, 2, r0, waiter0)

	waiter1 := NewWaiter(2, "A", 200)
	r1 := &Job{
		moduleName:   "A",
		requestRange: block.NewRange(200, 300),
	}
	_ = p.Add(ctx, 1, r1, waiter1)

	waiter2 := NewWaiter(3, "B", 100, &pbsubstreams.Module{Name: "A"})
	r2 := &Job{
		moduleName:   "B",
		requestRange: block.NewRange(100, 200),
	}
	_ = p.Add(ctx, 2, r2, waiter2)

	p.Start(ctx)

	// first request will be for A, since they have no dependencies and are ready right away.
	// it will be for the [100,200) range since this job waiter has a higher reverseIdx
	r, err := p.GetNext(ctx)
	require.Nil(t, err)
	require.NotNil(t, r)
	require.Equal(t, "A", r.moduleName)

	// we notify that A is ready up to block 100, which will put the request for B to the front of the queue
	p.Notify("A", 100)

	// NOTE:
	// if this test ever fails, it is almost certainly because some cpu race is happening here, and that the getNext below
	// is getting called before the Notify call above is done being processed.  we try to give it enough time, without slowing down
	// the testing process too much.
	time.Sleep(500 * time.Millisecond) // give it a teeny bit of time for notification to get processed.

	// assert that the request for B got put ahead of the request for A
	r, err = p.GetNext(ctx)
	require.Nil(t, err)
	require.NotNil(t, r)
	require.Equal(t, "B", r.moduleName)
	require.Equal(t, &block.Range{
		StartBlock:        100,
		ExclusiveEndBlock: 200,
	}, r.requestRange)

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

func TestWIP(t *testing.T) {
	t.Skip("work in progress")
	p := NewJobPool()
	ctx := context.Background()

	stores := []string{"A", "B", "C"}
	blockRanges := []*block.Range{{0, 100}, {100, 200}, {200, 300}}
	getAncestorStores := func(store string) []*pbsubstreams.Module {
		switch store {
		case "C":
			return []*pbsubstreams.Module{
				&pbsubstreams.Module{
					Name: "A",
				},
				&pbsubstreams.Module{
					Name: "B",
				},
			}
		case "B":
			return []*pbsubstreams.Module{
				&pbsubstreams.Module{
					Name: "A",
				},
			}
		default:
			return []*pbsubstreams.Module{}
		}
	}

	for _, store := range stores {
		for ix, blockRange := range blockRanges {
			ancestorStores := getAncestorStores(store)
			waiter := NewWaiter(ix, store, blockRange.StartBlock, ancestorStores...)
			job := &Job{moduleName: store, moduleSaveInterval: 100, requestRange: blockRange}
			p.Add(ctx, len(blockRanges)-ix, job, waiter)
		}
	}

	p.Start(ctx)

	type NextResult struct {
		StoreName string
		Range     *block.Range
	}

	var results []*NextResult
	for {
		job, err := p.GetNext(ctx)
		if err != nil {
			if err == io.EOF {
				break
			} else {
				t.Errorf("error getting next from pool: %s", err.Error())
			}
		}

		results = append(results, &NextResult{
			StoreName: job.moduleName,
			Range:     job.requestRange,
		})

		fmt.Printf("********** got result: store(%s):%s\n", job.moduleName, job.requestRange.String())
		fmt.Printf("notifying that %s is done at %d\n", job.moduleName, job.requestRange.ExclusiveEndBlock)
		p.Notify(job.moduleName, job.requestRange.ExclusiveEndBlock)
		time.Sleep(50 * time.Microsecond)
	}

	for i, r := range results {
		fmt.Printf("result %d: store(%s):%s\n", i, r.StoreName, r.Range.String())
	}
}

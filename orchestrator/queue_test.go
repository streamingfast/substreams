package orchestrator

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestQueue(t *testing.T) {
	in := make(chan *QueueItem)
	out := make(chan *QueueItem)
	ctx := context.Background()

	StartQueue(ctx, in, out)

	r1 := &Job{}
	r2 := &Job{}
	r3 := &Job{}

	wg := sync.WaitGroup{}
	wg.Add(3)

	go func() {
		defer wg.Done()
		in <- &QueueItem{
			job:      r1,
			Priority: 1,
		}
	}()

	go func() {
		defer wg.Done()
		//highest priority item
		in <- &QueueItem{
			job:      r2,
			Priority: 2,
		}
	}()

	go func() {
		defer wg.Done()
		in <- &QueueItem{
			job:      r3,
			Priority: 1,
		}
	}()

	wg.Wait()

	close(in)

	var pops []*Job
	for r := range out {
		pops = append(pops, r.job)
	}

	expected := []*Job{r2, r1, r3}
	require.Equal(t, expected, pops)
}

func TestQueueLoadTest(t *testing.T) {
	t.Skip()

	in := make(chan *QueueItem, 5_000)
	out := make(chan *QueueItem)
	ctx := context.Background()

	StartQueue(ctx, in, out)

	wg := sync.WaitGroup{}

	n := 1_000_000

	wg.Add(n)

	go func() {
		for i := 0; i < n; i++ {
			go func() {
				defer wg.Done()
				select {
				case <-ctx.Done():
					return
				case in <- &QueueItem{
					job:      &Job{},
					Priority: 0,
				}:
					return
				}
			}()
		}
	}()

	go func() {
		wg.Wait()
		close(in)
	}()

	var resultCount int
	for range out {
		resultCount++
	}

	require.Equal(t, n, resultCount)
}

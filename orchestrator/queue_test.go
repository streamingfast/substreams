package orchestrator

import (
	"context"
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

	in <- &QueueItem{
		job:      r1,
		Priority: 1,
	}

	//highest priority item
	in <- &QueueItem{
		job:      r2,
		Priority: 2,
	}

	in <- &QueueItem{
		job:      r3,
		Priority: 1,
	}

	close(in)

	var pops []*Job
	for r := range out {
		pops = append(pops, r.job)
	}

	expected := []*Job{r2, r1, r3}
	require.Equal(t, expected, pops)
}

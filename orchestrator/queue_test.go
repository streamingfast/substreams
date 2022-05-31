package orchestrator

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

func TestQueue(t *testing.T) {
	in := make(chan *QueueItem)
	out := make(chan *QueueItem)
	ctx := context.Background()

	StartQueue(ctx, in, out)

	r1 := &pbsubstreams.Request{}
	r2 := &pbsubstreams.Request{}
	r3 := &pbsubstreams.Request{}

	in <- &QueueItem{
		Request:  r1,
		Priority: 1,
	}

	//highest priority item
	in <- &QueueItem{
		Request:  r2,
		Priority: 2,
	}

	in <- &QueueItem{
		Request:  r3,
		Priority: 1,
	}

	close(in)

	var pops []*pbsubstreams.Request
	for r := range out {
		pops = append(pops, r.Request)
	}

	expected := []*pbsubstreams.Request{r2, r1, r3}
	require.Equal(t, expected, pops)
}

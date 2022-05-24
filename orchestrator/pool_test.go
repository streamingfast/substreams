package orchestrator

import (
	"context"
	"github.com/streamingfast/substreams/block"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"testing"
)

func TestName(t *testing.T) {
	p := &Pool{}

	ctx := context.Background()

	p.Add(ctx, &PoolItem{
		Request: &pbsubstreams.Request{},
		Waiter: NewWaiter(&block.Range{
			StartBlock:        100,
			ExclusiveEndBlock: 200,
		}, &pbsubstreams.Module{
			Name: "test1",
		}),
	})

	p.Notify("test1", 100)

}

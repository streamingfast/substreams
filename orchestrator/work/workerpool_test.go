package work

import (
	"context"
	"testing"

	"github.com/streamingfast/substreams"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

func Test_workerPoolPool_Borrow_Return(t *testing.T) {
	ctx := context.Background()
	pi := NewWorkerPool(ctx, 2, func(logger *zap.Logger) Worker {
		return NewWorkerFactoryFromFunc(func(ctx context.Context, request *pbsubstreams.Request, respFunc substreams.ResponseFunc) *Result {
			return &Result{}
		})
	})
	p := pi.(*workerPool)

	assert.Len(t, p.workers, 2)
	workerPool := p.Borrow(ctx)
	assert.Len(t, p.workers, 1)
	p.Return(workerPool)
	assert.Len(t, p.workers, 2)
}

func Test_workerPoolPool_Borrow_Return_Canceled_Ctx(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	pi := NewWorkerPool(ctx, 1, func(logger *zap.Logger) Worker {
		return NewWorkerFactoryFromFunc(func(ctx context.Context, request *pbsubstreams.Request, respFunc substreams.ResponseFunc) *Result {
			return &Result{}
		})
	})
	p := pi.(*workerPool)

	assert.Len(t, p.workers, 1)
	<-p.workers
	assert.Len(t, p.workers, 0)
	workerPool := p.Borrow(ctx)
	assert.Nil(t, workerPool)

}

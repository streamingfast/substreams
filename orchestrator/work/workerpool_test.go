package work

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"

	"github.com/streamingfast/substreams"
	pbssinternal "github.com/streamingfast/substreams/pb/sf/substreams/intern/v2"
)

func Test_workerPoolPool_Borrow_Return(t *testing.T) {
	ctx := context.Background()
	pi := NewWorkerPool(ctx, 2, func(logger *zap.Logger) Worker {
		return NewWorkerFactoryFromFunc(func(ctx context.Context, request *pbssinternal.ProcessRangeRequest, respFunc substreams.ResponseFunc) *Result {
			return &Result{}
		})
	})
	p := pi.(*WorkerPool)

	assert.Len(t, p.workers, 2)
	workerPool := p.Borrow()
	assert.Len(t, p.workers, 1)
	p.Return(workerPool)
	assert.Len(t, p.workers, 2)
}

func Test_workerPoolPool_Borrow_Return_Canceled_Ctx(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	pi := NewWorkerPool(ctx, 1, func(logger *zap.Logger) Worker {
		return NewWorkerFactoryFromFunc(func(ctx context.Context, request *pbssinternal.ProcessRangeRequest, respFunc substreams.ResponseFunc) *Result {
			return &Result{}
		})
	})
	p := pi.(*WorkerPool)

	assert.Len(t, p.workers, 1)
	<-p.workers
	assert.Len(t, p.workers, 0)
	workerPool := p.Borrow()
	assert.Nil(t, workerPool)

}

package work

import (
	"context"

	"github.com/streamingfast/substreams/reqctx"
	"go.uber.org/zap"
)

type WorkerPool interface {
	Borrow(context.Context) Worker
	Return(Worker)
}

var _ WorkerPool = (*workerPool)(nil)

type workerPool struct {
	workers chan Worker
}

func NewWorkerPool(ctx context.Context, workerCount uint64, workerFactory WorkerFactory) WorkerPool {
	logger := reqctx.Logger(ctx)

	logger.Info("initializing worker pool", zap.Uint64("worker_count", workerCount))
	workers := make(chan Worker, workerCount)
	for i := uint64(0); i < workerCount; i++ {
		workers <- workerFactory(logger)
	}

	return &workerPool{
		workers: workers,
	}

}

func (p *workerPool) Borrow(ctx context.Context) Worker {
	select {
	case <-ctx.Done():
		return nil
	case w := <-p.workers:
		return w
	}
}

func (p *workerPool) Return(worker Worker) {
	p.workers <- worker
}

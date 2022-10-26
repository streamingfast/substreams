package work

import (
	"context"
	"github.com/streamingfast/substreams/reqctx"
	"go.uber.org/zap"
)

type JobRunnerPool interface {
	Borrow() JobRunner
	Return(JobRunner)
}

type jobRunnerPool struct {
	workers chan JobRunner
}

func NewJobRunnerPool(ctx context.Context, workerCount uint64, newWorkerFunc WorkerFactory) *jobRunnerPool {
	logger := reqctx.Logger(ctx)
	logger.Info("initiating worker pool", zap.Uint64("worker_count", workerCount))
	workers := make(chan JobRunner, workerCount)
	for i := uint64(0); i < workerCount; i++ {
		workers <- newWorkerFunc(logger)
	}
	workerPool := &jobRunnerPool{
		workers: workers,
	}
	return workerPool
}

func (p *jobRunnerPool) Borrow() JobRunner {
	w := <-p.workers
	return w
}

func (p *jobRunnerPool) Return(worker JobRunner) {
	p.workers <- worker
}

type RetryableErr struct {
	cause error
}

func (r *RetryableErr) Error() string {
	return r.cause.Error()
}

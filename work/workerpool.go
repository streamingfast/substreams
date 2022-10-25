package work

import (
	"go.uber.org/zap"
)

// TODO(abourget): JobRunner instead of Worker everywhere here.

type JobRunnerPool interface {
	Borrow() JobRunner
	Return(JobRunner)
}

type WorkerPool struct {
	workers chan JobRunner
}

func NewWorkerPool(workerCount uint64, newWorkerFunc WorkerFactory, logger *zap.Logger) *WorkerPool {
	logger.Info("initiating worker pool", zap.Uint64("worker_count", workerCount))
	workers := make(chan JobRunner, workerCount)
	for i := uint64(0); i < workerCount; i++ {
		workers <- newWorkerFunc(logger)
	}

	workerPool := &WorkerPool{
		workers: workers,
	}

	return workerPool
}

func (p *WorkerPool) Borrow() JobRunner {
	w := <-p.workers
	return w
}

func (p *WorkerPool) Return(worker JobRunner) {
	p.workers <- worker
}

type RetryableErr struct {
	cause error
}

func (r *RetryableErr) Error() string {
	return r.cause.Error()
}

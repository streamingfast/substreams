package work

import (
	"go.uber.org/zap"
)

type WorkerPool struct {
	workers chan Worker
}

func NewWorkerPool(workerCount uint64, newWorkerFunc WorkerFactory, logger *zap.Logger) *WorkerPool {
	logger.Info("initiating worker pool", zap.Uint64("worker_count", workerCount))
	workers := make(chan Worker, workerCount)
	for i := uint64(0); i < workerCount; i++ {
		workers <- newWorkerFunc(logger)
	}

	workerPool := &WorkerPool{
		workers: workers,
	}

	return workerPool
}

func (p *WorkerPool) Borrow() Worker {
	w := <-p.workers
	return w
}

func (p *WorkerPool) ReturnWorker(worker Worker) {
	p.workers <- worker
}

type RetryableErr struct {
	cause error
}

func (r *RetryableErr) Error() string {
	return r.cause.Error()
}

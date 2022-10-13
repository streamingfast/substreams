package orchestrator

import (
	"go.uber.org/zap"
)

type WorkerPool struct {
	workers chan Worker
}

func NewWorkerPool(workerCount int, newWorkerFunc WorkerFactory) *WorkerPool {
	zlog.Info("initiating worker pool", zap.Int("worker_count", workerCount))
	workers := make(chan Worker, workerCount)
	for i := 0; i < workerCount; i++ {
		workers <- newWorkerFunc()
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

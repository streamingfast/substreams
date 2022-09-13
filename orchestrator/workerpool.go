package orchestrator

import (
	"go.opentelemetry.io/otel"
	"go.uber.org/zap"
)

type WorkerPool struct {
	workers chan Worker
}

func NewWorkerPool(workerCount int, newWorkerFunc NewWorkerFunc) *WorkerPool {
	zlog.Info("initiating worker pool", zap.Int("worker_count", workerCount))
	tracer := otel.GetTracerProvider().Tracer("worker")
	workers := make(chan Worker, workerCount)
	for i := 0; i < workerCount; i++ {
		workers <- newWorkerFunc(tracer)
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

package work

import (
	"context"

	"go.uber.org/zap"

	"github.com/streamingfast/substreams/reqctx"
)

type WorkerPool struct {
	workers []*WorkerStatus
}

type WorkerState int

const (
	WorkerFree WorkerState = iota
	WorkerWorking
)

type WorkerStatus struct {
	State  WorkerState
	Worker Worker
}

func NewWorkerPool(ctx context.Context, workerCount int, workerFactory WorkerFactory) *WorkerPool {
	logger := reqctx.Logger(ctx)

	logger.Info("initializing worker pool", zap.Int("worker_count", workerCount))

	workers := make([]*WorkerStatus, workerCount)
	for i := 0; i < workerCount; i++ {
		workers[i] = &WorkerStatus{
			Worker: workerFactory(logger),
			State:  WorkerFree,
		}
	}

	return &WorkerPool{
		workers: workers,
	}
}

func (p *WorkerPool) WorkerAvailable() bool {
	for _, w := range p.workers {
		if w.State == WorkerFree {
			return true
		}
	}
	return false
}

//func (p *WorkerPool) FreeWorkers() int {
//	count := 0
//	for _, w := range p.workers {
//		if w.State == WorkerFree {
//			count++
//		}
//	}
//	return count
//}

func (p *WorkerPool) Borrow() Worker {
	for _, status := range p.workers {
		if status.State == WorkerFree {
			status.State = WorkerWorking
			return status.Worker
		}
	}
	panic("no free workers, call WorkerAvailable() first")
}

func (p *WorkerPool) Return(worker Worker) {
	for _, status := range p.workers {
		if status.Worker == worker {
			status.State = WorkerFree
			return
		}
	}
}

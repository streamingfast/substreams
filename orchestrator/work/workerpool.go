package work

import (
	"context"
	"time"

	"go.uber.org/zap"

	"github.com/streamingfast/substreams/reqctx"
)

type WorkerPool struct {
	workers []*WorkerStatus
	started *time.Time
}

type WorkerState int

const (
	WorkerFree WorkerState = iota
	WorkerWorking
	WorkerInitialWait
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
		state := WorkerFree
		if i > 0 {
			state = WorkerInitialWait
		}
		workers[i] = &WorkerStatus{
			Worker: workerFactory(logger),
			State:  state,
		}
	}

	now := time.Now()
	return &WorkerPool{
		workers: workers,
		started: &now,
	}
}

func (p *WorkerPool) rampupWorkers() {
	if p.started == nil || time.Since(*p.started) < time.Second*4 {
		return
	}
	for _, w := range p.workers {
		if w.State == WorkerInitialWait {
			w.State = WorkerFree
		}
	}
	p.started = nil
}

func (p *WorkerPool) WorkerAvailable() bool {
	p.rampupWorkers()
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
			if status.State != WorkerWorking {
				panic("returned worker was already free")
			}
			status.State = WorkerFree
			return
		}
	}
}

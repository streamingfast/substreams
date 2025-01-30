package work

import (
	"context"
	"time"

	"github.com/streamingfast/substreams/client"
	"github.com/streamingfast/substreams/reqctx"
	"go.uber.org/zap"
)

type SimpleWorkerPool struct {
	freeWorker        int
	startedAt         time.Time
	firstWorkerServed bool
	clientFactory     client.InternalClientFactory
	logger            *zap.Logger
}

type WorkerState int

func NewSimpleWorkerPool(ctx context.Context, workerCount int, clientFactory client.InternalClientFactory) *SimpleWorkerPool {
	logger := reqctx.Logger(ctx)

	logger.Debug("initializing worker pool", zap.Int("worker_count", workerCount))

	return &SimpleWorkerPool{
		freeWorker:    workerCount,
		startedAt:     time.Now(),
		clientFactory: clientFactory,
		logger:        logger,
	}
}

func (p *SimpleWorkerPool) Borrow(_ context.Context) (Worker, error) {
	rampUpCompleted := time.Since(p.startedAt) < time.Second*4
	if !rampUpCompleted && p.firstWorkerServed {
		return nil, ErrorResourceExhausted
	}

	if p.freeWorker == 0 {
		return nil, ErrorResourceExhausted
	}

	p.freeWorker--
	p.firstWorkerServed = true
	worker := NewRemoteWorker(p.clientFactory, "worker.key", p.logger)
	return worker, nil

}

func (p *SimpleWorkerPool) Return(_ context.Context, _ Worker) {
	p.freeWorker++
}

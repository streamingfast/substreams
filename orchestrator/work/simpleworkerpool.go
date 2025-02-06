package work

import (
	"context"
	"time"

	"github.com/streamingfast/substreams/client"
	"github.com/streamingfast/substreams/reqctx"
	"go.uber.org/zap"
)

//todo: Test Test Test

type SimpleWorkerPool struct {
	freeWorker        int
	startedAt         time.Time
	firstWorkerServed bool
	clientFactory     client.InternalClientFactory
	rampingUp         bool
	logger            *zap.Logger
}

type WorkerState int

func NewSimpleWorkerPool(ctx context.Context, workerCount int, clientFactory client.InternalClientFactory) *SimpleWorkerPool {
	logger := reqctx.Logger(ctx)

	logger.Debug("initializing worker pool", zap.Int("worker_count", workerCount))
	wp := &SimpleWorkerPool{
		freeWorker:    workerCount,
		startedAt:     time.Now(),
		clientFactory: clientFactory,
		rampingUp:     true,
		logger:        logger,
	}

	go func() {
		time.Sleep(time.Second * 4)
		logger.Info("worker pool ramping up completed")
		wp.rampingUp = false

	}()

	return wp
}

func (p *SimpleWorkerPool) Borrow(_ context.Context) (Worker, error) {
	if p.rampingUp && p.firstWorkerServed {
		return nil, ErrorResourceExhausted
	}

	if p.freeWorker == 0 {
		return nil, ErrorResourceExhausted
	}

	p.freeWorker--
	p.firstWorkerServed = true
	worker := NewRemoteWorker(p.clientFactory, "", 0, p.logger)
	return worker, nil

}

func (p *SimpleWorkerPool) RampingUp() bool {
	return p.rampingUp
}

func (p *SimpleWorkerPool) Return(_ context.Context, _ Worker) {
	p.freeWorker++
}

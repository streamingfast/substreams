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
	firstWorkerActive bool
	clientFactory     client.InternalClientFactory
	rampingUp         bool
	logger            *zap.Logger
}

type WorkerState int

func NewSimpleWorkerPool(ctx context.Context, workerCount int, clientFactory client.InternalClientFactory) *SimpleWorkerPool {
	logger := reqctx.Logger(ctx)
	logger = logger.Named("simple-worker-pool").With(zap.Bool("keep", false))

	logger.Info("initializing worker pool", zap.Int("worker_count", workerCount))
	wp := &SimpleWorkerPool{
		freeWorker:    workerCount,
		startedAt:     time.Now(),
		clientFactory: clientFactory,
		rampingUp:     true,
		logger:        logger,
	}

	go func() {
		time.Sleep(time.Second * 4)
		logger.Debug("worker pool ramping up completed")
		wp.rampingUp = false

	}()

	return wp
}

func (p *SimpleWorkerPool) Borrow(_ context.Context) (Worker, error) {
	if p.rampingUp && p.firstWorkerActive {
		return nil, ErrorResourceExhaustedRampUp
	}

	if p.freeWorker == 0 {
		return nil, ErrorResourceExhausted
	}

	p.freeWorker--
	p.firstWorkerActive = true
	worker := NewRemoteWorker(p.clientFactory, "", p.logger)

	p.logger.Info("worker borrowed", zap.String("worker_key", worker.ID()), zap.Int("remaining_worker", p.freeWorker))

	return worker, nil

}

func (p *SimpleWorkerPool) RampingUp() bool {
	return p.rampingUp
}

func (p *SimpleWorkerPool) Return(_ context.Context, worker Worker) {
	if p.rampingUp {
		p.firstWorkerActive = false // in case ramp up is still ongoing, we free that 'first worker'
	}
	p.freeWorker++
	p.logger.Info("worker returned", zap.String("worker_key", worker.ID()), zap.Int("remaining_worker", p.freeWorker))
}

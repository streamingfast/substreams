package work

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/streamingfast/substreams/client"
	"github.com/streamingfast/substreams/reqctx"
	pbworker "github.com/streamingfast/worker-pool-protocol/pb/sf/worker/v1"
	"go.uber.org/zap"
)

type GlobalWorkerPool struct {
	userID            string
	traceID           string
	startedAt         time.Time
	firstWorkerServed bool

	remoteWorkerPoolClient pbworker.WorkerPoolClient
	logger                 *zap.Logger
	clientFactory          client.InternalClientFactory
	workerKeepAliveDelay   time.Duration
	maxWorkerForTraceID    uint64
}

func NewGlobalWorkerPool(ctx context.Context, userID string, traceID string, maxWorkerForTraceID uint64, remoteWorkerPoolClient pbworker.WorkerPoolClient, clientFactory client.InternalClientFactory, workerKeepAliveDelay time.Duration) *GlobalWorkerPool {
	logger := reqctx.Logger(ctx)
	logger = logger.Named("global-worker-pool")

	logger.Info("initializing worker pool", zap.String("user_id", userID), zap.String("trace_id", traceID))

	return &GlobalWorkerPool{
		userID:                 userID,
		traceID:                traceID,
		maxWorkerForTraceID:    maxWorkerForTraceID,
		remoteWorkerPoolClient: remoteWorkerPoolClient,
		startedAt:              time.Now(),
		clientFactory:          clientFactory,
		workerKeepAliveDelay:   workerKeepAliveDelay,
		logger:                 logger,
	}
}

var ErrorResourceExhausted = errors.New("resource exhausted")

func (p *GlobalWorkerPool) Borrow(ctx context.Context) (Worker, error) {
	rampingUp := time.Since(p.startedAt) < time.Second*4
	if rampingUp && p.firstWorkerServed {
		p.logger.Info("worker pool is exhausted because of ramp up", zap.Bool("first_worker_served", p.firstWorkerServed), zap.Bool("ramping_up", rampingUp), zap.Duration("time_since_start", time.Since(p.startedAt)))
		return nil, ErrorResourceExhausted
	}

	response, err := p.remoteWorkerPoolClient.BorrowWorker(ctx,
		&pbworker.BorrowWorkerRequest{
			UserId:              p.userID,
			TraceId:             p.traceID,
			MaxWorkerForTraceId: int64(p.maxWorkerForTraceID),
		},
	)

	if err != nil {
		return nil, fmt.Errorf("borrowing worker for user %q and trace %q: %w", p.userID, p.traceID, err)
	}

	if response.Status == pbworker.BorrowWorkerResponse_resource_exhausted {
		p.logger.Info("worker pool is exhausted", zap.String("worker_key", response.WorkerKey), zap.String("status", response.Status.String()))
		return nil, ErrorResourceExhausted
	}

	p.firstWorkerServed = true
	worker := NewRemoteWorker(p.clientFactory, response.WorkerKey, p.workerKeepAliveDelay, p.logger)
	return worker, nil
}

func (p *GlobalWorkerPool) Return(ctx context.Context, worker Worker) {
	key := worker.ID()
	_, err := p.remoteWorkerPoolClient.ReturnWorker(ctx,
		&pbworker.ReturnWorkerRequest{
			WorkerKey: key,
		})

	if err != nil {
		p.logger.Error("returning worker", zap.Error(err))
	}
	p.logger.Info("returning worker", zap.String("worker_key", key))
}

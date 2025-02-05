package work

import (
	"context"
	"errors"
	"fmt"
	"time"

	"connectrpc.com/connect"
	"github.com/streamingfast/substreams/client"
	"github.com/streamingfast/substreams/reqctx"
	pbworker "github.com/streamingfast/worker-pool-protocol/pb/sf/worker/v1"
	"github.com/streamingfast/worker-pool-protocol/pb/sf/worker/v1/pbworkerconnect"
	"go.uber.org/zap"
)

type GlobalWorkerPool struct {
	userID            string
	traceID           string
	startedAt         time.Time
	firstWorkerServed bool

	remoteWorkerPool     pbworkerconnect.WorkerPoolClient
	logger               *zap.Logger
	clientFactory        client.InternalClientFactory
	workerKeepAliveDelay time.Duration
	maxWorkerForTraceID  uint64
}

func NewGlobalWorkerPool(ctx context.Context, userID string, traceID string, maxWorkerForTraceID uint64, remoteWorkerPool pbworkerconnect.WorkerPoolClient, clientFactory client.InternalClientFactory, workerKeepAliveDelay time.Duration) *GlobalWorkerPool {
	logger := reqctx.Logger(ctx)

	logger.Debug("initializing worker pool", zap.String("user_id", userID), zap.String("trace_id", traceID))

	return &GlobalWorkerPool{
		userID:               userID,
		traceID:              traceID,
		maxWorkerForTraceID:  maxWorkerForTraceID,
		remoteWorkerPool:     remoteWorkerPool,
		startedAt:            time.Now(),
		clientFactory:        clientFactory,
		workerKeepAliveDelay: workerKeepAliveDelay,
		logger:               logger,
	}
}

var ErrorResourceExhausted = errors.New("resource exhausted")

func (p *GlobalWorkerPool) Borrow(ctx context.Context) (Worker, error) {
	rampUpCompleted := time.Since(p.startedAt) < time.Second*4
	if !rampUpCompleted && p.firstWorkerServed {
		return nil, ErrorResourceExhausted
	}

	response, err := p.remoteWorkerPool.BorrowWorker(ctx,
		&connect.Request[pbworker.BorrowWorkerRequest]{
			Msg: &pbworker.BorrowWorkerRequest{
				UserId:              p.userID,
				TraceId:             p.traceID,
				MaxWorkerForTraceId: int64(p.maxWorkerForTraceID),
			},
		},
	)

	if err != nil {
		return nil, fmt.Errorf("borrowing worker for user %q and trace %q: %w", p.userID, p.traceID, err)
	}

	if response.Msg.Status == pbworker.BorrowWorkerResponse_borrowed {
		return nil, ErrorResourceExhausted
	}

	p.firstWorkerServed = true
	worker := NewRemoteWorker(p.clientFactory, response.Msg.WorkerKey, p.workerKeepAliveDelay, p.logger)
	return worker, nil
}

func (p *GlobalWorkerPool) Return(ctx context.Context, worker Worker) {
	key := worker.ID()
	_, err := p.remoteWorkerPool.ReturnWorker(ctx, &connect.Request[pbworker.ReturnWorkerRequest]{
		Msg: &pbworker.ReturnWorkerRequest{
			WorkerKey: key,
		},
	})

	if err != nil {
		p.logger.Error("returning worker", zap.Error(err))
	}
}

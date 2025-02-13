package work

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/streamingfast/derr"
	"github.com/streamingfast/substreams/client"
	"github.com/streamingfast/substreams/reqctx"
	pbworker "github.com/streamingfast/worker-pool-protocol/pb/sf/worker/v1"
	"go.uber.org/zap"
)

const Tier2WorkerServiceName = "t2w"

type GlobalWorkerPool struct {
	userID             string
	apiKeyID           string
	traceID            string
	startedAt          time.Time
	rampUpWorkerServed bool

	borrowedWorker         map[string]interface{}
	remoteWorkerPoolClient pbworker.WorkerPoolClient
	logger                 *zap.Logger
	clientFactory          client.InternalClientFactory
	workerKeepAliveDelay   time.Duration
	maxWorkerForTraceID    uint64
	rampingUp              bool
}

func NewGlobalWorkerPool(ctx context.Context, userID string, apiKeyID string, traceID string, maxWorkerForTraceID uint64, remoteWorkerPoolClient pbworker.WorkerPoolClient, clientFactory client.InternalClientFactory, workerKeepAliveDelay time.Duration) *GlobalWorkerPool {
	logger := reqctx.Logger(ctx)
	logger.Info("initializing worker pool", zap.String("user_id", userID), zap.String("api_key_id", apiKeyID), zap.String("trace_id", traceID), zap.Uint64("max_worker_for_trace_id", maxWorkerForTraceID), zap.Duration("worker_keep_alive_delay", workerKeepAliveDelay))

	logger = logger.Named("global-worker-pool").With(zap.Bool("keep", false))

	wp := &GlobalWorkerPool{
		userID:                 userID,
		apiKeyID:               apiKeyID,
		traceID:                traceID,
		maxWorkerForTraceID:    maxWorkerForTraceID,
		remoteWorkerPoolClient: remoteWorkerPoolClient,
		startedAt:              time.Now(),
		clientFactory:          clientFactory,
		workerKeepAliveDelay:   workerKeepAliveDelay,
		logger:                 logger,
		rampingUp:              true,
		borrowedWorker:         make(map[string]interface{}),
	}

	go func() {
		<-ctx.Done()
		for s := range wp.borrowedWorker {
			logger.Info("returning worker on context cancel", zap.String("worker_key", s))
			wp.Return(context.Background(), NewRemoteWorker(wp.clientFactory, s, 0, wp.logger))
		}
	}()

	go func() {
		time.Sleep(time.Second * 4)
		logger.Info("worker pool ramping up completed")
		wp.rampingUp = false
	}()

	return wp
}

var ErrorResourceExhausted = errors.New("resource exhausted")
var ErrorResourceExhaustedRampUp = errors.New("resource exhausted during ramp up")

func (p *GlobalWorkerPool) Borrow(ctx context.Context) (Worker, error) {
	if p.rampingUp && p.rampUpWorkerServed {
		p.logger.Info("worker pool is exhausted because of ramp up", zap.Bool("first_worker_served", p.rampUpWorkerServed), zap.Bool("ramping_up", p.rampingUp), zap.Duration("time_since_start", time.Since(p.startedAt)))
		return nil, ErrorResourceExhaustedRampUp
	}

	borrowWorkerResp := &pbworker.BorrowWorkerResponse{}
	err := derr.RetryContext(ctx, 3, func(ctx context.Context) error {
		response, err := p.remoteWorkerPoolClient.BorrowWorker(ctx,
			&pbworker.BorrowWorkerRequest{
				Service:             Tier2WorkerServiceName,
				UserId:              p.userID,
				ApiKeyId:            p.apiKeyID,
				TraceId:             p.traceID,
				MaxWorkerForTraceId: int64(p.maxWorkerForTraceID),
			},
		)

		if err != nil {
			return fmt.Errorf("borrowing worker for user %q and trace %q: %w", p.userID, p.traceID, err)
		}

		borrowWorkerResp = response
		return nil
	})

	key := borrowWorkerResp.WorkerKey
	status := borrowWorkerResp.Status

	if err != nil {
		p.logger.Error("error borrowing worker, will return free worker", zap.Error(err))
		key = "FREE.WORKER.KEY"
		status = pbworker.BorrowWorkerResponse_unset
	}

	if status == pbworker.BorrowWorkerResponse_resource_exhausted {
		p.logger.Info("worker pool is exhausted", zap.String("worker_key", key), zap.String("status", status.String()))
		return nil, ErrorResourceExhausted
	}

	p.rampUpWorkerServed = true
	worker := NewRemoteWorker(p.clientFactory, key, p.workerKeepAliveDelay, p.logger)
	p.logger.Info("worker borrowed", zap.String("worker_key", key))
	p.borrowedWorker[key] = struct{}{}

	return worker, nil
}

func (p *GlobalWorkerPool) Return(ctx context.Context, worker Worker) {
	delete(p.borrowedWorker, worker.ID())
	key := worker.ID()
	_, err := p.remoteWorkerPoolClient.ReturnWorker(ctx,
		&pbworker.ReturnWorkerRequest{
			WorkerKey: key,
		})

	if err != nil {
		p.logger.Error("returning worker", zap.Error(err))
	}
	p.rampUpWorkerServed = false
	p.logger.Info("returning worker", zap.String("worker_key", key))
}

func (p *GlobalWorkerPool) RampingUp() bool {
	return p.rampingUp
}

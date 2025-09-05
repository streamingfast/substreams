package work

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/streamingfast/substreams/client"
	"github.com/streamingfast/substreams/reqctx"
	pbworker "github.com/streamingfast/worker-pool-protocol/pb/sf/worker/v1"
	"go.uber.org/zap"
	"google.golang.org/grpc"
)

const Tier2WorkerServiceName = "t2w"
const Tier1RequestServiceName = "t1r"
const FreeWorkerKeyPrefix = "FREE.WORKER.KEY:"

var rampupTime = time.Second * 4

func init() {
	if envRampupTime := os.Getenv("SUBSTREAMS_WORKERS_RAMPUP_TIME"); envRampupTime != "" {
		if d, err := time.ParseDuration(envRampupTime); err == nil {
			rampupTime = d
		}
	}
}

type GlobalWorkerPool struct {
	userID             string
	apiKeyID           string
	traceID            string
	startedAt          time.Time
	rampUpWorkerServed bool

	borrowedWorker         map[string]Worker
	borrowedWorkerMutex    sync.Mutex
	remoteWorkerPoolClient pbworker.WorkerPoolClient
	logger                 *zap.Logger
	clientFactory          client.InternalClientFactory
	maxWorkerForTraceID    uint64
	rampingUp              bool
}

func NewGlobalWorkerPool(ctx context.Context, userID string, apiKeyID string, traceID string, maxWorkerForTraceID uint64, remoteWorkerPoolClient pbworker.WorkerPoolClient, clientFactory client.InternalClientFactory) *GlobalWorkerPool {
	logger := reqctx.Logger(ctx)
	logger.Info("initializing worker pool", zap.String("user_id", userID), zap.String("api_key_id", apiKeyID), zap.String("trace_id", traceID), zap.Uint64("max_worker_for_trace_id", maxWorkerForTraceID),
		zap.Duration("ramp_up_time", rampupTime))

	logger = logger.Named("global-worker-pool").With(zap.Bool("keep", false))

	wp := &GlobalWorkerPool{
		userID:                 userID,
		apiKeyID:               apiKeyID,
		traceID:                traceID,
		maxWorkerForTraceID:    maxWorkerForTraceID,
		remoteWorkerPoolClient: remoteWorkerPoolClient,
		startedAt:              time.Now(),
		clientFactory:          clientFactory,
		logger:                 logger,
		rampingUp:              true,
		borrowedWorker:         make(map[string]Worker),
	}

	go func() {
		<-ctx.Done()
		for s, w := range wp.borrowedWorker {
			logger.Info("returning workers on context cancel", zap.String("worker_key", s))
			wp.Return(context.Background(), w)
		}
	}()

	go func() {
		time.Sleep(rampupTime)
		logger.Debug("worker pool ramping up completed")
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

	borrowWorkerResp, err := p.remoteWorkerPoolClient.BorrowWorker(ctx,
		&pbworker.BorrowWorkerRequest{
			Service:             Tier2WorkerServiceName, // t2w
			UserId:              p.userID,
			ApiKeyId:            p.apiKeyID,
			TraceId:             p.traceID,
			MaxWorkerForTraceId: int64(p.maxWorkerForTraceID),
		},
		grpc.WaitForReady(false),
	)

	key := ""
	status := pbworker.BorrowWorkerResponse_unset

	if err != nil {
		borrowWorkerResp = &pbworker.BorrowWorkerResponse{}
		p.logger.Error("error borrowing worker, will return free worker", zap.Error(err))
		key = FreeWorkerKeyPrefix + time.Now().String()
		status = pbworker.BorrowWorkerResponse_borrowed

		if uint64(len(p.borrowedWorker)) >= p.maxWorkerForTraceID {
			status = pbworker.BorrowWorkerResponse_resource_exhausted
		}
	} else {
		key = borrowWorkerResp.WorkerKey
		status = borrowWorkerResp.Status
	}

	if status == pbworker.BorrowWorkerResponse_resource_exhausted {
		p.logger.Info("worker pool is exhausted", zap.String("worker_key", key), zap.String("status", status.String()))
		return nil, ErrorResourceExhausted
	}

	p.rampUpWorkerServed = true
	worker := NewRemoteWorker(p.clientFactory, key, p.logger)
	p.logger.Info("worker borrowed", zap.String("worker_key", key))
	p.borrowedWorkerMutex.Lock()
	p.borrowedWorker[key] = worker
	p.borrowedWorkerMutex.Unlock()

	return worker, nil
}

func (p *GlobalWorkerPool) Return(ctx context.Context, worker Worker) {
	p.borrowedWorkerMutex.Lock()
	delete(p.borrowedWorker, worker.ID())
	p.borrowedWorkerMutex.Unlock()

	if strings.HasPrefix(worker.ID(), FreeWorkerKeyPrefix) {
		return
	}

	key := worker.ID()
	_, err := p.remoteWorkerPoolClient.ReturnWorker(ctx,
		&pbworker.ReturnWorkerRequest{
			WorkerKey: key,
		},
		grpc.WaitForReady(false),
	)

	if err != nil {
		p.logger.Error("returning worker", zap.Error(err))
		//do not propagate that err...
	}

	p.rampUpWorkerServed = false
	p.logger.Info("returning worker", zap.String("worker_key", key))
}

func (p *GlobalWorkerPool) RampingUp() bool {
	return p.rampingUp
}

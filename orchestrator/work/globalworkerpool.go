package work

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"sync/atomic"
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
	rampUpWorkerServed int32

	borrowedWorker         map[string]Worker
	borrowedWorkerMutex    sync.Mutex
	remoteWorkerPoolClient WorkerBroker
	logger                 *zap.Logger
	clientFactory          client.InternalClientFactory
	workerKeepAliveDelay   time.Duration
	maxWorkerForTraceID    uint64
	rampingUp              int32
}

func NewGlobalWorkerPool(ctx context.Context, userID string, apiKeyID string, traceID string, maxWorkerForTraceID uint64, remoteWorkerPoolClient WorkerBroker, clientFactory client.InternalClientFactory, workerKeepAliveDelay time.Duration) *GlobalWorkerPool {
	logger := reqctx.Logger(ctx)
	logger.Info("initializing worker pool", zap.String("user_id", userID), zap.String("api_key_id", apiKeyID), zap.String("trace_id", traceID), zap.Uint64("max_worker_for_trace_id", maxWorkerForTraceID), zap.Duration("worker_keep_alive_delay", workerKeepAliveDelay),
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
		workerKeepAliveDelay:   workerKeepAliveDelay,
		logger:                 logger,
		rampingUp:              0,
		borrowedWorker:         make(map[string]Worker),
	}

	go func() {
		<-ctx.Done()

		// collect keys under lock to avoid concurrent map access while iterating
		wp.borrowedWorkerMutex.Lock()
		keys := make([]string, 0, len(wp.borrowedWorker))
		for k := range wp.borrowedWorker {
			keys = append(keys, k)
		}
		wp.borrowedWorkerMutex.Unlock()

		for _, k := range keys {
			// fetch the worker under lock to avoid races between Return/Delete
			wp.borrowedWorkerMutex.Lock()
			w, ok := wp.borrowedWorker[k]
			wp.borrowedWorkerMutex.Unlock()

			if !ok {
				continue
			}

			logger.Info("returning workers on context cancel", zap.String("worker_key", k))
			wp.Return(context.Background(), w)
		}
	}()

	// If rampupTime is set to a positive duration, start in rampingUp mode and clear it after the duration.
	if rampupTime > 0 {
		atomic.StoreInt32(&wp.rampingUp, 1)
		go func() {
			time.Sleep(rampupTime)
			logger.Debug("worker pool ramping up completed")
			atomic.StoreInt32(&wp.rampingUp, 0)
		}()
	} else {
		// immediate completion
		atomic.StoreInt32(&wp.rampingUp, 0)
	}

	return wp
}

var ErrorResourceExhausted = errors.New("resource exhausted")
var ErrorResourceExhaustedRampUp = errors.New("resource exhausted during ramp up")

func (p *GlobalWorkerPool) Borrow(ctx context.Context) (Worker, error) {
	if atomic.LoadInt32(&p.rampingUp) == 1 && atomic.LoadInt32(&p.rampUpWorkerServed) == 1 {
		p.logger.Info("worker pool is exhausted because of ramp up", zap.Bool("first_worker_served", atomic.LoadInt32(&p.rampUpWorkerServed) == 1), zap.Bool("ramping_up", atomic.LoadInt32(&p.rampingUp) == 1), zap.Duration("time_since_start", time.Since(p.startedAt)))
		return nil, ErrorResourceExhaustedRampUp
	}

	borrowWorkerResp, err := p.remoteWorkerPoolClient.BorrowWorker(ctx,
		&pbworker.BorrowWorkerRequest{
			Service:             Tier2WorkerServiceName,
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

	atomic.StoreInt32(&p.rampUpWorkerServed, 1)
	worker := NewRemoteWorker(p.clientFactory, key, p.logger)
	p.logger.Info("worker borrowed", zap.String("worker_key", key))
	p.borrowedWorkerMutex.Lock()
	p.borrowedWorker[key] = worker
	p.borrowedWorkerMutex.Unlock()

	worker.StartKeepAlive(ctx, p.workerKeepAliveDelay, p.remoteWorkerPoolClient)

	return worker, nil
}

func (p *GlobalWorkerPool) Return(ctx context.Context, worker Worker) {
	worker.StopKeepAlive()

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

	atomic.StoreInt32(&p.rampUpWorkerServed, 0)
	p.logger.Info("returning worker", zap.String("worker_key", key))
}

func (p *GlobalWorkerPool) RampingUp() bool {
	return atomic.LoadInt32(&p.rampingUp) == 1
}

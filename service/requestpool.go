package service

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/streamingfast/substreams/orchestrator/work"
	pbworker "github.com/streamingfast/worker-pool-protocol/pb/sf/worker/v1"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/durationpb"
)

type GlobalRequestPool struct {
	userBorrowedRequest              map[string]uint64
	borrowedRequestMutex             sync.Mutex
	remoteWorkerPoolClient           pbworker.WorkerPoolClient
	requestKeepAliveDelay            time.Duration
	logger                           *zap.Logger
	defaultMaxRequestPerUser         uint64
	defaultMinimalWorkerLifeDuration time.Duration
}

type BorrowedRequest struct {
	key                       string
	userID                    string
	status                    pbworker.BorrowWorkerResponse_BorrowStatus
	state                     *pbworker.WorkersState
	minimalWorkerLifeDuration time.Duration
	logger                    *zap.Logger
	done                      chan struct{}
}

func NewBorrowedRequest(key string, userID string, status pbworker.BorrowWorkerResponse_BorrowStatus, state *pbworker.WorkersState, minimalWorkerLifeDuration time.Duration, logger *zap.Logger) *BorrowedRequest {
	logger = logger.Named("borrowed-request")
	return &BorrowedRequest{
		key:                       key,
		userID:                    userID,
		status:                    status,
		state:                     state,
		minimalWorkerLifeDuration: minimalWorkerLifeDuration,
		done:                      make(chan struct{}),
		logger:                    logger,
	}
}

func (r *BorrowedRequest) startKeepAlive(ctx context.Context, delay time.Duration, remoteWorkerPoolClient pbworker.WorkerPoolClient) {
	if strings.HasPrefix(r.key, work.FreeWorkerKeyPrefix) {
		r.logger.Info("keep alive is not needed for free worker")
		return
	}

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-r.done:
				return
			case <-time.After(delay):
				_, err := remoteWorkerPoolClient.KeepAlive(
					ctx,
					&pbworker.KeepAliveRequest{
						WorkerKey: r.key,
					},
					grpc.WaitForReady(false),
				)
				if err != nil {
					r.logger.Error("failed to call keep request worker alive", zap.String("worker_id", r.key), zap.Error(err))
				}
			}
		}
	}()
}
func (r *BorrowedRequest) StopKeepAlive() {
	close(r.done)
}

func NewGlobalRequestPool(remoteWorkerPoolClient pbworker.WorkerPoolClient, requestKeepAliveDelay time.Duration, defaultMaxRequestPerUser uint64, defaultMinimalWorkerLifeDuration time.Duration, logger *zap.Logger) *GlobalRequestPool {
	logger = logger.Named("global-request-pool")

	rp :=
		&GlobalRequestPool{
			userBorrowedRequest:              make(map[string]uint64),
			borrowedRequestMutex:             sync.Mutex{},
			remoteWorkerPoolClient:           remoteWorkerPoolClient,
			requestKeepAliveDelay:            requestKeepAliveDelay,
			defaultMaxRequestPerUser:         defaultMaxRequestPerUser,
			defaultMinimalWorkerLifeDuration: defaultMinimalWorkerLifeDuration,
			logger:                           logger,
		}

	return rp
}

func (p *GlobalRequestPool) BorrowRequest(ctx context.Context, userID string, apiKeyID string, traceID string) *BorrowedRequest {
	resp, err := p.remoteWorkerPoolClient.BorrowWorker(ctx,
		&pbworker.BorrowWorkerRequest{
			Service:  work.Tier1RequestServiceName,
			UserId:   userID,
			ApiKeyId: apiKeyID,
			TraceId:  traceID,
		},
		grpc.WaitForReady(false),
	)

	key := ""
	status := pbworker.BorrowWorkerResponse_unset
	var state *pbworker.WorkersState
	var minimalWorkerLifeDuration = p.defaultMinimalWorkerLifeDuration

	if err != nil {
		resp = &pbworker.BorrowWorkerResponse{}
		p.logger.Error("error borrowing request worker, will return free worker", zap.Error(err))
		key = work.FreeWorkerKeyPrefix + time.Now().String()
		status = pbworker.BorrowWorkerResponse_borrowed

		p.borrowedRequestMutex.Lock()
		borrowedUserRequestCount := p.userBorrowedRequest[userID]
		p.borrowedRequestMutex.Unlock()

		if borrowedUserRequestCount >= p.defaultMaxRequestPerUser {
			status = pbworker.BorrowWorkerResponse_resource_exhausted
		}

		state = &pbworker.WorkersState{
			AvailableWorkers: int64(p.defaultMaxRequestPerUser - borrowedUserRequestCount),
			BorrowedWorkers:  int64(borrowedUserRequestCount),
			MaxWorkers:       int64(p.defaultMaxRequestPerUser),
			ActiveTraceId:    int64(borrowedUserRequestCount),
		}

	} else {
		key = resp.WorkerKey
		status = resp.Status
		state = resp.WorkerState
		minimalWorkerLifeDuration = resp.MinimalWorkerLifeDuration.AsDuration()
	}

	r := NewBorrowedRequest(key, userID, status, state, minimalWorkerLifeDuration, p.logger)

	if status == pbworker.BorrowWorkerResponse_resource_exhausted {
		p.logger.Info("worker pool is exhausted", zap.String("worker_key", key), zap.String("status", status.String()))
		return r
	}

	p.borrowedRequestMutex.Lock()
	p.userBorrowedRequest[userID]++
	p.borrowedRequestMutex.Unlock()

	r.startKeepAlive(ctx, p.requestKeepAliveDelay, p.remoteWorkerPoolClient)

	p.logger.Info("borrowed request worker", zap.String("worker_key", key))

	return r
}

func (p *GlobalRequestPool) ReturnRequest(r *BorrowedRequest) {

	r.StopKeepAlive()

	p.borrowedRequestMutex.Lock()
	p.userBorrowedRequest[r.userID]--
	if count, found := p.userBorrowedRequest[r.userID]; found && count < 0 {
		//this may occur if the borrow failed to call the sidecar
		p.userBorrowedRequest[r.userID] = 0
	}
	p.borrowedRequestMutex.Unlock()

	if strings.HasPrefix(r.key, work.FreeWorkerKeyPrefix) {
		p.logger.Info("returning free request worker", zap.String("worker_key", r.key), zap.Bool("keep", false))
		return
	}

	// Use zero duration to signal immediate return (pure active streams)
	// This ensures the request is immediately returned when the connection closes
	resp, err := p.remoteWorkerPoolClient.ReturnWorker(context.Background(),
		&pbworker.ReturnWorkerRequest{
			WorkerKey:                 r.key,
			MinimalWorkerLifeDuration: durationpb.New(0), // Zero duration for immediate return
		},
		grpc.WaitForReady(false),
	)

	if err != nil {
		p.logger.Error("returning request worker", zap.Error(err))
		//do not propagate that err...
	} else {
		p.logger.Info("returned request worker (pure mode)", zap.String("key", r.key), zap.Stringer("status", resp.Status))
	}
}

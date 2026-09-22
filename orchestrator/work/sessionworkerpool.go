package work

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/streamingfast/dsession"
	"github.com/streamingfast/substreams/client"
	"github.com/streamingfast/substreams/reqctx"
	"go.uber.org/zap"
)

const Tier2WorkerServiceName = "t2w"

var rampupTime = time.Second * 4

func init() {
	if envDuration := os.Getenv("SUBSTREAMS_WORKERS_RAMPUP_TIME"); envDuration != "" {
		if parsed, err := time.ParseDuration(envDuration); err == nil {
			rampupTime = parsed
		}
	}
}

type SessionWorkerPool struct {
	sessionPool   dsession.SessionPool
	clientFactory client.InternalClientFactory
	logger        *zap.Logger

	serviceName          string
	sessionKey           string // This is the sessionID from sessionPool.Get(), used as requestKey in GetWorker
	maxWorkersPerSession int

	borrowedWorkers      map[string]Worker
	borrowedWorkersMutex sync.Mutex

	launchQueue *LaunchQueue

	rampingUp         *atomic.Bool
	rampupWorkerGiven *atomic.Bool
}

func NewSessionWorkerPool(
	ctx context.Context,
	sessionKey string,
	sessionPool dsession.SessionPool,
	clientFactory client.InternalClientFactory,
) *SessionWorkerPool {
	logger := reqctx.Logger(ctx)
	logger = logger.Named("session-worker-pool").With(zap.Bool("keep", false))

	reqDetails := reqctx.Details(ctx)
	maxWorkers := int(reqDetails.MaxParallelJobs)

	logger.Debug("initializing session worker pool",
		zap.String("session_key", sessionKey),
		zap.Int("max_workers", maxWorkers),
	)

	wp := &SessionWorkerPool{
		sessionPool:          sessionPool,
		clientFactory:        clientFactory,
		logger:               logger,
		serviceName:          Tier2WorkerServiceName,
		sessionKey:           sessionKey,
		maxWorkersPerSession: maxWorkers,
		borrowedWorkers:      make(map[string]Worker),
		launchQueue:          NewLaunchQueue(maxWorkers),
		rampingUp:            &atomic.Bool{},
		rampupWorkerGiven:    &atomic.Bool{},
	}
	wp.rampingUp.Store(true)
	time.AfterFunc(rampupTime, func() {
		wp.rampingUp.Store(false)
	})

	// Clean up workers on context cancellation
	go func() {
		<-ctx.Done()
		wp.ReleaseAll()
	}()

	return wp
}

func (p *SessionWorkerPool) ReleaseAll() {
	p.borrowedWorkersMutex.Lock()
	defer p.borrowedWorkersMutex.Unlock()

	for key := range p.borrowedWorkers {
		p.logger.Debug("returning worker", zap.String("worker_key", key))
		p.sessionPool.ReleaseWorker(key)
		delete(p.borrowedWorkers, key)
	}
}

func (p *SessionWorkerPool) Borrow(ctx context.Context) (Worker, error) {
	rampingUp := p.rampingUp.Load()
	if rampingUp {
		if p.rampupWorkerGiven.Load() {
			return nil, ErrorResourceExhaustedRampUp
		}
	}

	// Use sessionKey as the requestKey parameter for GetWorker
	workerKey, err := p.sessionPool.GetWorker(
		ctx,
		p.serviceName,
		p.sessionKey, // sessionKey is passed as requestKey to GetWorker
		p.maxWorkersPerSession,
	)

	if err != nil {
		if errors.Is(err, dsession.ErrWorkersLimitExceeded) {
			p.logger.Debug("worker pool is exhausted", zap.Error(err))
			return nil, ErrorResourceExhausted
		}
		if errors.Is(err, dsession.ErrSessionNotFound) {
			p.logger.Error("session not found in pool", zap.Error(err))
			return nil, fmt.Errorf("session not found: %w", err)
		}
		p.logger.Error("failed to get worker from session pool", zap.Error(err))
		return nil, fmt.Errorf("failed to get worker: %w", err)
	}

	worker := NewRemoteWorker(p.clientFactory, workerKey, p.logger, p.launchQueue)

	p.borrowedWorkersMutex.Lock()
	p.borrowedWorkers[workerKey] = worker
	p.borrowedWorkersMutex.Unlock()

	p.logger.Debug("worker borrowed",
		zap.String("worker_key", workerKey),
		zap.Int("active_workers", len(p.borrowedWorkers)),
	)

	if rampingUp {
		p.rampupWorkerGiven.Store(true)
	}

	return worker, nil
}

func (p *SessionWorkerPool) Return(ctx context.Context, worker Worker) {
	workerKey := worker.ID()

	p.borrowedWorkersMutex.Lock()
	delete(p.borrowedWorkers, workerKey)
	activeWorkers := len(p.borrowedWorkers)
	p.borrowedWorkersMutex.Unlock()

	p.sessionPool.ReleaseWorker(workerKey)

	p.logger.Debug("worker returned",
		zap.String("worker_key", workerKey),
		zap.Int("remaining_workers", activeWorkers),
	)
}

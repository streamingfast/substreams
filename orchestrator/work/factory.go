package work

import (
	"context"
	"time"

	"github.com/streamingfast/dauth"
	tracing "github.com/streamingfast/sf-tracing"
	"github.com/streamingfast/substreams/client"
	"github.com/streamingfast/substreams/reqctx"
	"github.com/streamingfast/worker-pool-protocol/pb/sf/worker/v1/pbworkerconnect"
)

type WorkerPoolFactory func(ctx context.Context) WorkerPool

type GlobalWorkerPoolFactory struct {
	clientFactory        client.InternalClientFactory
	remoteWorkerPool     pbworkerconnect.WorkerPoolClient
	workerKeepAliveDelay time.Duration
}

func NewGlobalWorkerPoolFactory(remoteWorkerPool pbworkerconnect.WorkerPoolClient, clientFactory client.InternalClientFactory, workerKeepAliveDelay time.Duration) *GlobalWorkerPoolFactory {
	return &GlobalWorkerPoolFactory{
		remoteWorkerPool:     remoteWorkerPool,
		workerKeepAliveDelay: workerKeepAliveDelay,
		clientFactory:        clientFactory,
	}
}

func (f *GlobalWorkerPoolFactory) WorkerPool(ctx context.Context) WorkerPool {
	userID := dauth.FromContext(ctx).UserID()
	traceID := tracing.GetTraceID(ctx)
	workerPool := NewGlobalWorkerPool(ctx, userID, traceID.String(), f.remoteWorkerPool, f.clientFactory, f.workerKeepAliveDelay)

	return workerPool
}

type SimpleWorkerPoolFactory struct {
	clientFactory client.InternalClientFactory
}

func NewSimpleWorkerPoolFactory(clientFactory client.InternalClientFactory) *SimpleWorkerPoolFactory {
	return &SimpleWorkerPoolFactory{
		clientFactory: clientFactory,
	}
}

func (f *SimpleWorkerPoolFactory) WorkerPool(ctx context.Context) WorkerPool {
	reqDetails := reqctx.Details(ctx)
	workerPool := NewSimpleWorkerPool(ctx, int(reqDetails.MaxParallelJobs), f.clientFactory)

	return workerPool
}

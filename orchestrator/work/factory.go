package work

import (
	"context"

	"github.com/streamingfast/dauth"
	tracing "github.com/streamingfast/sf-tracing"
	"github.com/streamingfast/substreams/client"
	"github.com/streamingfast/substreams/reqctx"
	pbworker "github.com/streamingfast/worker-pool-protocol/pb/sf/worker/v1"
)

type WorkerPoolFactory func(ctx context.Context) WorkerPool

type GlobalWorkerPoolFactory struct {
	clientFactory    client.InternalClientFactory
	remoteWorkerPool pbworker.WorkerPoolClient
}

func NewGlobalWorkerPoolFactory(remoteWorkerPool pbworker.WorkerPoolClient, clientFactory client.InternalClientFactory) *GlobalWorkerPoolFactory {

	return &GlobalWorkerPoolFactory{
		remoteWorkerPool: remoteWorkerPool,
		clientFactory:    clientFactory,
	}
}

func (f *GlobalWorkerPoolFactory) WorkerPool(ctx context.Context) WorkerPool {
	userID := dauth.FromContext(ctx).UserID()
	apiKeyID := dauth.FromContext(ctx).APIKeyID()
	traceID := tracing.GetTraceID(ctx)
	reqDetails := reqctx.Details(ctx)
	workerPool := NewGlobalWorkerPool(ctx, userID, apiKeyID, traceID.String(), reqDetails.MaxParallelJobs, f.remoteWorkerPool, f.clientFactory)

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

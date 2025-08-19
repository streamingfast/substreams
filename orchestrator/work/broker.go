package work

import (
	"context"

	pbworker "github.com/streamingfast/worker-pool-protocol/pb/sf/worker/v1"
	"google.golang.org/grpc"
)

// WorkerBroker abstracts the external worker-pool service used by GlobalWorkerPool.
// This allows decoupling the GlobalWorkerPool from the concrete gRPC client and
// makes testing easier
type WorkerBroker interface {
	BorrowWorker(ctx context.Context, req *pbworker.BorrowWorkerRequest, opts ...grpc.CallOption) (*pbworker.BorrowWorkerResponse, error)
	ReturnWorker(ctx context.Context, req *pbworker.ReturnWorkerRequest, opts ...grpc.CallOption) (*pbworker.ReturnWorkerResponse, error)
	KeepAlive(ctx context.Context, req *pbworker.KeepAliveRequest, opts ...grpc.CallOption) (*pbworker.KeepAliveResponse, error)
}

// GRPCWorkerBroker is a thin wrapper around the generated pbworker.WorkerPoolClient
// that implements WorkerBroker.
type GRPCWorkerBroker struct {
	client pbworker.WorkerPoolClient
}

func NewGRPCWorkerBroker(client pbworker.WorkerPoolClient) *GRPCWorkerBroker {
	return &GRPCWorkerBroker{client: client}
}

func (g *GRPCWorkerBroker) BorrowWorker(ctx context.Context, req *pbworker.BorrowWorkerRequest, opts ...grpc.CallOption) (*pbworker.BorrowWorkerResponse, error) {
	return g.client.BorrowWorker(ctx, req, opts...)
}

func (g *GRPCWorkerBroker) ReturnWorker(ctx context.Context, req *pbworker.ReturnWorkerRequest, opts ...grpc.CallOption) (*pbworker.ReturnWorkerResponse, error) {
	return g.client.ReturnWorker(ctx, req, opts...)
}

func (g *GRPCWorkerBroker) KeepAlive(ctx context.Context, req *pbworker.KeepAliveRequest, opts ...grpc.CallOption) (*pbworker.KeepAliveResponse, error) {
	return g.client.KeepAlive(ctx, req, opts...)
}

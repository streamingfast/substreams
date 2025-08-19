package work

import (
	"context"
	"testing"

	pbworker "github.com/streamingfast/worker-pool-protocol/pb/sf/worker/v1"
	"google.golang.org/grpc"
)

type fakeClient struct{}

func (f *fakeClient) BorrowWorker(ctx context.Context, req *pbworker.BorrowWorkerRequest, _ ...grpc.CallOption) (*pbworker.BorrowWorkerResponse, error) {
	return &pbworker.BorrowWorkerResponse{WorkerKey: "k", Status: pbworker.BorrowWorkerResponse_borrowed}, nil
}
func (f *fakeClient) ReturnWorker(ctx context.Context, req *pbworker.ReturnWorkerRequest, _ ...grpc.CallOption) (*pbworker.ReturnWorkerResponse, error) {
	return &pbworker.ReturnWorkerResponse{}, nil
}
func (f *fakeClient) KeepAlive(ctx context.Context, req *pbworker.KeepAliveRequest, _ ...grpc.CallOption) (*pbworker.KeepAliveResponse, error) {
	return &pbworker.KeepAliveResponse{}, nil
}

func TestGRPCWorkerBroker_Forwards(t *testing.T) {
	fc := &fakeClient{}
	var broker WorkerBroker = fc

	if _, err := broker.BorrowWorker(context.Background(), &pbworker.BorrowWorkerRequest{}); err != nil {
		t.Fatalf("borrow failed: %v", err)
	}
	if _, err := broker.ReturnWorker(context.Background(), &pbworker.ReturnWorkerRequest{}); err != nil {
		t.Fatalf("return failed: %v", err)
	}
	if _, err := broker.KeepAlive(context.Background(), &pbworker.KeepAliveRequest{}); err != nil {
		t.Fatalf("keepalive failed: %v", err)
	}
}

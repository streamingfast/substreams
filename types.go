package substreams

import (
	"context"

	pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

type ResponseFromAnyTier interface {
	ProtoMessage()
}
type ResponseFunc func(ResponseFromAnyTier) error

func NewBlockScopedDataResponse(in *pbsubstreamsrpc.BlockScopedData) *pbsubstreamsrpc.Response {
	return &pbsubstreamsrpc.Response{
		Message: &pbsubstreamsrpc.Response_BlockScopedData{BlockScopedData: in},
	}
}

func NewSnapshotData(in *pbsubstreamsrpc.InitialSnapshotData) *pbsubstreamsrpc.Response {
	return &pbsubstreamsrpc.Response{
		Message: &pbsubstreamsrpc.Response_DebugSnapshotData{DebugSnapshotData: in},
	}
}

func NewSnapshotComplete() *pbsubstreamsrpc.Response {
	return &pbsubstreamsrpc.Response{
		Message: &pbsubstreamsrpc.Response_DebugSnapshotComplete{DebugSnapshotComplete: &pbsubstreamsrpc.InitialSnapshotComplete{}},
	}
}

// BlockHooks will always be called with a valid clock
type BlockHook func(ctx context.Context, clock *pbsubstreams.Clock) error

// PostJobHooks will be called at the end of a job. The clock can be `nil` in some circumstances, or it can be >= job.StopBlock
type PostJobHook func(ctx context.Context, clock *pbsubstreams.Clock) error

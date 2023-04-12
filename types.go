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

func NewBlockDataResponse(in *pbsubstreamsrpc.BlockScopedData) *pbsubstreamsrpc.Response {
	return &pbsubstreamsrpc.Response{
		Message: &pbsubstreamsrpc.Response_BlockData{BlockData: in},
	}
}

func NewModulesProgressResponse(in []*pbsubstreamsrpc.ModuleProgress) *pbsubstreamsrpc.Response {
	return &pbsubstreamsrpc.Response{
		Message: &pbsubstreamsrpc.Response_Progress{Progress: &pbsubstreamsrpc.ModulesProgress{Modules: in}},
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

type BlockHook func(ctx context.Context, clock *pbsubstreams.Clock) error
type PostJobHook func(ctx context.Context, clock *pbsubstreams.Clock) error

package substreams

import (
	"context"

	pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
	"google.golang.org/grpc"
)

type GrpcClientFactory func(context.Context) (streamClient pbsubstreamsrpc.StreamClient, closeFunc func() error, opts []grpc.CallOption, err error)

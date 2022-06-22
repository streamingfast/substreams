package substreams

import (
	"context"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"google.golang.org/grpc"
)

type GrpcClientFactory func(context.Context) (streamClient pbsubstreams.StreamClient, closeFunc func() error, opts []grpc.CallOption, err error)

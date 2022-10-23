package pipeline

import (
	"context"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

type FinalBlockHandler interface {
	HandleFinal(ctx context.Context, clock *pbsubstreams.Clock) error
}

type EndOfStreamHandler interface {
	EndOfStream(ctx context.Context, clock *pbsubstreams.Clock) error
}

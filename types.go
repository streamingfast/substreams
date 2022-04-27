package substreams

import (
	"context"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

type ReturnFunc func(any *pbsubstreams.BlockScopedData) error

type BlockHook func(ctx context.Context, clock *pbsubstreams.Clock) error
type PostJobHook func(ctx context.Context, clock *pbsubstreams.Clock) error

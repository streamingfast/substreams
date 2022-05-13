package substreams

import (
	"context"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

type ReturnFunc func(any *pbsubstreams.BlockScopedData, progress *pbsubstreams.ModulesProgress) error
type ProgressFunc func(modulesProgress *pbsubstreams.ModulesProgress) error

type BlockHook func(ctx context.Context, clock *pbsubstreams.Clock) error
type PostJobHook func(ctx context.Context, clock *pbsubstreams.Clock) error

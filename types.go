package substreams

import (
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

type ReturnFunc func(any *pbsubstreams.BlockScopedData) error

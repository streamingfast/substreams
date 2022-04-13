package substreams

import (
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"

	"github.com/streamingfast/bstream"
)

type ReturnFunc func(any *pbsubstreams.Output, step bstream.StepType, cursor *bstream.Cursor) error

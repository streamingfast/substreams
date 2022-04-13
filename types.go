package substreams

import (
	"time"

	"github.com/streamingfast/bstream"
	"google.golang.org/protobuf/types/known/anypb"
)

type ReturnFunc func(any *anypb.Any, blockNum uint64, blockID string, blockTime time.Time, step bstream.StepType, cursor *bstream.Cursor) error

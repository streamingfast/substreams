package substreams

import (
	"github.com/streamingfast/bstream"
	"google.golang.org/protobuf/types/known/anypb"
)

type ReturnFunc func(any *anypb.Any, block *bstream.Block, step bstream.StepType, cursor *bstream.Cursor) error

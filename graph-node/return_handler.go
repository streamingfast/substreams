package graphnode

import (
	"github.com/streamingfast/bstream"
	"google.golang.org/protobuf/types/known/anypb"
)

type GraphNodeImporter struct {
}

func (gni *GraphNodeImporter) ReturnHandler(any *anypb.Any, step bstream.StepType, cursor *bstream.Cursor) error {
	return nil
}

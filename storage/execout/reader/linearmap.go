package reader

import (
	"github.com/streamingfast/substreams"
	"github.com/streamingfast/substreams/block"
	"github.com/streamingfast/substreams/orchestrator/loop"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

type LinearMapReader struct {
	responseFunc substreams.ResponseFunc
	module       *pbsubstreams.Module
	segmenter    *block.Segmenter
	nextSegment  int
}

func NewLinearMapReader(module *pbsubstreams.Module, segmenter *block.Segmenter) *LinearMapReader {
	return &LinearMapReader{
		segmenter:   segmenter,
		module:      module,
		nextSegment: 0,
	}
}

func (r *LinearMapReader) CmdDownloadNextFile() loop.Cmd {
	return func() loop.Msg {

		return nil
	}
}

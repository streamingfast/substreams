package service

import (
	"github.com/streamingfast/bstream"
	"github.com/streamingfast/bstream/hub"
	"github.com/streamingfast/bstream/stream"
	"github.com/streamingfast/dstore"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type StreamFactory struct {
	mergedBlocksStore dstore.Store
	forkedBlocksStore dstore.Store
	hub               *hub.ForkableHub
}

func (sf *StreamFactory) New(
	h bstream.Handler,
	startBlockNum int64,
	stopBlockNum uint64,
	cursor string,
) (Streamable, error) {

	options := []stream.Option{
		stream.WithStopBlock(stopBlockNum),
		stream.WithCustomStepTypeFilter(bstream.StepsAll), // substreams always wants new, undo, new+irreversible, irreversible, stalled
	}

	if cursor != "" {
		cur, err := bstream.CursorFromOpaque(cursor)
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "invalid start cursor %q: %s", cursor, err)
		}

		options = append(options, stream.WithCursor(cur))
	}

	return stream.New(
		sf.forkedBlocksStore,
		sf.mergedBlocksStore,
		sf.hub,
		startBlockNum,
		h,
		options...), nil
}

func (s *StreamFactory) GetRecentFinalBlock() (uint64, error) {
	_, _, _, finalBlockNum, err := s.hub.HeadInfo()
	if finalBlockNum > bstream.GetProtocolFirstStreamableBlock+200 {
		finalBlockNum -= finalBlockNum % 100
		finalBlockNum -= 100
	} else if finalBlockNum > bstream.GetProtocolFirstStreamableBlock+200 {
		finalBlockNum -= finalBlockNum % 100
	}

	return finalBlockNum, err
}

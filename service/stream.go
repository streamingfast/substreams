package service

import (
	"context"

	"github.com/streamingfast/bstream"
	"github.com/streamingfast/bstream/hub"
	"github.com/streamingfast/bstream/stream"
	"github.com/streamingfast/dmetering"
	"github.com/streamingfast/dstore"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type StreamFactory struct {
	mergedBlocksStore dstore.Store
	forkedBlocksStore dstore.Store
	hub               *hub.ForkableHub
}

func (sf *StreamFactory) New(
	ctx context.Context,
	h bstream.Handler,
	startBlockNum int64,
	stopBlockNum uint64,
	cursor string,
	finalBlocksOnly bool,
	cursorIsTarget bool,
	logger *zap.Logger,
) (Streamable, error) {
	options := []stream.Option{
		stream.WithStopBlock(stopBlockNum),
		stream.WithCustomStepTypeFilter(bstream.StepsAll), // substreams always wants new, undo, new+irreversible, irreversible, stalled
		stream.WithLogger(logger),
	}
	if finalBlocksOnly {
		options = append(options, stream.WithFinalBlocksOnly())
	}

	if cursor != "" {
		cur, err := bstream.CursorFromOpaque(cursor)
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "invalid StartCursor %q: %s", cursor, err)
		}
		if cursorIsTarget {
			options = append(options, stream.WithTargetCursor(cur))
		} else {
			options = append(options, stream.WithCursor(cur))
		}
	}

	forkedBlocksStore := sf.forkedBlocksStore
	if clonable, ok := forkedBlocksStore.(dstore.Clonable); ok {
		var err error
		forkedBlocksStore, err = clonable.Clone(ctx)
		if err != nil {
			return nil, err
		}
		forkedBlocksStore.SetMeter(dmetering.GetBytesMeter(ctx))
	} else {
		logger.Debug("forkedBlocksStore cannot be cloned, will not be metered")
	}

	mergedBlocksStore := sf.mergedBlocksStore
	if clonable, ok := mergedBlocksStore.(dstore.Clonable); ok {
		var err error
		mergedBlocksStore, err = clonable.Clone(ctx)
		if err != nil {
			return nil, err
		}
		mergedBlocksStore.SetMeter(dmetering.GetBytesMeter(ctx))
	} else {
		logger.Debug("mergedBlocksStore cannot be cloned, will not be metered")
	}

	return stream.New(
		forkedBlocksStore,
		mergedBlocksStore,
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

func (s *StreamFactory) GetHeadBlock() (uint64, error) {
	headNum, _, _, _, err := s.hub.HeadInfo()
	if err != nil {
		return 0, err
	}

	return headNum, nil
}

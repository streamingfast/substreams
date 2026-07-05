package service

import (
	"context"
	"fmt"

	"github.com/streamingfast/bstream"
	"github.com/streamingfast/bstream/hub"
	"github.com/streamingfast/bstream/stream"
	"github.com/streamingfast/dmetering"
	"github.com/streamingfast/dstore"
	"github.com/streamingfast/substreams/metering"

	"connectrpc.com/connect"
	"go.uber.org/zap"
)

type StreamFactory struct {
	mergedBlocksStore      dstore.Store
	forkedBlocksStore      dstore.Store
	hub                    *hub.ForkableHub
	mergedBlocksBundleSize uint64 // 0 means bstream.DefaultMergedBlocksBundleSize
}

func (sf *StreamFactory) New(
	ctx context.Context,
	h bstream.Handler,
	startBlockNum int64,
	stopBlockNum uint64,
	cursor string,
	finalBlocksOnly bool,
	includePartialBlocks bool,
	cursorIsTarget bool,
	logger *zap.Logger,
	extraOpts ...stream.Option,
) (Streamable, error) {
	options := []stream.Option{
		stream.WithStopBlock(stopBlockNum),
		stream.WithCustomStepTypeFilter(bstream.StepsAll), // substreams always wants new, undo, new+irreversible, irreversible, stalled
		stream.WithLogger(logger),
	}
	if sf.mergedBlocksBundleSize != 0 {
		options = append(options, stream.WithMergedBlocksBundleSize(sf.mergedBlocksBundleSize))
	}
	if finalBlocksOnly {
		options = append(options, stream.WithFinalBlocksOnly())
	}

	if includePartialBlocks {
		options = append(options, stream.WithCustomStepTypeFilter(bstream.StepsAllWithPartial))
	}

	if cursor != "" {
		cur, err := bstream.CursorFromOpaque(cursor)
		if err != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid StartCursor %q: %w", cursor, err))
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
		//todo: (deprecated)
		forkedBlocksStore.SetMeter(dmetering.GetBytesMeter(ctx))
	} else {
		logger.Debug("forkedBlocksStore cannot be cloned, will not be metered")
	}

	mergedBlocksStore := sf.mergedBlocksStore
	if clonable, ok := mergedBlocksStore.(dstore.Clonable); ok {
		var err error
		mergedBlocksStore, err = clonable.Clone(ctx, metering.WithBlockBytesReadMeteringOptions(dmetering.GetBytesMeter(ctx), logger)...)
		if err != nil {
			return nil, err
		}
		//todo: (deprecated)
		mergedBlocksStore.SetMeter(dmetering.GetBytesMeter(ctx))
	} else {
		logger.Debug("mergedBlocksStore cannot be cloned, will not be metered")
	}

	for _, opt := range extraOpts {
		options = append(options, opt)
	}

	factory := stream.New(
		forkedBlocksStore,
		mergedBlocksStore,
		sf.hub,
		startBlockNum,
		h,
		options...)

	return factory, nil
}

func (s *StreamFactory) GetRecentFinalBlock() (uint64, error) {
	_, _, _, finalBlockNum, err := s.hub.HeadInfo()
	return roundToBundleFinalBlock(finalBlockNum, s.bundleSize(), bstream.GetProtocolFirstStreamableBlock), err
}

func (s *StreamFactory) bundleSize() uint64 {
	if s.mergedBlocksBundleSize != 0 {
		return s.mergedBlocksBundleSize
	}
	return bstream.DefaultMergedBlocksBundleSize
}

// roundToBundleFinalBlock rounds a final block down to the start of the previous
// merged-blocks bundle, the most recent block that is guaranteed to be readable
// from a fully-written merged-blocks file.
func roundToBundleFinalBlock(finalBlockNum, bundleSize, firstStreamableBlock uint64) uint64 {
	if finalBlockNum > firstStreamableBlock+2*bundleSize {
		finalBlockNum -= finalBlockNum % bundleSize
		finalBlockNum -= bundleSize
	}
	return finalBlockNum
}

func (s *StreamFactory) GetHeadBlock() (uint64, error) {
	headNum, _, _, _, err := s.hub.HeadInfo()
	if err != nil {
		return 0, err
	}

	return headNum, nil
}

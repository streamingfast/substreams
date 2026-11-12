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

// GetRecentFinalBlock returns the most recent final block up to which tier2 can process:
// tier2 only reads merged-blocks files, and it needs the first block of the file after a
// segment to know that the segment is over. The linear handoff to tier1 is placed there.
func (s *StreamFactory) GetRecentFinalBlock() (uint64, error) {
	_, _, _, finalBlockNum, err := s.hub.HeadInfo()
	recent := roundToBundleFinalBlock(finalBlockNum, s.bundleSize(), bstream.GetProtocolFirstStreamableBlock)
	if err != nil {
		return recent, err
	}

	mergedUpTo, ok := s.hub.MergedBlocksUpTo()
	if !ok {
		return recent, nil
	}
	return capToLastMergedFile(recent, mergedUpTo, s.bundleSize(), bstream.GetProtocolFirstStreamableBlock), nil
}

// capToLastMergedFile lowers recentFinalBlock to the start of the last merged-blocks file
// the hub found, mergedUpTo being the first block that is not in a merged-blocks file.
// With a merger that is behind, it keeps tier2 off files that do not exist yet and leaves
// the blocks above to tier1, which reads them from the hub. With a merger keeping up, it
// only lowers recentFinalBlock by one file for the few seconds the hub takes to find each
// new file.
func capToLastMergedFile(recentFinalBlock, mergedUpTo, bundleSize, firstStreamableBlock uint64) uint64 {
	var lastFileStart uint64
	if mergedUpTo > bundleSize {
		lastFileStart = mergedUpTo - bundleSize
	}
	return min(recentFinalBlock, max(lastFileStart, firstStreamableBlock))
}

func (s *StreamFactory) bundleSize() uint64 {
	if s.mergedBlocksBundleSize != 0 {
		return s.mergedBlocksBundleSize
	}
	return bstream.DefaultMergedBlocksBundleSize
}

// roundToBundleFinalBlock rounds a final block down to the start of the previous
// merged-blocks bundle, the most recent block readable from a fully-written
// merged-blocks file when the merger keeps up. capToLastMergedFile covers a merger
// that is behind.
func roundToBundleFinalBlock(finalBlockNum, bundleSize, firstStreamableBlock uint64) uint64 {
	// Need at least one complete bundle above the chain start; below that no
	// merged-blocks file exists yet, so there is nothing to snap to.
	if finalBlockNum < firstStreamableBlock+bundleSize {
		return finalBlockNum
	}
	rounded := finalBlockNum - finalBlockNum%bundleSize - bundleSize
	if rounded < firstStreamableBlock {
		return firstStreamableBlock
	}
	return rounded
}

func (s *StreamFactory) GetHeadBlock() (uint64, error) {
	headNum, _, _, _, err := s.hub.HeadInfo()
	if err != nil {
		return 0, err
	}

	return headNum, nil
}

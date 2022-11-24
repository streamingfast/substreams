package pipeline

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/streamingfast/bstream/stream"
	"github.com/streamingfast/substreams/reqctx"
	"go.uber.org/zap"
	"google.golang.org/grpc/metadata"
)

// OnStreamTerminated performs flush of store and setting trailers when the stream terminated gracefully from our point of view.
// If the stream terminated gracefully, we return `nil` otherwise, the original is returned.
func (p *Pipeline) OnStreamTerminated(ctx context.Context, streamSrv Trailable, err error) error {
	logger := reqctx.Logger(ctx)
	reqDetails := reqctx.Details(ctx)

	for _, executor := range p.moduleExecutors {
		executor.FreeMem()
	}

	if errors.Is(err, stream.ErrStopBlockReached) || errors.Is(err, io.EOF) {
		logger.Debug("stream of blocks ended",
			zap.Uint64("stop_block_num", reqDetails.Request.StopBlockNum),
			zap.Bool("eof", errors.Is(err, io.EOF)),
			zap.Bool("stop_block_reached", errors.Is(err, stream.ErrStopBlockReached)),
		)

		if err := p.execOutputCache.EndOfStream(p.lastFinalClock); err != nil {
			return fmt.Errorf("end of stream: %w", err)
		}

		if err := p.stores.flushStores(ctx, reqDetails.Request.StopBlockNum); err != nil {
			return fmt.Errorf("step new irr: stores end of stream: %w", err)
		}

		if p.stores.partialsWritten != nil {
			partialRanges := make([]string, len(p.stores.partialsWritten))
			for i, rng := range p.stores.partialsWritten {
				partialRanges[i] = fmt.Sprintf("%d-%d", rng.StartBlock, rng.ExclusiveEndBlock)
			}
			logger.Info("setting trailer", zap.Strings("ranges", partialRanges))
			streamSrv.SetTrailer(metadata.MD{"substreams-partials-written": []string{strings.Join(partialRanges, ",")}})
		}

		// It was an ok error, so let's
		return nil
	}

	// We are not responsible for doing any other error handling here, caller will deal with them
	return err
}

type Trailable interface {
	SetTrailer(metadata.MD)
}

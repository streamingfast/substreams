package pipeline

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/streamingfast/bstream/stream"
	"github.com/streamingfast/substreams/reqctx"
	"github.com/streamingfast/substreams/tracking"
	"go.uber.org/zap"
	"google.golang.org/grpc/metadata"
)

// OnStreamTerminated performs flush of store and setting trailers when the stream terminated gracefully from our point of view.
// If the stream terminated gracefully, we return `nil` otherwise, the original is returned.
func (p *Pipeline) OnStreamTerminated(ctx context.Context, streamSrv Trailable, err error) error {
	logger := reqctx.Logger(ctx)
	reqDetails := reqctx.Details(ctx)
	bytesMeter := tracking.GetBytesMeter(ctx)

	for _, executor := range p.moduleExecutors {
		executor.FreeMem()
	}

	if !errors.Is(err, stream.ErrStopBlockReached) && !errors.Is(err, io.EOF) {
		return err
	}

	logger.Info("stream of blocks ended",
		zap.Uint64("stop_block_num", reqDetails.Request.StopBlockNum),
		zap.Bool("eof", errors.Is(err, io.EOF)),
		zap.Bool("stop_block_reached", errors.Is(err, stream.ErrStopBlockReached)),
		//zap.Uint64("total_bytes_written", bytesMeter.BytesWritten()),
		//zap.Uint64("total_bytes_read", bytesMeter.BytesRead()),
	)

	// TODO(abourget): check, in the tier1, there might not be a `lastFinalClock`
	// if we just didn't run the `streamFactoryFunc`
	if err := p.execOutputCache.EndOfStream(p.lastFinalClock); err != nil {
		return fmt.Errorf("end of stream: %w", err)
	}

	if err := p.stores.flushStores(ctx, reqDetails.Request.StopBlockNum); err != nil {
		return fmt.Errorf("step new irr: stores end of stream: %w", err)
	}

	p.execOutputCache.Close()

	if err := bytesMeter.Send(ctx, p.respFunc); err != nil {
		return fmt.Errorf("sending bytes meter %w", err)
	}

	if p.stores.partialsWritten != nil {
		partialRanges := make([]string, len(p.stores.partialsWritten))
		for i, rng := range p.stores.partialsWritten {
			partialRanges[i] = fmt.Sprintf("%d-%d", rng.StartBlock, rng.ExclusiveEndBlock)
		}
		logger.Info("setting trailer", zap.Strings("ranges", partialRanges))
		streamSrv.SetTrailer(metadata.MD{"substreams-partials-written": []string{strings.Join(partialRanges, ",")}})
	}

	return nil
}

type Trailable interface {
	SetTrailer(metadata.MD)
}

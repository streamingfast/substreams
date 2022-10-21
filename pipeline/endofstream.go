package pipeline

import (
	"context"
	"errors"
	"fmt"
	"github.com/streamingfast/substreams/reqctx"
	"io"
	"strings"

	"github.com/streamingfast/bstream/stream"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// TODO(abourget): wuuutz that? In `error.go` we're doing critical return value handling (setting the `ranges` trailer based on data accumulated as a side-effeect in `partialsWritten`.
// We end up saying we're not responsible for doing error handling? But we're in `error.go` ! Who else otherwise?
// So many comments in here. Let's clean this up a bit.

// OnStreamTerminated performs flush of store and setting trailers when the stream terminated gracefully from our point of view.
// If the stream terminated gracefully, we return `nil` otherwise, the original is returned.
func (p *Pipeline) OnStreamTerminated(ctx context.Context, streamSrv pbsubstreams.Stream_BlocksServer, err error) error {
	logger := reqctx.Logger(ctx)
	reqDetails := reqctx.Details(ctx)
	isStopBlockReachedErr := errors.Is(err, stream.ErrStopBlockReached)

	if isStopBlockReachedErr || errors.Is(err, io.EOF) {
		if isStopBlockReachedErr {
			logger.Debug("stream of blocks reached end block, triggering StoreSave",
				zap.Uint64("stop_block_num", reqDetails.Request.StopBlockNum),
			)

			// We use `StopBlockNum` as the argument to flush stores as possible boundaries (if chain has holes...)
			//
			// `OnStreamTerminated` is invoked by the service when an error occurs with the connection, in this case,
			// we are outside any active span we want to attach the event to the root span of the pipeline
			// which should always be set.
			if err := p.flushStores(ctx, reqDetails.Request.StopBlockNum); err != nil {
				return status.Errorf(codes.Internal, "handling store save boundaries: %s", err)
			}
		}

		partialRanges := make([]string, len(p.partialsWritten))
		for i, rng := range p.partialsWritten {
			partialRanges[i] = fmt.Sprintf("%d-%d", rng.StartBlock, rng.ExclusiveEndBlock)
		}

		logger.Info("setting trailer", zap.Strings("ranges", partialRanges))
		streamSrv.SetTrailer(metadata.MD{"substreams-partials-written": []string{strings.Join(partialRanges, ",")}})

		// It was an ok error, so let's
		return nil
	}

	// We are not responsible of doing any other error handling here, caller will deal with them
	return err
}

package pipeline

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/streamingfast/bstream/stream"
	"go.uber.org/zap"

	"github.com/streamingfast/substreams/reqctx"
)

const progressMessageInterval = time.Millisecond * 200

// OnStreamTerminated performs flush of store and setting trailers when the stream terminated gracefully from our point of view.
// If the stream terminated gracefully, we return `nil` otherwise, the original is returned.
func (p *Pipeline) OnStreamTerminated(ctx context.Context, err error) error {
	logger := reqctx.Logger(ctx)
	reqDetails := reqctx.Details(ctx)

	if err := p.cleanUpModuleExecutors(ctx); err != nil {
		return err
	}

	p.runPostJobHooks(ctx, p.lastFinalClock)

	if !errors.Is(err, stream.ErrStopBlockReached) && !errors.Is(err, io.EOF) {
		if err == nil {
			err = fmt.Errorf("stream terminated without reaching the stop block")
		}
		return err
	}

	logger.Info("stream of blocks ended",
		zap.Uint64("stop_block_num", reqDetails.StopBlockNum),
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

	if err := p.stores.flushStores(ctx, p.executionStages, reqDetails.StopBlockNum); err != nil {
		return fmt.Errorf("step new irr: stores end of stream: %w", err)
	}

	if reqctx.Details(ctx).IsTier2Request {
		err := p.returnInternalModuleProgressOutputs(p.lastFinalClock, true)
		if err != nil {
			logger.Error("returning internal module progress outputs", zap.Error(err))
		}
	} else {
		err := p.returnRPCModuleProgressOutputs(true)
		if err != nil {
			logger.Error("returning internal module progress outputs", zap.Error(err))
		}
	}
	return nil
}

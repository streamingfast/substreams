package service

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/streamingfast/bstream"
	pbbstream "github.com/streamingfast/bstream/pb/sf/bstream/v1"
	"github.com/streamingfast/substreams/client"
	pbssinternal "github.com/streamingfast/substreams/pb/sf/substreams/intern/v2"
	"go.uber.org/zap"
)

type LiveBackFiller struct {
	NextHandler          bstream.Handler
	irreversibleBlock    chan uint64
	currentSegment       uint64
	segmentSize          uint64
	receivedBlockNumbers map[uint64]struct{}
}

func NewLiveBackFiller(nextHandler bstream.Handler, segmentSize uint64, linearHandoff uint64) *LiveBackFiller {
	return &LiveBackFiller{
		NextHandler:       nextHandler,
		irreversibleBlock: make(chan uint64),
		currentSegment:    linearHandoff / segmentSize,
		segmentSize:       segmentSize,
	}
}

func (l *LiveBackFiller) ProcessBlock(blk *pbbstream.Block, obj interface{}) (err error) {
	step := obj.(bstream.Stepable).Step()
	if !(step.Matches(bstream.StepIrreversible)) {
		return l.NextHandler.ProcessBlock(blk, obj)
	}

	l.irreversibleBlock <- blk.Number

	return l.NextHandler.ProcessBlock(blk, obj)
}

func (l *LiveBackFiller) RequestBackProcessing(ctx context.Context, logger *zap.Logger, liveCachingRequest *pbssinternal.ProcessRangeRequest, clientFactory client.InternalClientFactory) error {
	grpcClient, closeFunc, grpcCallOpts, _, err := clientFactory()
	if err != nil {
		logger.Warn("failed to create live cache grpc client", zap.Error(err))
	}

	zlog.Debug("request live back filling", zap.Uint64("start_block", liveCachingRequest.StartBlock()), zap.Uint64("end_block", liveCachingRequest.StopBlock()))
	stream, err := grpcClient.ProcessRange(ctx, liveCachingRequest, grpcCallOpts...)
	if err != nil {
		return fmt.Errorf("getting stream: %w", err)
	}

	doneCh := make(chan struct{})
	errCh := make(chan error)

	go func() {
		for {
			_, err := stream.Recv()
			if err != nil {
				if err == io.EOF {
					close(doneCh)
					return
				}
				errCh <- fmt.Errorf("receiving stream: %w", err)
			}
		}
	}()

	select {
	case <-time.After(10 * time.Second):
		return fmt.Errorf("time exceeded")
	case err = <-errCh:
		return err
	case <-doneCh:
		break
	}

	defer func() {
		if err = stream.CloseSend(); err != nil {
			logger.Warn("closing stream", zap.Error(err))
		}
		if err = closeFunc(); err != nil {
			logger.Warn("closing stream", zap.Error(err))
		}
	}()

	closeFunc()

	return nil
}

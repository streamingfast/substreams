package service

import (
	"context"
	"fmt"
	"io"

	"github.com/streamingfast/derr"
	"github.com/streamingfast/substreams/block"
	"github.com/streamingfast/substreams/orchestrator/work"
	"github.com/streamingfast/substreams/reqctx"

	"github.com/streamingfast/bstream"
	pbbstream "github.com/streamingfast/bstream/pb/sf/bstream/v1"
	"github.com/streamingfast/substreams/client"
	pbssinternal "github.com/streamingfast/substreams/pb/sf/substreams/intern/v2"
	"go.uber.org/zap"
)

type RequestBackProcessingFunc = func(ctx context.Context, logger *zap.Logger, blockRange *block.Range, stageToProcess int, clientFactory client.InternalClientFactory, jobCompleted chan struct{}, jobFailed *bool)

type LiveBackFiller struct {
	RequestBackProcessing RequestBackProcessingFunc
	NextHandler           bstream.Handler
	irreversibleBlock     chan uint64
	currentSegment        uint64
	segmentSize           uint64
	receivedBlockNumbers  map[uint64]struct{}
	logger                *zap.Logger
	stageToProcess        int
	clientFactory         client.InternalClientFactory
}

func NewLiveBackFiller(nextHandler bstream.Handler, logger *zap.Logger, stageToProcess int, segmentSize uint64, linearHandoff uint64, clientFactory client.InternalClientFactory, requestBackProcessing RequestBackProcessingFunc) *LiveBackFiller {
	return &LiveBackFiller{
		RequestBackProcessing: requestBackProcessing,
		stageToProcess:        stageToProcess,
		NextHandler:           nextHandler,
		irreversibleBlock:     make(chan uint64),
		currentSegment:        linearHandoff / segmentSize,
		segmentSize:           segmentSize,
		logger:                logger,
		clientFactory:         clientFactory,
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

func RequestBackProcessing(ctx context.Context, logger *zap.Logger, blockRange *block.Range, stageToProcess int, clientFactory client.InternalClientFactory, jobCompleted chan struct{}, jobFailed *bool) {
	liveBackFillerRequest := work.NewRequest(ctx, reqctx.Details(ctx), stageToProcess, blockRange)

	err := derr.RetryContext(ctx, 999, func(ctx context.Context) error {
		err := requestBackProcessing(ctx, logger, liveBackFillerRequest, clientFactory)
		if err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		*jobFailed = true
		logger.Warn("job failed while processing live caching", zap.Error(err), zap.Uint64("segment_processed", liveBackFillerRequest.SegmentNumber))
	}

	jobCompleted <- struct{}{}
}

func requestBackProcessing(ctx context.Context, logger *zap.Logger, liveCachingRequest *pbssinternal.ProcessRangeRequest, clientFactory client.InternalClientFactory) error {
	grpcClient, closeFunc, grpcCallOpts, _, err := clientFactory()
	if err != nil {
		logger.Warn("failed to create live cache grpc client", zap.Error(err))
	}

	zlog.Debug("request live back filling", zap.Uint64("start_block", liveCachingRequest.StartBlock()), zap.Uint64("end_block", liveCachingRequest.StopBlock()))
	stream, err := grpcClient.ProcessRange(ctx, liveCachingRequest, grpcCallOpts...)
	if err != nil {
		return fmt.Errorf("getting stream: %w", err)
	}

	for {
		_, err = stream.Recv()
		if err != nil {
			if err == io.EOF {
				break
			}
		}
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

func (l *LiveBackFiller) Start(ctx context.Context) {
	l.logger.Info("start live back filler", zap.Uint64("current_segment", l.currentSegment))

	var targetSegment uint64
	var jobFailed bool
	var jobProcessing bool
	var blockNumber uint64
	jobCompleted := make(chan struct{})
	for {
		select {
		case <-ctx.Done():
			return
		case <-jobCompleted:
			jobProcessing = false
			l.currentSegment++
		case blockNumber = <-l.irreversibleBlock:
			targetSegment = blockNumber / l.segmentSize
		}

		if jobFailed {
			// We don't want to run more jobs if one has failed permanently
			continue
		}

		if jobProcessing {
			continue
		}

		segmentStart := l.currentSegment * l.segmentSize
		segmentEnd := (l.currentSegment + 1) * l.segmentSize
		mergedBlockIsWritten := (blockNumber - segmentStart) > 120

		if (targetSegment > l.currentSegment) && mergedBlockIsWritten {

			liveBackFillerRange := block.NewRange(segmentStart, segmentEnd)

			jobProcessing = true
			go l.RequestBackProcessing(ctx, l.logger, liveBackFillerRange, l.stageToProcess, l.clientFactory, jobCompleted, &jobFailed)
		}
	}
}

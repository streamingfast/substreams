package service

import (
	"context"
	"fmt"
	"io"

	"github.com/streamingfast/dauth"

	"github.com/streamingfast/bstream"
	pbbstream "github.com/streamingfast/bstream/pb/sf/bstream/v1"
	"github.com/streamingfast/derr"
	"github.com/streamingfast/substreams/client"
	"github.com/streamingfast/substreams/orchestrator/work"
	pbssinternal "github.com/streamingfast/substreams/pb/sf/substreams/intern/v2"
	"github.com/streamingfast/substreams/reqctx"
	"go.uber.org/zap"
)

const finalBlockDelay = 120
const backfillRetries = 999 // no point in failing "early". It may be failing because merged blocks are lagging behind a little bit.

type RequestBackProcessingFunc = func(ctx context.Context, logger *zap.Logger, startBlock uint64, stageToProcess int, clientFactory client.InternalClientFactory, jobCompleted chan error)

type LiveBackFiller struct {
	RequestBackProcessing RequestBackProcessingFunc
	NextHandler           bstream.Handler
	irreversibleBlock     chan uint64
	currentSegment        uint64
	segmentSize           uint64
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

func RequestBackProcessing(ctx context.Context, logger *zap.Logger, startBlock uint64, stageToProcess int, clientFactory client.InternalClientFactory, jobResult chan error) {
	liveBackFillerRequest := work.NewRequest(ctx, reqctx.Details(ctx), stageToProcess, startBlock)

	err := derr.RetryContext(ctx, backfillRetries, func(ctx context.Context) error {
		err := requestBackProcessing(ctx, logger, liveBackFillerRequest, clientFactory)
		if err != nil {
			logger.Debug("retryable error while live backprocessing", zap.Error(err))
			return err
		}

		return nil
	})

	jobResult <- err
}

func requestBackProcessing(ctx context.Context, logger *zap.Logger, liveCachingRequest *pbssinternal.ProcessRangeRequest, clientFactory client.InternalClientFactory) error {
	zlog.Debug("request live back filling", zap.Uint64("start_block", liveCachingRequest.StartBlock()), zap.Uint64("end_block", liveCachingRequest.StopBlock()))

	grpcClient, closeFunc, grpcCallOpts, _, err := clientFactory()
	if err != nil {
		return fmt.Errorf("failed to create live cache grpc client: %w", err)
	}

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
			return err
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
	ctx = dauth.FromContext(ctx).ToOutgoingGRPCContext(ctx)
	ctx = reqctx.WithBackfillerRequest(ctx)

	l.logger.Info("start live back filler", zap.Uint64("current_segment", l.currentSegment))

	var targetSegment uint64
	var jobFailed bool
	var jobProcessing bool
	var blockNumber uint64
	jobResult := make(chan error)
	for {
		select {
		case <-ctx.Done():
			return
		case err := <-jobResult:
			if err != nil {
				l.logger.Warn("job failed while processing live caching", zap.Error(err), zap.Uint64("segment_processed", l.currentSegment))
				jobFailed = true
				break
			}
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
		mergedBlockIsWritten := (blockNumber - segmentEnd) > finalBlockDelay

		if (targetSegment > l.currentSegment) && mergedBlockIsWritten {
			jobProcessing = true
			go l.RequestBackProcessing(ctx, l.logger, segmentStart, l.stageToProcess, l.clientFactory, jobResult)
		}
	}
}

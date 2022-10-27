package work

import (
	"context"
	"fmt"
	"io"

	"github.com/streamingfast/substreams"
	"github.com/streamingfast/substreams/block"
	"github.com/streamingfast/substreams/client"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/reqctx"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	ttrace "go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"google.golang.org/grpc/metadata"
)

type JobRunner func(ctx context.Context, request *pbsubstreams.Request, respFunc substreams.ResponseFunc) (partialsWritten []*block.Range, err error)

// The tracer will be provided by the worker pool, on worker creation
type WorkerFactory = func(logger *zap.Logger) JobRunner

type RemoteWorker struct {
	clientFactory client.Factory
	tracer        ttrace.Tracer
	logger        *zap.Logger
}

func NewRemoteWorker(clientFactory client.Factory, logger *zap.Logger) *RemoteWorker {
	return &RemoteWorker{
		clientFactory: clientFactory,
		tracer:        otel.GetTracerProvider().Tracer("worker"),
		logger:        logger,
	}
}

//job *Job, requestModules *pbsubstreams.Modules
func (w *RemoteWorker) Run(ctx context.Context, request *pbsubstreams.Request, respFunc substreams.ResponseFunc) (ranges []*block.Range, err error) {
	ctx, span := reqctx.WithSpan(ctx, "running_job")
	defer span.EndWithErr(&err)
	span.SetAttributes(attribute.StringSlice("module_names", request.OutputModules))
	span.SetAttributes(attribute.Int64("start_block", int64(request.StartBlockNum)))
	span.SetAttributes(attribute.Int64("stop_block", int64(request.StopBlockNum)))
	logger := w.logger

	logger.Info("creating gprc client")
	w.logger.Info("creating gprc client")
	grpcClient, closeFunc, grpcCallOpts, err := w.clientFactory()
	if err != nil {
		return nil, fmt.Errorf("failed to create Substreams client: %w", err)
	}

	ctx = metadata.NewOutgoingContext(ctx, metadata.New(map[string]string{"substreams-partial-mode": "true"}))

	stream, err := grpcClient.Blocks(ctx, request, grpcCallOpts...)
	if err != nil {
		if ctx.Err() != nil {
			return nil, err
		}
		return nil, &RetryableErr{cause: fmt.Errorf("getting block stream: %w", err)}
	}
	defer func() {
		if err := stream.CloseSend(); err != nil {
			logger.Warn("failed to close stream on job termination", zap.Error(err))
		}
		if err := closeFunc(); err != nil {
			logger.Warn("failed to close grpc client on job termination", zap.Error(err))
		}
	}()

	meta, err := stream.Header()
	if err != nil {
		logger.Warn("error getting stream header", zap.Error(err))
	}
	remoteHostname := "unknown"
	if hosts := meta.Get("host"); len(hosts) != 0 {
		remoteHostname = hosts[0]
		logger = logger.With(zap.String("remote_hostname", remoteHostname))
	}

	span.SetAttributes(attribute.String("remote_hostname", remoteHostname))

	for {
		select {
		case <-ctx.Done():
			err := ctx.Err()
			if err != nil {
				return nil, err
			}
			return nil, nil
		default:
		}

		resp, err := stream.Recv()
		if resp != nil {
			switch r := resp.Message.(type) {
			case *pbsubstreams.Response_Progress:

				err := respFunc(resp)
				if err != nil {
					span.SetStatus(codes.Error, err.Error())
					return nil, &RetryableErr{cause: fmt.Errorf("sending progress: %w", err)}
				}

				for _, progress := range resp.GetProgress().Modules {
					if f := progress.GetFailed(); f != nil {
						err := fmt.Errorf("module %s failed on host: %s", progress.Name, f.Reason)
						span.SetStatus(codes.Error, err.Error())
						return nil, err
					}
				}

				//if len(resp.GetProgress().Modules) > 0 {
				//	module := resp.GetProgress().Modules[0]
				//	if rangeCount := len(module.GetProcessedRanges().ProcessedRanges); rangeCount > 0 {
				//		endBlock := module.GetProcessedRanges().ProcessedRanges[rangeCount-1].EndBlock
				//	}
				//}

			case *pbsubstreams.Response_SnapshotData:
				_ = r.SnapshotData
			case *pbsubstreams.Response_SnapshotComplete:
				_ = r.SnapshotComplete
			case *pbsubstreams.Response_Data:
				// These are not returned by virtue of `returnOutputs`
			}
		}

		if err != nil {
			if err == io.EOF {
				logger.Info("worker done")
				trailers := stream.Trailer().Get("substreams-partials-written")
				var partialsWritten []*block.Range
				if len(trailers) != 0 {
					logger.Info("partial written", zap.String("trailer", trailers[0]))
					partialsWritten = block.ParseRanges(trailers[0])
				}
				return partialsWritten, nil
			}
			return nil, &RetryableErr{cause: fmt.Errorf("receiving stream resp: %w", err)}
		}
	}
}

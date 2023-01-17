package work

import (
	"context"
	"fmt"
	"io"
	"sync/atomic"

	"github.com/streamingfast/substreams"
	"github.com/streamingfast/substreams/block"
	"github.com/streamingfast/substreams/client"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/reqctx"
	"github.com/streamingfast/substreams/tracking"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	ttrace "go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"google.golang.org/grpc/metadata"
)

var lastWorkerID uint64

type Result struct {
	PartialsWritten []*block.Range
	Error           error
}

type Worker interface {
	ID() string
	Work(ctx context.Context, request *pbsubstreams.Request, respFunc substreams.ResponseFunc) *Result
}

func NewWorkerFactoryFromFunc(f func(ctx context.Context, request *pbsubstreams.Request, respFunc substreams.ResponseFunc) *Result) *SimpleWorkerFactory {
	return &SimpleWorkerFactory{
		f:  f,
		id: atomic.AddUint64(&lastWorkerID, 1),
	}
}

type SimpleWorkerFactory struct {
	f  func(ctx context.Context, request *pbsubstreams.Request, respFunc substreams.ResponseFunc) *Result
	id uint64
}

func (f SimpleWorkerFactory) Work(ctx context.Context, request *pbsubstreams.Request, respFunc substreams.ResponseFunc) *Result {
	return f.f(ctx, request, respFunc)
}

func (f SimpleWorkerFactory) ID() string {
	return fmt.Sprintf("%d", f.id)
}

// The tracer will be provided by the worker pool, on worker creation
type WorkerFactory = func(logger *zap.Logger) Worker

type RemoteWorker struct {
	clientFactory client.Factory
	tracer        ttrace.Tracer
	logger        *zap.Logger
	id            uint64
}

func NewRemoteWorker(clientFactory client.Factory, logger *zap.Logger) *RemoteWorker {
	return &RemoteWorker{
		clientFactory: clientFactory,
		tracer:        otel.GetTracerProvider().Tracer("worker"),
		logger:        logger,
		id:            atomic.AddUint64(&lastWorkerID, 1),
	}
}

func (w *RemoteWorker) ID() string {
	return fmt.Sprintf("%d", w.id)
}

func (w *RemoteWorker) Work(ctx context.Context, request *pbsubstreams.Request, respFunc substreams.ResponseFunc) *Result {
	var err error
	ctx, span := reqctx.WithSpan(ctx, "running_job")
	defer span.EndWithErr(&err)
	span.SetAttributes(attribute.String("output_module", request.MustGetOutputModuleName()))
	span.SetAttributes(attribute.Int64("start_block", request.StartBlockNum))
	span.SetAttributes(attribute.Int64("stop_block", int64(request.StopBlockNum)))
	logger := w.logger

	grpcClient, closeFunc, grpcCallOpts, err := w.clientFactory()
	if err != nil {
		return &Result{
			Error: fmt.Errorf("unable to create grpc client: %w", err),
		}
	}

	ctx = metadata.NewOutgoingContext(ctx, metadata.New(map[string]string{"substreams-partial-mode": "true"}))

	w.logger.Info("launching remote worker",
		zap.Int64("start_block_num", request.StartBlockNum),
		zap.Uint64("stop_block_num", request.StopBlockNum),
		zap.String("output_module", request.MustGetOutputModuleName()),
	)

	stream, err := grpcClient.Blocks(ctx, request, grpcCallOpts...)
	if err != nil {
		if ctx.Err() != nil {
			return &Result{
				Error: ctx.Err(),
			}
		}
		return &Result{
			Error: &RetryableErr{cause: fmt.Errorf("getting block stream: %w", err)},
		}
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
		if ctx.Err() != nil {
			return &Result{
				Error: ctx.Err(),
			}
		}
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
			return &Result{
				Error: ctx.Err(),
			}
		default:
		}

		resp, err := stream.Recv()
		if resp != nil {
			switch r := resp.Message.(type) {
			case *pbsubstreams.Response_Progress:
				err := respFunc(resp)
				if err != nil {
					span.SetStatus(codes.Error, err.Error())
					return &Result{
						Error: &RetryableErr{cause: fmt.Errorf("sending progress: %w", err)},
					}
				}

				for _, progress := range resp.GetProgress().Modules {
					if f := progress.GetProcessedBytes(); f != nil {
						bm := tracking.GetBytesMeter(ctx)
						bm.AddBytesWritten(int(f.TotalBytesWritten))
						bm.AddBytesRead(int(f.TotalBytesRead))
					}

					if f := progress.GetFailed(); f != nil {
						err := fmt.Errorf("module %s failed on host: %s", progress.Name, f.Reason)
						span.SetStatus(codes.Error, err.Error())
						return &Result{
							Error: err,
						}
					}
				}
			case *pbsubstreams.Response_DebugSnapshotData:
				_ = r.DebugSnapshotData
			case *pbsubstreams.Response_DebugSnapshotComplete:
				_ = r.DebugSnapshotComplete
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
				return &Result{
					PartialsWritten: partialsWritten,
				}
			}
			return &Result{
				Error: &RetryableErr{cause: fmt.Errorf("receiving stream resp: %w", err)},
			}
		}
	}
}

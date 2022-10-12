package orchestrator

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/streamingfast/substreams"
	"github.com/streamingfast/substreams/block"
	"github.com/streamingfast/substreams/client"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	ttrace "go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

type Worker interface {
	Run(ctx context.Context, job *Job, requestModules *pbsubstreams.Modules, respFunc substreams.ResponseFunc) ([]*block.Range, error)
}

type NewWorkerFunc func(tracer trace.Tracer) Worker

type RemoteWorker struct {
	callOpts           []grpc.CallOption
	tracer             ttrace.Tracer
	newSubstreamClient client.NewSubstreamClient
}

func NewRemoteWorker(newSubstreamClient client.NewSubstreamClient) *RemoteWorker {
	return &RemoteWorker{
		newSubstreamClient: newSubstreamClient,
	}
}

func (w *RemoteWorker) Run(ctx context.Context, job *Job, requestModules *pbsubstreams.Modules, respFunc substreams.ResponseFunc) ([]*block.Range, error) {
	ctx, span := w.tracer.Start(ctx, "running_job")
	span.SetAttributes(attribute.String("module_name", job.ModuleName))
	span.SetAttributes(attribute.Int64("start_block", int64(job.requestRange.StartBlock)))
	span.SetAttributes(attribute.Int64("stop_block", int64(job.requestRange.ExclusiveEndBlock)))
	defer span.End()
	start := time.Now()

	zlog.Info("creating gprc client")
	grpcClient, closeFunc, grpcCallOpts, err := w.newSubstreamClient()
	if err != nil {
		return nil, fmt.Errorf("failed to create Substreams client: %w", err)
	}

	jobLogger := zlog.With(zap.Object("job", job))

	ctx = metadata.NewOutgoingContext(ctx, metadata.New(map[string]string{"substreams-partial-mode": "true"}))

	request := job.CreateRequest(requestModules)

	stream, err := grpcClient.Blocks(ctx, request, grpcCallOpts...)
	if err != nil {
		if ctx.Err() != nil {
			return nil, err
		}
		span.SetStatus(codes.Error, err.Error())
		return nil, &RetryableErr{cause: fmt.Errorf("getting block stream: %w", err)}
	}
	defer func() {
		if err := stream.CloseSend(); err != nil {
			jobLogger.Warn("failed to close stream on job termination", zap.Error(err))
		}
		if err := closeFunc(); err != nil {
			jobLogger.Warn("failed to close grpc client on job termination", zap.Error(err))
		}
	}()

	meta, err := stream.Header()
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		jobLogger.Warn("error getting stream header", zap.Error(err))
	}
	remoteHostname := "unknown"
	if hosts := meta.Get("host"); len(hosts) != 0 {
		remoteHostname = hosts[0]
		jobLogger = jobLogger.With(zap.String("remote_hostname", remoteHostname))
	}
	span.SetAttributes(attribute.String("remote_hostname", remoteHostname))

	jobLogger.Info("running job", zap.Object("job", job))
	defer func() {
		jobLogger.Info("job completed", zap.Object("job", job), zap.Duration("in", time.Since(start)))
	}()

	for {
		select {
		case <-ctx.Done():
			err := ctx.Err()
			if err != nil {
				span.SetStatus(codes.Error, err.Error())
				return nil, err
			}
			span.SetStatus(codes.Ok, "context cancelled")
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
				jobLogger.Info("worker done", zap.Object("job", job))
				trailers := stream.Trailer().Get("substreams-partials-written")
				var partialsWritten []*block.Range
				if len(trailers) != 0 {
					jobLogger.Info("partial written", zap.String("trailer", trailers[0]))
					partialsWritten = block.ParseRanges(trailers[0])
				}
				span.SetStatus(codes.Ok, "done")
				return partialsWritten, nil
			}
			span.SetStatus(codes.Error, err.Error())
			return nil, &RetryableErr{cause: fmt.Errorf("receiving stream resp: %w", err)}
		}
	}
}

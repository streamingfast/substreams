package work

import (
	"context"
	"fmt"
	"io"
	"sync/atomic"

	"github.com/streamingfast/substreams"
	"github.com/streamingfast/substreams/client"
	pbssinternal "github.com/streamingfast/substreams/pb/sf/substreams/intern/v2"
	pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
	"github.com/streamingfast/substreams/reqctx"
	"github.com/streamingfast/substreams/storage/store"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	ttrace "go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	grpcCodes "google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

var lastWorkerID uint64

type Result struct {
	PartialFilesWritten store.FileInfos
	Error               error
}

type Worker interface {
	ID() string
	Work(ctx context.Context, request *pbssinternal.ProcessRangeRequest, respFunc substreams.ResponseFunc) *Result
}

func NewWorkerFactoryFromFunc(f func(ctx context.Context, request *pbssinternal.ProcessRangeRequest, respFunc substreams.ResponseFunc) *Result) *SimpleWorkerFactory {
	return &SimpleWorkerFactory{
		f:  f,
		id: atomic.AddUint64(&lastWorkerID, 1),
	}
}

type SimpleWorkerFactory struct {
	f  func(ctx context.Context, request *pbssinternal.ProcessRangeRequest, respFunc substreams.ResponseFunc) *Result
	id uint64
}

func (f SimpleWorkerFactory) Work(ctx context.Context, request *pbssinternal.ProcessRangeRequest, respFunc substreams.ResponseFunc) *Result {
	return f.f(ctx, request, respFunc)
}

func (f SimpleWorkerFactory) ID() string {
	return fmt.Sprintf("%d", f.id)
}

// The tracer will be provided by the worker pool, on worker creation
type WorkerFactory = func(logger *zap.Logger) Worker

type RemoteWorker struct {
	clientFactory client.InternalClientFactory
	tracer        ttrace.Tracer
	logger        *zap.Logger
	id            uint64
}

func NewRemoteWorker(clientFactory client.InternalClientFactory, logger *zap.Logger) *RemoteWorker {
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

func (w *RemoteWorker) Work(ctx context.Context, request *pbssinternal.ProcessRangeRequest, respFunc substreams.ResponseFunc) *Result {
	var err error

	ctx, span := reqctx.WithSpan(ctx, fmt.Sprintf("substreams/tier1/schedule/%s/%d-%d", request.OutputModule, request.StartBlockNum, request.StopBlockNum))
	defer span.EndWithErr(&err)
	span.SetAttributes(
		attribute.String("substreams.output_module", request.OutputModule),
		attribute.Int64("substreams.start_block", int64(request.StartBlockNum)),
		attribute.Int64("substreams.stop_block", int64(request.StopBlockNum)),
		attribute.Int64("substreams.worker_id", int64(w.id)),
	)
	logger := w.logger

	grpcClient, closeFunc, grpcCallOpts, err := w.clientFactory()
	if err != nil {
		return &Result{
			Error: fmt.Errorf("unable to create grpc client: %w", err),
		}
	}

	ctx = metadata.NewOutgoingContext(ctx, metadata.New(map[string]string{"substreams-partial-mode": "true"}))

	w.logger.Info("launching remote worker",
		zap.Int64("start_block_num", int64(request.StartBlockNum)),
		zap.Uint64("stop_block_num", request.StopBlockNum),
		zap.String("output_module", request.OutputModule),
	)

	stream, err := grpcClient.ProcessRange(ctx, request, grpcCallOpts...)
	if err != nil {
		if ctx.Err() != nil {
			return &Result{
				Error: ctx.Err(),
			}
		}
		return &Result{
			Error: NewRetryableErr(fmt.Errorf("getting block stream: %w", err)),
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

	span.SetAttributes(attribute.String("substreams.remote_hostname", remoteHostname))

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
			switch r := resp.Type.(type) {
			case *pbssinternal.ProcessRangeResponse_ProcessedRange:
				forwardResponse := toRPCRangeProgressResponse(resp.ModuleName, r.ProcessedRange.StartBlock, r.ProcessedRange.EndBlock)
				err := respFunc(forwardResponse)
				if err != nil {
					if ctx.Err() != nil {
						return &Result{
							Error: ctx.Err(),
						}
					}
					span.SetStatus(codes.Error, err.Error())
					return &Result{
						Error: NewRetryableErr(fmt.Errorf("sending progress: %w", err)),
					}
				}

			case *pbssinternal.ProcessRangeResponse_ProcessedBytes:
				// commented out while these message are causing issues
				//bm := tracking.GetBytesMeter(ctx)
				//bm.AddBytesWritten(int(r.ProcessedBytes.BytesWrittenDelta))
				//bm.AddBytesRead(int(r.ProcessedBytes.BytesReadDelta))
				//respFunc(toRPCProcessedBytes(resp.ModuleName, bm.BytesReadDelta(), bm.BytesWrittenDelta(), bm.BytesRead(), bm.BytesWritten(), 0))

			case *pbssinternal.ProcessRangeResponse_Failed:
				// FIXME(abourget): we do NOT emit those Failed objects anymore. There was a flow
				// for that that would pick up the errors, and pack the remaining logs
				// and reasons into a message. This is nowhere to be found now.
				forwardResponse := toRPCFailedProgressResponse(resp.ModuleName, r.Failed.Reason, r.Failed.Logs, r.Failed.LogsTruncated)
				respFunc(forwardResponse)
				err := fmt.Errorf("module %s failed on host: %s", resp.ModuleName, r.Failed.Reason)
				span.SetStatus(codes.Error, err.Error())
				return &Result{
					Error: err,
				}

			case *pbssinternal.ProcessRangeResponse_Completed:
				logger.Info("worker done")
				return &Result{
					PartialFilesWritten: toRPCPartialFiles(r.Completed),
				}
			}
		}

		if err != nil {
			if err == io.EOF {
				return &Result{}
			}
			if ctx.Err() != nil {
				return &Result{
					Error: ctx.Err(),
				}
			}
			if s, ok := status.FromError(err); ok {
				if s.Code() == grpcCodes.InvalidArgument {
					return &Result{
						Error: err,
					}
				}
			}
			return &Result{
				Error: NewRetryableErr(fmt.Errorf("receiving stream resp: %w", err)),
			}
		}
	}
}
func toRPCFailedProgressResponse(moduleName, reason string, logs []string, logsTruncated bool) *pbsubstreamsrpc.Response {
	return &pbsubstreamsrpc.Response{
		Message: &pbsubstreamsrpc.Response_Progress{
			Progress: &pbsubstreamsrpc.ModulesProgress{
				Modules: []*pbsubstreamsrpc.ModuleProgress{
					{
						Name: moduleName,
						Type: &pbsubstreamsrpc.ModuleProgress_Failed_{
							Failed: &pbsubstreamsrpc.ModuleProgress_Failed{
								Reason:        reason,
								Logs:          logs,
								LogsTruncated: logsTruncated,
							},
						},
					},
				},
			},
		},
	}
}

func toRPCProcessedBytes(
	moduleName string,
	bytesReadDelta uint64,
	bytesWrittenDelta uint64,
	totalBytesRead uint64,
	totalBytesWritten uint64,
	nanoSeconds uint64,
) *pbsubstreamsrpc.Response {
	return &pbsubstreamsrpc.Response{
		Message: &pbsubstreamsrpc.Response_Progress{
			Progress: &pbsubstreamsrpc.ModulesProgress{
				Modules: []*pbsubstreamsrpc.ModuleProgress{
					{
						Name: moduleName,
						Type: &pbsubstreamsrpc.ModuleProgress_ProcessedBytes_{
							ProcessedBytes: &pbsubstreamsrpc.ModuleProgress_ProcessedBytes{
								BytesReadDelta:    bytesReadDelta,
								BytesWrittenDelta: bytesWrittenDelta,
								TotalBytesRead:    totalBytesRead,
								TotalBytesWritten: totalBytesWritten,
								NanoSecondsDelta:  nanoSeconds,
							},
						},
					},
				},
			},
		},
	}
}

func toRPCRangeProgressResponse(moduleName string, start, end uint64) *pbsubstreamsrpc.Response {
	return &pbsubstreamsrpc.Response{
		Message: &pbsubstreamsrpc.Response_Progress{
			Progress: &pbsubstreamsrpc.ModulesProgress{
				Modules: []*pbsubstreamsrpc.ModuleProgress{
					{
						Name: moduleName,
						Type: &pbsubstreamsrpc.ModuleProgress_ProcessedRanges_{
							ProcessedRanges: &pbsubstreamsrpc.ModuleProgress_ProcessedRanges{
								ProcessedRanges: []*pbsubstreamsrpc.BlockRange{
									{
										StartBlock: start,
										EndBlock:   end,
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

func toRPCPartialFiles(completed *pbssinternal.Completed) (out store.FileInfos) {
	out = make(store.FileInfos, len(completed.AllProcessedRanges))
	for i, b := range completed.AllProcessedRanges {
		out[i] = store.NewPartialFileInfo(b.StartBlock, b.EndBlock, completed.TraceId)
	}
	return
}

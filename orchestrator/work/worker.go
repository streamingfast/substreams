package work

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/streamingfast/bstream"
	"github.com/streamingfast/dauth"
	"github.com/streamingfast/derr"
	"github.com/streamingfast/dgrpc"
	"github.com/streamingfast/substreams/client"
	"github.com/streamingfast/substreams/metrics"
	"github.com/streamingfast/substreams/orchestrator/loop"
	"github.com/streamingfast/substreams/orchestrator/response"
	"github.com/streamingfast/substreams/orchestrator/stage"
	pbssinternal "github.com/streamingfast/substreams/pb/sf/substreams/intern/v2"
	pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
	"github.com/streamingfast/substreams/reqctx"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	otelCodes "go.opentelemetry.io/otel/codes"
	ttrace "go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
)

var lastWorkerID uint64

type Result struct {
	Error error
}

type Worker interface {
	ID() string
	Work(ctx context.Context, unit stage.Unit, startBlock uint64, moduleNames []string, upstream *response.Stream, streamOutput bool) loop.Cmd // *Result
}

type RemoteWorker struct {
	clientFactory client.InternalClientFactory
	tracer        ttrace.Tracer
	logger        *zap.Logger
	id            string
}

func NewRemoteWorker(clientFactory client.InternalClientFactory, id string, logger *zap.Logger) *RemoteWorker {
	logger = logger.Named("remote-worker")
	return &RemoteWorker{
		clientFactory: clientFactory,
		tracer:        otel.GetTracerProvider().Tracer("worker"),
		logger:        logger,
		id:            id,
	}
}

func (w *RemoteWorker) ID() string {
	return w.id
}

func NewRequest(ctx context.Context, req *reqctx.RequestDetails, stageIndex int, startBlock uint64, streamOutput bool) *pbssinternal.ProcessRangeRequest {
	tier2ReqParams, ok := reqctx.GetTier2RequestParameters(ctx)
	if !ok {
		panic("unable to get tier2 request parameters")
	}

	segment := startBlock / tier2ReqParams.StateBundleSize

	return &pbssinternal.ProcessRangeRequest{
		Modules:              req.Modules,
		OutputModule:         req.OutputModule,
		Stage:                uint32(stageIndex),
		MeteringConfig:       tier2ReqParams.MeteringConfig,
		FirstStreamableBlock: tier2ReqParams.FirstStreamableBlock,
		MergedBlocksStore:    tier2ReqParams.MergedBlockStoreURL,
		StateStore:           tier2ReqParams.StateStoreURL,
		SegmentSize:          tier2ReqParams.StateBundleSize,
		SegmentNumber:        segment,
		StateStoreDefaultTag: tier2ReqParams.StateStoreDefaultTag,
		WasmExtensionConfigs: tier2ReqParams.WASMModules,
		BlockType:            tier2ReqParams.BlockType,
		ProductionMode:       req.ProductionMode,
		StreamOutput:         streamOutput,
	}
}

var workerMaxRetries = 5
var workerMaxTimeoutRetries = 2

func init() {
	if val := os.Getenv("SUBSTREAMS_WORKER_MAX_RETRIES"); val != "" {
		if parsed, err := strconv.Atoi(val); err == nil {
			workerMaxRetries = parsed
		}
	}

	if val := os.Getenv("SUBSTREAMS_WORKER_MAX_TIMEOUT_RETRIES"); val != "" {
		if parsed, err := strconv.Atoi(val); err == nil {
			workerMaxTimeoutRetries = parsed
		}
	}
}

func (w *RemoteWorker) Work(ctx context.Context, unit stage.Unit, startBlock uint64, moduleNames []string, upstream *response.Stream, streamOutput bool) loop.Cmd {
	request := NewRequest(ctx, reqctx.Details(ctx), unit.Stage, startBlock, streamOutput)
	logger := reqctx.Logger(ctx)

	return func() loop.Msg {
		var res *Result
		retryIdx := 0
		startTime := time.Now()
		maxRetries := workerMaxRetries
		maxExecutionTimeouts := workerMaxTimeoutRetries
		executionTimeouts := 0

		stats := reqctx.ReqStats(ctx)
		startBlock := request.SegmentNumber * request.SegmentSize
		jobIdx := stats.RecordNewSubrequest(request.Stage, startBlock, startBlock+request.SegmentSize)

		var previousError error
		err := derr.RetryContext(ctx, math.MaxUint64, func(ctx context.Context) error {
			w.logger.Info("launching remote worker",
				zap.Uint64("segment", request.SegmentNumber),
				zap.Uint32("stage", request.Stage),
				zap.String("output_module", request.OutputModule),
				zap.Int("attempt", retryIdx+1),
				zap.Int("execution_timeouts", executionTimeouts),
				zap.NamedError("previous_error", previousError),
			)

			res = w.work(ctx, request, moduleNames, upstream, jobIdx)
			err := res.Error
			switch err.(type) {
			case *RetryableErr:
				previousError = err
				if strings.Contains(err.Error(), "service currently overloaded") || // previous tier2 behavior, for backward compatibility, replaced by codes.ResourceExhausted
					errors.Is(err, ErrConnectionRefused) { // tier2 unavailable
					stats.RecordJobDelayed(jobIdx)
					metrics.Tier1WorkerRejectedOverloadedCounter.Inc()
					// don't count towards maxRetries, retry immediately
					return err

				}
				if grpcError := dgrpc.AsGRPCError(err); grpcError != nil && grpcError.Code() == codes.ResourceExhausted { // tier2 overloaded
					stats.RecordJobDelayed(jobIdx)
					metrics.Tier1WorkerRejectedOverloadedCounter.Inc()
					// don't count towards maxRetries, retry immediately
					return err
				}

				if streamOutput {
					// never retry for jobs that stream blocks, we don't know how much data they already sent
					segmentStart := request.SegmentNumber * request.SegmentSize
					return derr.NewFatalError(fmt.Errorf("segment [%d-%d] failed while streaming data: %w", segmentStart, segmentStart+request.SegmentSize, err))
				}
				metrics.Tier1WorkerRetryCounter.Inc()
				stats.RecordJobRetried(jobIdx)

				grpcError := dgrpc.AsGRPCError(err)
				if (grpcError != nil && grpcError.Code() == codes.DeadlineExceeded) || strings.Contains(err.Error(), "DeadlineExceeded") {
					executionTimeouts++
					if executionTimeouts >= maxExecutionTimeouts {
						segmentStart := request.SegmentNumber * request.SegmentSize
						return derr.NewFatalError(fmt.Errorf("segment [%d-%d] timed out %d times, giving up. Last error from worker: %s", segmentStart, segmentStart+request.SegmentSize, executionTimeouts, err))
					}
					return err
				}

				retryIdx++
				if retryIdx >= maxRetries {
					segmentStart := request.SegmentNumber * request.SegmentSize
					return derr.NewFatalError(fmt.Errorf("segment [%d-%d] failed %d times, giving up. Last error from worker: %s", segmentStart, segmentStart+request.SegmentSize, retryIdx, err))
				}

				return err
			default:
				if err != nil {
					return derr.NewFatalError(err)
				}
				return nil
			}
		})

		timeTook := time.Since(startTime)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				err = context.Canceled // remove wrapping on this error
				stats.RecordEndSubrequest(jobIdx, metrics.JobCancelled)
			} else {
				stats.RecordEndSubrequest(jobIdx, metrics.JobFailed)
			}

			logger.Warn(
				"incomplete job",
				zap.Object("unit", unit),
				zap.Int("number_of_tries", retryIdx),
				zap.Strings("module_name", moduleNames),
				zap.Duration("duration", timeTook),
				zap.Error(err),
			)
			return MsgJobFailed{Unit: unit, Worker: w, Error: err}
		}

		if err := ctx.Err(); err != nil {
			logger.Warn(
				"incomplete job",
				zap.Object("unit", unit),
				zap.Int("number_of_tries", retryIdx),
				zap.Strings("module_name", moduleNames),
				zap.Duration("duration", timeTook),
				zap.Error(err),
			)
			stats.RecordEndSubrequest(jobIdx, metrics.JobCancelled)
			return MsgJobFailed{Unit: unit, Worker: w, Error: err}
		}

		stats.RecordEndSubrequest(jobIdx, metrics.JobComplete)
		logger.Info(
			"job completed",
			zap.Object("unit", unit),
			zap.Int("number_of_tries", retryIdx),
			zap.Strings("module_name", moduleNames),
			zap.Duration("duration", timeTook),
			zap.Float64("processing_time_per_block", timeTook.Seconds()/float64(request.SegmentSize)),
		)
		return MsgJobSucceeded{
			Unit:     unit,
			Worker:   w,
			Streamed: streamOutput,
		}
	}
}

var ErrConnectionRefused = errors.New("connection refused")

func (w *RemoteWorker) work(ctx context.Context, request *pbssinternal.ProcessRangeRequest, _ []string, upstream *response.Stream, jobIdx uint64) (res *Result) {
	metrics.Tier1ActiveWorkerRequest.Inc()
	metrics.Tier1WorkerRequestCounter.Inc()
	defer metrics.Tier1ActiveWorkerRequest.Dec()

	var err error

	ctx, span := reqctx.WithSpan(ctx, fmt.Sprintf("substreams/tier1/schedule/%s/%d", request.OutputModule, request.SegmentNumber))
	defer span.EndWithErr(&err)
	span.SetAttributes(
		attribute.String("substreams.output_module", request.OutputModule),
		attribute.Int64("substreams.segment_number", int64(request.SegmentNumber)),
		attribute.String("substreams.worker_id", w.id),
	)
	logger := w.logger

	grpcClient, closeFunc, grpcCallOpts, headers, err := w.clientFactory()
	if err != nil {
		return &Result{Error: fmt.Errorf("unable to create grpc client: %w", err)}
	}

	ctx = dauth.FromContext(ctx).ToOutgoingGRPCContext(ctx)

	if headers.IsSet() {
		ctx = metadata.AppendToOutgoingContext(ctx, headers.ToArray()...)
	}
	stream, err := grpcClient.ProcessRange(ctx, request, grpcCallOpts...)
	if err != nil {
		if ctx.Err() != nil {
			return &Result{Error: ctx.Err()}
		}
		return &Result{
			Error: NewRetryableErr(ErrConnectionRefused),
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
			return &Result{Error: ctx.Err()}
		}
		logger.Warn("error getting stream header", zap.Error(err))
	}
	remoteHostname := "unknown"
	if hosts := meta.Get("host"); len(hosts) != 0 {
		remoteHostname = hosts[0]
		logger = logger.With(zap.String("remote_hostname", remoteHostname))
	}

	span.SetAttributes(attribute.String("substreams.remote_hostname", remoteHostname))

	var tier2SupportsStreamingMode bool
	stats := reqctx.ReqStats(ctx)
	for {
		resp, err := stream.Recv()

		if err := ctx.Err(); err != nil {
			if err == context.Canceled {
				return &Result{}
			}
			return &Result{Error: err}
		}

		if resp != nil {
			switch r := resp.Type.(type) {
			case *pbssinternal.ProcessRangeResponse_Update:
				stats.RecordJobUpdate(jobIdx, r.Update)

			case *pbssinternal.ProcessRangeResponse_Failed:
				// FIXME(abourget): we do NOT emit those Failed objects anymore. There was a flow
				// for that that would pick up the errors, and pack the remaining logs
				// and reasons into a message. This is nowhere to be found now.

				upstream.RPCFailedProgressResponse(r.Failed.Reason, r.Failed.Logs, r.Failed.LogsTruncated)

				err := fmt.Errorf("work failed on remote host: %s", r.Failed.Reason)
				span.SetStatus(otelCodes.Error, err.Error())
				return &Result{Error: err}

			case *pbssinternal.ProcessRangeResponse_Completed:
				logger.Debug("worker done")
				tier2SupportsStreamingMode = r.Completed.StreamingMode
				stats.RecordBlocksProcessed(r.Completed.ProcessedBlocks) // add workers' processed blocks count to our own stats

			case *pbssinternal.ProcessRangeResponse_BlockScopedData:
				clock := r.BlockScopedData.Clock
				details := reqctx.Details(ctx)
				if clock.Number >= details.ResolvedStartBlockNum && (details.StopBlockNum == 0 || clock.Number < details.StopBlockNum) {

					blockRef := bstream.NewBlockRef(clock.Id, clock.Number)
					cursor := bstream.Cursor{
						Step:      bstream.StepNewIrreversible,
						Block:     blockRef,
						LIB:       blockRef,
						HeadBlock: blockRef,
					}

					upstream.BlockScopedData(&pbsubstreamsrpc.BlockScopedData{
						Output: &pbsubstreamsrpc.MapModuleOutput{
							Name:      details.OutputModule,
							MapOutput: r.BlockScopedData.Output,
						},
						Clock:            clock,
						FinalBlockHeight: r.BlockScopedData.Clock.Number,
						Cursor:           cursor.ToOpaque(),
					})
				}
			}
		}

		if err != nil {
			if err == io.EOF {
				if request.StreamOutput && !tier2SupportsStreamingMode {
					return &Result{Error: fmt.Errorf("tier2 incompatible: it does not support streaming mode, operator should upgrade tier2 server")}
				}

				return &Result{}
			}
			if ctx.Err() != nil {
				return &Result{Error: ctx.Err()}
			}
			if grpcErr := dgrpc.AsGRPCError(err); grpcErr != nil {
				switch grpcErr.Code() {
				case codes.InvalidArgument:
					return &Result{Error: err}
				case codes.DeadlineExceeded, codes.ResourceExhausted, codes.Unavailable:
					return &Result{
						Error: NewRetryableErr(err),
					}
				}
			}

			return &Result{
				Error: NewRetryableErr(fmt.Errorf("receiving stream resp: %w", err)),
			}
		}
	}
}

package work

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync/atomic"
	"time"

	"google.golang.org/grpc/metadata"

	"github.com/streamingfast/dauth"
	"github.com/streamingfast/derr"
	"github.com/streamingfast/dgrpc"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	otelCodes "go.opentelemetry.io/otel/codes"
	ttrace "go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"

	"github.com/streamingfast/substreams/block"
	"github.com/streamingfast/substreams/client"
	"github.com/streamingfast/substreams/metrics"
	"github.com/streamingfast/substreams/orchestrator/loop"
	"github.com/streamingfast/substreams/orchestrator/response"
	"github.com/streamingfast/substreams/orchestrator/stage"
	pbssinternal "github.com/streamingfast/substreams/pb/sf/substreams/intern/v2"
	"github.com/streamingfast/substreams/reqctx"
	"github.com/streamingfast/substreams/storage/store"
)

var lastWorkerID uint64

type Result struct {
	PartialFilesWritten store.FileInfos
	Error               error
}

type Worker interface {
	ID() string
	Work(ctx context.Context, unit stage.Unit, workRange *block.Range, moduleNames []string, upstream *response.Stream) loop.Cmd // *Result
}

func NewWorkerFactoryFromFunc(f func(ctx context.Context, unit stage.Unit, workRange *block.Range, moduleNames []string, upstream *response.Stream) loop.Cmd) *SimpleWorkerFactory {
	return &SimpleWorkerFactory{
		f:  f,
		id: atomic.AddUint64(&lastWorkerID, 1),
	}
}

type SimpleWorkerFactory struct {
	f  func(ctx context.Context, unit stage.Unit, workRange *block.Range, moduleNames []string, upstream *response.Stream) loop.Cmd
	id uint64
}

func (f SimpleWorkerFactory) Work(ctx context.Context, unit stage.Unit, workRange *block.Range, moduleNames []string, upstream *response.Stream) loop.Cmd {
	return f.f(ctx, unit, workRange, moduleNames, upstream)
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

func NewRequest(ctx context.Context, req *reqctx.RequestDetails, stageIndex int, workRange *block.Range) *pbssinternal.ProcessRangeRequest {
	tier2ReqParams, ok := reqctx.GetTier2RequestParameters(ctx)
	if !ok {
		panic("unable to get tier2 request parameters")
	}

	return &pbssinternal.ProcessRangeRequest{
		StartBlockNum: workRange.StartBlock,
		StopBlockNum:  workRange.ExclusiveEndBlock,
		Modules:       req.Modules,
		OutputModule:  req.OutputModule,
		Stage:         uint32(stageIndex),

		MeteringConfig:       tier2ReqParams.MeteringConfig,
		FirstStreamableBlock: tier2ReqParams.FirstStreamableBlock,
		MergedBlocksStore:    tier2ReqParams.MergedBlockStoreURL,
		StateStore:           tier2ReqParams.StateStoreURL,
		StateBundleSize:      tier2ReqParams.StateBundleSize,
		StateStoreDefaultTag: tier2ReqParams.StateStoreDefaultTag,
		WasmModules:          tier2ReqParams.WASMModules,
		BlockType:            tier2ReqParams.BlockType,
	}
}

func (w *RemoteWorker) Work(ctx context.Context, unit stage.Unit, workRange *block.Range, moduleNames []string, upstream *response.Stream) loop.Cmd {
	request := NewRequest(ctx, reqctx.Details(ctx), unit.Stage, workRange)
	logger := reqctx.Logger(ctx)

	return func() loop.Msg {
		var res *Result
		retryIdx := 0
		startTime := time.Now()
		maxRetries := 720 //TODO: make this configurable
		var previousError error
		err := derr.RetryContext(ctx, uint64(maxRetries), func(ctx context.Context) error {
			w.logger.Info("launching remote worker",
				zap.Int64("start_block_num", int64(request.StartBlockNum)),
				zap.Uint64("stop_block_num", request.StopBlockNum),
				zap.Uint32("stage", request.Stage),
				zap.String("output_module", request.OutputModule),
				zap.Int("attempt", retryIdx+1),
				zap.NamedError("previous_error", previousError),
			)

			res = w.work(ctx, request, moduleNames, upstream)
			err := res.Error
			switch err.(type) {
			case *RetryableErr:
				metrics.Tier1WorkerRetryCounter.Inc()
				if err != nil && strings.Contains(err.Error(), "service currently overloaded") {
					metrics.Tier1WorkerRejectedOverloadedCounter.Inc()
				}
				previousError = err
				retryIdx++
				return err
			default:
				if err != nil {
					return derr.NewFatalError(err)
				}
				return nil
			}
		})

		if err != nil {
			if errors.Is(err, context.Canceled) {
				logger.Debug("job canceled", zap.Object("unit", unit), zap.Error(err))
			} else {
				logger.Warn("job failed", zap.Object("unit", unit), zap.Error(err))
			}

			timeTook := time.Since(startTime)
			logger.Warn(
				"incomplete job",
				zap.Object("unit", unit),
				zap.Int("number_of_tries", retryIdx),
				zap.Strings("module_name", moduleNames),
				zap.Duration("duration", timeTook),
				zap.Float64("num_of_blocks_per_sec", float64(request.StopBlockNum-request.StartBlockNum)/timeTook.Seconds()),
				zap.Error(err),
			)
			return MsgJobFailed{Unit: unit, Error: err}
		}

		if err := ctx.Err(); err != nil {
			logger.Warn("job not completed", zap.Object("unit", unit), zap.Error(err))
			return MsgJobFailed{Unit: unit, Error: err}
		}

		timeTook := time.Since(startTime)
		logger.Info(
			"job completed",
			zap.Object("unit", unit),
			zap.Int("number_of_tries", retryIdx),
			zap.Strings("module_name", moduleNames),
			zap.Float64("duration", timeTook.Seconds()),
			zap.Float64("processing_time_per_block", timeTook.Seconds()/float64(request.StopBlockNum-request.StartBlockNum)),
		)
		return MsgJobSucceeded{
			Unit:   unit,
			Worker: w,
			// TODO: Clean the PartialFilesWritten from the res because it's not needed anymore.
			//Files:  res.PartialFilesWritten,
		}
	}
}

func (w *RemoteWorker) work(ctx context.Context, request *pbssinternal.ProcessRangeRequest, moduleNames []string, upstream *response.Stream) *Result {
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

	grpcClient, closeFunc, grpcCallOpts, headers, err := w.clientFactory()
	if err != nil {
		return &Result{Error: fmt.Errorf("unable to create grpc client: %w", err)}
	}

	stats := reqctx.ReqStats(ctx)
	jobIdx := stats.RecordNewSubrequest(request.Stage, request.StartBlockNum, request.StopBlockNum)
	defer stats.RecordEndSubrequest(jobIdx)

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
				return &Result{Error: ctx.Err()}
			}
			if grpcErr := dgrpc.AsGRPCError(err); grpcErr.Code() == codes.InvalidArgument {
				return &Result{Error: err}
			}
			return &Result{
				Error: NewRetryableErr(fmt.Errorf("receiving stream resp: %w", err)),
			}
		}
	}
}

func toRPCPartialFiles(completed *pbssinternal.Completed) (out store.FileInfos) {
	// TODO(abourget): Add the MODULE Name in there, so we know to which modules each of those things
	// are attached in the tier1.
	// TODO(abourget): actually, here generate all the partial file infos
	// based on the request, segment and stage, and disregard what was
	// sent over the Complete message.. we will simply wait for the
	// stores to all having been processed.
	out = make(store.FileInfos, len(completed.AllProcessedRanges))
	for i, b := range completed.AllProcessedRanges {
		out[i] = store.NewPartialFileInfo("TODO:CHANGE-ME", b.StartBlock, b.EndBlock)
	}
	return
}

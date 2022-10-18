package service

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/streamingfast/substreams/metrics"
	"github.com/streamingfast/substreams/service/config"

	"github.com/streamingfast/bstream/hub"
	"github.com/streamingfast/bstream/stream"
	dgrpcserver "github.com/streamingfast/dgrpc/server"
	"github.com/streamingfast/dstore"
	"github.com/streamingfast/logging"
	"github.com/streamingfast/substreams/client"
	"github.com/streamingfast/substreams/orchestrator"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/pipeline"
	"github.com/streamingfast/substreams/pipeline/execout"
	"github.com/streamingfast/substreams/pipeline/execout/cachev1"
	"github.com/streamingfast/substreams/tracing"
	"github.com/streamingfast/substreams/wasm"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	ttrace "go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	grpccode "google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type Service struct {
	blockType                 string
	partialModeEnabled        bool
	wasmExtensions            []wasm.WASMExtensioner
	pipelineOptions           []pipeline.PipelineOptioner
	streamFactory             *StreamFactory
	workerPool                *orchestrator.WorkerPool
	parallelSubRequests       int
	blockRangeSizeSubRequests int

	// properties of cache
	storesSaveInterval           uint64
	outputCacheSaveBlockInterval uint64
	baseStateStore               dstore.Store

	tracer ttrace.Tracer
	logger *zap.Logger
}

func New(
	stateStore dstore.Store,
	blockType string,
	parallelSubRequests int,
	blockRangeSizeSubRequests int,
	substreamsClientConfig *client.SubstreamsClientConfig,
	opts ...Option,
) (s *Service, err error) {
	s = &Service{
		baseStateStore:            stateStore,
		blockType:                 blockType,
		parallelSubRequests:       parallelSubRequests,
		blockRangeSizeSubRequests: blockRangeSizeSubRequests,
		tracer:                    otel.GetTracerProvider().Tracer("service"),
	}

	zlog.Info("registering substreams metrics")
	metrics.Metricset.Register()

	zlog.Info("creating gprc client factory", zap.Reflect("config", substreamsClientConfig))
	newSubstreamClientFunc := client.NewFactory(substreamsClientConfig)

	s.workerPool = orchestrator.NewWorkerPool(parallelSubRequests, func() orchestrator.Worker {
		return orchestrator.NewRemoteWorker(newSubstreamClientFunc)
	})

	for _, opt := range opts {
		opt(s)
	}

	return s, nil
}

func (s *Service) BaseStateStore() dstore.Store {
	return s.baseStateStore
}

func (s *Service) BlockType() string {
	return s.blockType
}

func (s *Service) WasmExtensions() []wasm.WASMExtensioner {
	return s.wasmExtensions
}

func (s *Service) Register(
	server dgrpcserver.Server,
	mergedBlocksStore dstore.Store,
	forkedBlocksStore dstore.Store,
	forkableHub *hub.ForkableHub,
	logger *zap.Logger) {

	sf := &StreamFactory{
		mergedBlocksStore: mergedBlocksStore,
		forkedBlocksStore: forkedBlocksStore,
		hub:               forkableHub,
	}

	s.streamFactory = sf
	s.logger = logger
	server.RegisterService(func(gs grpc.ServiceRegistrar) {
		pbsubstreams.RegisterStreamServer(gs, s)
	})
}

func (s *Service) Blocks(request *pbsubstreams.Request, streamSrv pbsubstreams.Stream_BlocksServer) (grpcError error) {
	// We keep `err` here as the unaltered error from `blocks` call, this is used in the EndSpan to record the full error
	// and not only the `grpcError` one which is a subset view of the full `err`.
	var err error

	ctx, span := s.tracer.Start(streamSrv.Context(), "substreams_request")
	defer tracing.EndSpan(span, tracing.WithEndErr(&err))

	// Weird behavior because we want the pipeline to set the logger in the request Context
	logger := logging.Logger(streamSrv.Context(), s.logger)

	hostname := updateStreamHeadersHostname(streamSrv, logger)
	span.SetAttributes(attribute.String("hostname", hostname))

	// We execute the blocks stream handler and then transform `err` to a gRPC error, keeping both of them.
	// They will be both `nil` if `err` is `nil` itself.
	err = s.blocks(ctx, request, streamSrv, logger)
	grpcError = s.toGRPCError(err)

	if grpcError != nil && status.Code(grpcError) == codes.Internal {
		logger.Info("unexpected termination of stream of blocks", zap.Error(err))
	}

	return grpcError
}

func (s *Service) blocks(ctx context.Context, request *pbsubstreams.Request, streamSrv pbsubstreams.Stream_BlocksServer, logger *zap.Logger) error {
	logger.Info("validating request")
	graph, err := validateGraph(request, s.blockType)
	if err != nil {
		return status.Error(grpccode.InvalidArgument, err.Error())
	}

	// TODO: missing dmetering hook that was present for each output
	// payload, we'd send the increment in EgressBytes sent.  We'll
	// want to review that anyway.
	var opts []pipeline.Option
	for _, pipeOpts := range s.pipelineOptions {
		for _, opt := range pipeOpts.PipelineOptions(ctx, request) {
			opts = append(opts, opt)
		}
	}

	/*
		this entire `if` is not good, the ctx is from the StreamServer so there
		is no substreams-partial-mode, the actual flag is substreams-partial-mode-enabled
	*/

	isSubrequest := false
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		partialMode := md.Get("substreams-partial-mode")
		logger.Debug("extracting meta data", zap.Strings("partial_mode", partialMode))
		if len(partialMode) == 1 && partialMode[0] == "true" {
			// TODO: only allow partial-mode if the AUTHORIZATION layer permits it
			// partial-mode should be
			if !s.partialModeEnabled {
				return status.Error(grpccode.InvalidArgument, "substreams-partial-mode not enabled on this instance")
			}
			isSubrequest = true
		}
	}

	responseHandler := func(resp *pbsubstreams.Response) error {
		if err := streamSrv.Send(resp); err != nil {
			logger.Info("unable to send block probably due to client disconnecting", zap.Error(err))
			return status.Error(codes.Unavailable, err.Error())
		}
		return nil
	}

	requestCtx, err := pipeline.NewRequestContext(ctx, request, isSubrequest)
	if err != nil {
		return err
	}

	storeBoundary := pipeline.NewStoreBoundary(s.storesSaveInterval)
	cachingEngine := execout.NewNoOpCache()
	if s.baseStateStore != nil {
		cachingEngine, err = cachev1.NewEngine(context.Background(), s.outputCacheSaveBlockInterval, s.baseStateStore, requestCtx.Logger())
		if err != nil {
			return fmt.Errorf("error building caching engine: %w", err)
		}
	}

	pipe := pipeline.New(
		requestCtx,
		graph,
		s.blockType,
		s.wasmExtensions,
		cachingEngine,
		&config.RuntimeConfig{
			StoreSnapshotsObjectStore:  s.baseStateStore,
			StoreSnapshotsSaveInterval: s.storesSaveInterval,
			SubrequestsSplitSize:       s.blockRangeSizeSubRequests,
			ParallelSubrequests:        s.parallelSubRequests,
		},
		storeBoundary,
		responseHandler,
		opts...,
	)

	if err := pipe.Init(s.workerPool); err != nil {
		return fmt.Errorf("error building pipeline: %w", err)
	}

	// It's ok to use `StartBlockNum` directly here (instead of `requestCtx.EffectiveStartBlockNum`)
	// and in the constructor we also pass `StartCursor` which will be handled by `streamFactory.New`
	// and will be used to bootstrap the stream correctly from it if set.
	zlog.Info("creating firehose stream",
		zap.Int64("start_block", request.StartBlockNum),
		zap.Uint64("end_block", request.StopBlockNum),
		zap.String("start_block", request.StartCursor),
	)
	blockStream, err := s.streamFactory.New(
		pipe,
		request.StartBlockNum,
		request.StopBlockNum,
		request.StartCursor,
	)
	if err != nil {
		return fmt.Errorf("error getting stream: %w", err)
	}

	return pipe.OnStreamTerminated(streamSrv, blockStream.Run(ctx))
}

func updateStreamHeadersHostname(streamSrv pbsubstreams.Stream_BlocksServer, logger *zap.Logger) string {
	hostname, err := os.Hostname()
	if err != nil {
		logger.Warn("cannot find hostname, using 'unknown'", zap.Error(err))
		hostname = "unknown host"
	}
	if os.Getenv("SUBSTREAMS_SEND_HOSTNAME") == "true" {
		md := metadata.New(map[string]string{"host": hostname})
		err = streamSrv.SetHeader(md)
		if err != nil {
			logger.Warn("cannot send header metadata", zap.Error(err))
		}
	}
	return hostname
}

// toGRPCError turns an `err` into a gRPC error if it's non-nil, in the `nil` case,
// `nil` is returned right away.
//
// If the `err` has in its chain of error either `context.Canceled`, `context.DeadlineExceeded`
// or `stream.ErrInvalidArg`, error is turned into a proper gRPC error respectively of code
// `Canceled`, `DeadlineExceeded` or `InvalidArgument`.
//
// If the `err` has its in chain any error constructed through `status.Error` (and its variants), then
// we return the first found error of such type directly, because it's already a gRPC error.
//
// Otherwise, the error is assumed to be an internal error and turned backed into a proper
// `status.Error(codes.Internal, err.Error())`.
func (s *Service) toGRPCError(err error) error {
	if err == nil {
		return nil
	}

	if errors.Is(err, context.Canceled) {
		return status.Error(codes.Canceled, "source canceled")
	}

	if errors.Is(err, context.DeadlineExceeded) {
		return status.Error(codes.DeadlineExceeded, "source deadline exceeded")
	}

	var errInvalidArg *stream.ErrInvalidArg
	if errors.As(err, &errInvalidArg) {
		return status.Error(codes.InvalidArgument, errInvalidArg.Error())
	}

	// Do we want to print the full cause as coming from Golang? Would we like to maybe trim off "operational"
	// data?
	return status.Error(codes.Internal, err.Error())
}

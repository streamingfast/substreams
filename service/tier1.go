package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/streamingfast/bstream/hub"
	"github.com/streamingfast/bstream/stream"
	"github.com/streamingfast/dauth/authenticator"
	"github.com/streamingfast/dgrpc"
	dgrpcserver "github.com/streamingfast/dgrpc/server"
	"github.com/streamingfast/dstore"
	"github.com/streamingfast/logging"
	tracing "github.com/streamingfast/sf-tracing"
	"go.opentelemetry.io/otel/attribute"
	ttrace "go.opentelemetry.io/otel/trace"
	"go.uber.org/atomic"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/streamingfast/substreams"
	"github.com/streamingfast/substreams/client"
	"github.com/streamingfast/substreams/metrics"
	"github.com/streamingfast/substreams/orchestrator/work"
	pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
	"github.com/streamingfast/substreams/pipeline"
	"github.com/streamingfast/substreams/pipeline/cache"
	"github.com/streamingfast/substreams/pipeline/exec"
	"github.com/streamingfast/substreams/pipeline/outputmodules"
	"github.com/streamingfast/substreams/reqctx"
	"github.com/streamingfast/substreams/service/config"
	"github.com/streamingfast/substreams/storage/execout"
	"github.com/streamingfast/substreams/storage/store"
	"github.com/streamingfast/substreams/tracking"
	"github.com/streamingfast/substreams/wasm"
)

type Tier1Service struct {
	blockType         string
	wasmExtensions    []wasm.WASMExtensioner
	pipelineOptions   []pipeline.PipelineOptioner
	streamFactoryFunc StreamFactoryFunc
	runtimeConfig     config.RuntimeConfig
	tracer            ttrace.Tracer
	logger            *zap.Logger

	getRecentFinalBlock func() (uint64, error)
	resolveCursor       pipeline.CursorResolver
	getHeadBlock        func() (uint64, error)
}

var workerID atomic.Uint64

func NewTier1(
	stateStore dstore.Store,
	blockType string,
	parallelSubRequests uint64,
	subrequestSplitSize uint64,
	substreamsClientConfig *client.SubstreamsClientConfig,
	opts ...Option,
) (s *Tier1Service, err error) {

	zlog.Info("creating gprc client factory", zap.Reflect("config", substreamsClientConfig))
	clientFactory := client.NewInternalClientFactory(substreamsClientConfig)

	runtimeConfig := config.NewRuntimeConfig(
		1000, // overridden by Options
		subrequestSplitSize,
		parallelSubRequests,
		10,
		0,
		stateStore,
		func(logger *zap.Logger) work.Worker {
			return work.NewRemoteWorker(clientFactory, logger)
		},
	)
	s = &Tier1Service{
		runtimeConfig: runtimeConfig,
		blockType:     blockType,
		tracer:        tracing.GetTracer(),
	}

	zlog.Info("registering substreams metrics")
	metrics.MetricSet.Register()

	for _, opt := range opts {
		opt(s)
	}

	return s, nil
}

func (s *Tier1Service) BaseStateStore() dstore.Store {
	return s.runtimeConfig.BaseObjectStore
}

func (s *Tier1Service) BlockType() string {
	return s.blockType
}

func (s *Tier1Service) Register(
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

	s.streamFactoryFunc = sf.New
	s.getRecentFinalBlock = sf.GetRecentFinalBlock
	s.resolveCursor = pipeline.NewCursorResolver(forkableHub, mergedBlocksStore, forkedBlocksStore)
	s.getHeadBlock = sf.GetHeadBlock
	s.logger = logger
	server.RegisterService(func(gs grpc.ServiceRegistrar) {
		pbsubstreamsrpc.RegisterStreamServer(gs, s)
	})
}

func (s *Tier1Service) Blocks(request *pbsubstreamsrpc.Request, streamSrv pbsubstreamsrpc.Stream_BlocksServer) (grpcError error) {
	// We keep `err` here as the unaltered error from `blocks` call, this is used in the EndSpan to record the full error
	// and not only the `grpcError` one which is a subset view of the full `err`.
	var err error
	ctx := streamSrv.Context()

	logger := reqctx.Logger(ctx).Named("tier1")
	respFunc := responseHandler(logger, streamSrv)
	traceId := tracing.GetTraceID(ctx).String()
	respFunc(&pbsubstreamsrpc.Response{
		Message: &pbsubstreamsrpc.Response_Session{
			Session: &pbsubstreamsrpc.SessionInit{
				TraceId: traceId,
			},
		},
	})

	ctx = logging.WithLogger(ctx, logger)
	ctx = reqctx.WithTracer(ctx, s.tracer)

	ctx, span := reqctx.WithSpan(ctx, "substreams/tier1/request")
	defer span.EndWithErr(&err)

	span.SetAttributes(attribute.Int64("substreams.tier", 1))

	hostname := updateStreamHeadersHostname(streamSrv.SetHeader, logger)
	span.SetAttributes(attribute.String("hostname", hostname))

	if bytesMeter := tracking.GetBytesMeter(ctx); bytesMeter != nil {
		// WARN: we shouldn't mutate this object. It's globally shared.
		// If we need to pass down a custom runtimeConfig (RuntimeConfig
		// is by definition a global), we should refactor things around
		// and perhaps clone it.
		//
		//s.runtimeConfig.BaseObjectStore.SetMeter(bytesMeter)
	}

	if request.Modules == nil {
		return status.Error(codes.InvalidArgument, "missing modules in request")
	}
	moduleNames := make([]string, len(request.Modules.Modules))
	for i := 0; i < len(moduleNames); i++ {
		moduleNames[i] = request.Modules.Modules[i].Name
	}
	fields := []zap.Field{
		zap.Int64("start_block", request.StartBlockNum),
		zap.Uint64("stop_block", request.StopBlockNum),
		zap.String("cursor", request.StartCursor),
		zap.Strings("modules", moduleNames),
		zap.String("output_module", request.OutputModule),
	}
	fields = append(fields, zap.Bool("production_mode", request.ProductionMode))
	auth := authenticator.GetCredentials(ctx)
	if id := auth.GetUserID(); id != "" {
		fields = append(fields, zap.String("user_id", id))
	}
	logger.Info("incoming Substreams Blocks request", fields...)

	err = s.blocks(ctx, request, respFunc)
	grpcError = toGRPCError(err)

	if grpcError != nil && status.Code(grpcError) == codes.Internal {
		logger.Info("unexpected termination of stream of blocks", zap.String("stream_processor", "tier1"), zap.Error(err))
	}

	return grpcError
}

func (s *Tier1Service) blocks(ctx context.Context, request *pbsubstreamsrpc.Request, respFunc substreams.ResponseFunc) error {
	logger := reqctx.Logger(ctx)

	if err := outputmodules.ValidateTier1Request(request, s.blockType); err != nil {
		return stream.NewErrInvalidArg(fmt.Errorf("validate request: %w", err).Error())
	}

	outputGraph, err := outputmodules.NewOutputModuleGraph(request.OutputModule, request.ProductionMode, request.Modules)
	if err != nil {
		return stream.NewErrInvalidArg(err.Error())
	}

	ctx, requestStats := setupRequestStats(ctx, logger, s.runtimeConfig.WithRequestStats, false)

	//bytesMeter := tracking.NewBytesMeter(ctx)
	//bytesMeter.Launch(ctx, respFunc)
	//ctx = tracking.WithBytesMeter(ctx, bytesMeter)

	requestDetails, undoSignal, err := pipeline.BuildRequestDetails(ctx, request, s.getRecentFinalBlock, s.resolveCursor, s.getHeadBlock)
	if err != nil {
		return fmt.Errorf("build request details: %w", err)
	}

	ctx = reqctx.WithRequest(ctx, requestDetails)
	if s.runtimeConfig.ModuleExecutionTracing {
		ctx = reqctx.WithModuleExecutionTracing(ctx)
	}

	if err := outputGraph.ValidateRequestStartBlock(requestDetails.ResolvedStartBlockNum); err != nil {
		return stream.NewErrInvalidArg(err.Error())
	}

	wasmRuntime := wasm.NewRuntime(s.wasmExtensions, s.runtimeConfig.MaxWasmFuel)

	execOutputConfigs, err := execout.NewConfigs(s.runtimeConfig.BaseObjectStore, outputGraph.UsedModules(), outputGraph.ModuleHashes(), s.runtimeConfig.CacheSaveInterval, logger)
	if err != nil {
		return fmt.Errorf("new config map: %w", err)
	}

	storeConfigs, err := store.NewConfigMap(s.runtimeConfig.BaseObjectStore, outputGraph.Stores(), outputGraph.ModuleHashes(), tracing.GetTraceID(ctx).String())
	if err != nil {
		return fmt.Errorf("configuring stores: %w", err)
	}

	stores := pipeline.NewStores(storeConfigs, s.runtimeConfig.CacheSaveInterval, requestDetails.LinearHandoffBlockNum, request.StopBlockNum, false, "tier1")

	execOutputCacheEngine, err := cache.NewEngine(ctx, s.runtimeConfig, nil, s.blockType)
	if err != nil {
		return fmt.Errorf("error building caching engine: %w", err)
	}

	opts := s.buildPipelineOptions(ctx)
	if undoSignal != nil {
		opts = append(opts, pipeline.WithPendingUndoMessage(
			&pbsubstreamsrpc.Response{
				Message: &pbsubstreamsrpc.Response_BlockUndoSignal{
					BlockUndoSignal: undoSignal,
				},
			}))
	}
	if request.FinalBlocksOnly {
		opts = append(opts, pipeline.WithFinalBlocksOnly())
	}

	pipe := pipeline.New(
		ctx,
		outputGraph,
		stores,
		execOutputConfigs,
		wasmRuntime,
		execOutputCacheEngine,
		s.runtimeConfig,
		respFunc,
		"tier1",
		tracing.GetTraceID(ctx).String(),
		opts...,
	)

	if requestStats != nil {
		requestStats.Start(10 * time.Second)
		defer requestStats.Shutdown()
	}
	logger.Info("initializing pipeline",
		zap.Int64("request_start_block", request.StartBlockNum),
		zap.Uint64("resolved_start_block", requestDetails.ResolvedStartBlockNum),
		zap.Uint64("request_stop_block", request.StopBlockNum),
		zap.String("request_start_cursor", request.StartCursor),
		zap.String("resolved_cursor", requestDetails.ResolvedCursor),
		zap.String("output_module", request.OutputModule),
	)

	if err := pipe.InitStoresAndBackprocess(ctx); err != nil {
		return fmt.Errorf("error during init_stores_and_backprocess: %w", err)
	}
	if requestDetails.LinearHandoffBlockNum == request.StopBlockNum {
		return pipe.OnStreamTerminated(ctx, nil)
	}

	if err := pipe.InitWASM(ctx); err != nil {
		return fmt.Errorf("error building pipeline WASM: %w", err)
	}

	var streamErr error
	cursor := requestDetails.ResolvedCursor
	var cursorIsTarget bool
	if requestDetails.ResolvedStartBlockNum != requestDetails.LinearHandoffBlockNum {
		cursorIsTarget = true
	}
	logger.Info("creating firehose stream",
		zap.Uint64("handoff_block", requestDetails.LinearHandoffBlockNum),
		zap.Uint64("stop_block", request.StopBlockNum),
		zap.String("cursor", cursor),
	)

	blockStream, err := s.streamFactoryFunc(
		ctx,
		pipe,
		int64(requestDetails.LinearHandoffBlockNum),
		request.StopBlockNum,
		cursor,
		request.FinalBlocksOnly,
		cursorIsTarget,
		logger.Named("stream"),
	)
	if err != nil {
		return fmt.Errorf("error getting stream: %w", err)
	}

	ctx, span := reqctx.WithSpan(ctx, "substreams/tier1/pipeline/blocks_stream")
	streamErr = blockStream.Run(ctx)
	span.EndWithErr(&streamErr)

	return pipe.OnStreamTerminated(ctx, streamErr)
}

func (s *Tier1Service) buildPipelineOptions(ctx context.Context) (opts []pipeline.Option) {
	reqDetails := reqctx.Details(ctx)
	for _, pipeOpts := range s.pipelineOptions {
		opts = append(opts, pipeOpts.PipelineOptions(ctx, reqDetails.ResolvedStartBlockNum, reqDetails.StopBlockNum, reqDetails.UniqueIDString())...)
	}
	return
}

func responseHandler(logger *zap.Logger, streamSrv pbsubstreamsrpc.Stream_BlocksServer) func(substreams.ResponseFromAnyTier) error {
	return func(anyResp substreams.ResponseFromAnyTier) error {
		resp := anyResp.(*pbsubstreamsrpc.Response)
		if err := streamSrv.Send(resp); err != nil {
			logger.Info("unable to send block probably due to client disconnecting", zap.Error(err))
			return status.Error(codes.Unavailable, err.Error())
		}
		return nil
	}
}

func setupRequestStats(ctx context.Context, logger *zap.Logger, withRequestStats, isSubRequest bool) (context.Context, metrics.Stats) {
	if isSubRequest {
		wid := workerID.Inc()
		logger = logger.With(zap.Uint64("worker_id", wid))
		return reqctx.WithLogger(ctx, logger), metrics.NewNoopStats()
	}

	// we only want to measure stats when enabled an on the Main request
	if withRequestStats {
		stats := metrics.NewReqStats(logger)
		return reqctx.WithReqStats(ctx, stats), stats
	}
	return ctx, metrics.NewNoopStats()
}

func updateStreamHeadersHostname(setHeader func(metadata.MD) error, logger *zap.Logger) string {
	hostname, err := os.Hostname()
	if err != nil {
		logger.Warn("cannot find hostname, using 'unknown'", zap.Error(err))
		hostname = "unknown host"
	}
	if os.Getenv("SUBSTREAMS_SEND_HOSTNAME") == "true" {
		md := metadata.New(map[string]string{"host": hostname})
		err = setHeader(md)
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
func toGRPCError(err error) error {
	if err == nil {
		return nil
	}

	if errors.Is(err, context.Canceled) {
		return status.Error(codes.Canceled, "source canceled")
	}

	if errors.Is(err, context.DeadlineExceeded) {
		return status.Error(codes.DeadlineExceeded, "source deadline exceeded")
	}

	if errors.Is(err, exec.ErrWasmDeterministicExec) {
		return status.Error(codes.InvalidArgument, err.Error())
	}

	dgrpc.AsGRPCError(err)

	var errInvalidArg *stream.ErrInvalidArg
	if errors.As(err, &errInvalidArg) {
		return status.Error(codes.InvalidArgument, errInvalidArg.Error())
	}

	// Do we want to print the full cause as coming from Golang? Would we like to maybe trim off "operational"
	// data?
	return status.Error(codes.Internal, err.Error())
}

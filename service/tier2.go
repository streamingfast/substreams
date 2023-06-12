package service

import (
	"context"
	"fmt"
	"time"

	"github.com/streamingfast/bstream/hub"
	"github.com/streamingfast/bstream/stream"
	dgrpcserver "github.com/streamingfast/dgrpc/server"
	"github.com/streamingfast/dstore"
	"github.com/streamingfast/logging"
	tracing "github.com/streamingfast/sf-tracing"
	"go.opentelemetry.io/otel/attribute"
	ttrace "go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/streamingfast/substreams"
	"github.com/streamingfast/substreams/metrics"
	pbssinternal "github.com/streamingfast/substreams/pb/sf/substreams/intern/v2"
	"github.com/streamingfast/substreams/pipeline"
	"github.com/streamingfast/substreams/pipeline/cache"
	"github.com/streamingfast/substreams/pipeline/outputmodules"
	"github.com/streamingfast/substreams/reqctx"
	"github.com/streamingfast/substreams/service/config"
	"github.com/streamingfast/substreams/storage/execout"
	"github.com/streamingfast/substreams/storage/store"
	"github.com/streamingfast/substreams/tracking"
	"github.com/streamingfast/substreams/wasm"
)

type Tier2Service struct {
	blockType         string
	wasmExtensions    []wasm.WASMExtensioner
	pipelineOptions   []pipeline.PipelineOptioner
	streamFactoryFunc StreamFactoryFunc
	runtimeConfig     config.RuntimeConfig
	tracer            ttrace.Tracer
	logger            *zap.Logger
}

func NewTier2(
	stateStore dstore.Store,
	blockType string,
	opts ...Option,
) (s *Tier2Service) {

	runtimeConfig := config.NewRuntimeConfig(
		1000, // overridden by Options
		0,
		0, // tier2 don't send subrequests
		0, // tier2 don't send subrequests
		0, // tier2 don't send subrequests
		stateStore,
		nil,
	)
	s = &Tier2Service{
		runtimeConfig: runtimeConfig,
		blockType:     blockType,
		tracer:        tracing.GetTracer(),
	}

	zlog.Info("registering substreams metrics")
	metrics.MetricSet.Register()

	for _, opt := range opts {
		opt(s)
	}

	return s
}

func (s *Tier2Service) BaseStateStore() dstore.Store {
	return s.runtimeConfig.BaseObjectStore
}

func (s *Tier2Service) BlockType() string {
	return s.blockType
}

func (s *Tier2Service) Register(
	server dgrpcserver.Server,
	mergedBlocksStore dstore.Store,
	_ dstore.Store,
	_ *hub.ForkableHub,
	logger *zap.Logger) {

	sf := &StreamFactory{
		mergedBlocksStore: mergedBlocksStore,
	}

	s.streamFactoryFunc = sf.New
	s.logger = logger
	server.RegisterService(func(gs grpc.ServiceRegistrar) {
		pbssinternal.RegisterSubstreamsServer(gs, s)
	})
}

func (s *Tier2Service) ProcessRange(request *pbssinternal.ProcessRangeRequest, streamSrv pbssinternal.Substreams_ProcessRangeServer) (grpcError error) {
	// We keep `err` here as the unaltered error from `blocks` call, this is used in the EndSpan to record the full error
	// and not only the `grpcError` one which is a subset view of the full `err`.
	var err error
	ctx := streamSrv.Context()

	// TODO: use stage and segment numbers when implemented
	stage := request.OutputModule
	segment := fmt.Sprintf("%d:%d",
		request.StartBlockNum,
		request.StopBlockNum)

	logger := reqctx.Logger(ctx).Named("tier2").With(
		zap.String("stage", stage),
		zap.String("segment", segment),
	)

	ctx = logging.WithLogger(ctx, logger)
	ctx = reqctx.WithTracer(ctx, s.tracer)

	ctx, span := reqctx.WithSpan(ctx, "substreams/tier2/request")
	defer span.EndWithErr(&err)
	span.SetAttributes(attribute.Int64("substreams.tier", 2))

	hostname := updateStreamHeadersHostname(streamSrv.SetHeader, logger)
	span.SetAttributes(attribute.String("hostname", hostname))

	if bytesMeter := tracking.GetBytesMeter(ctx); bytesMeter != nil {
		s.runtimeConfig.BaseObjectStore.SetMeter(bytesMeter)
	}

	if request.Modules == nil {
		return status.Error(codes.InvalidArgument, "missing modules in request")
	}
	moduleNames := make([]string, len(request.Modules.Modules))
	for i := 0; i < len(moduleNames); i++ {
		moduleNames[i] = request.Modules.Modules[i].Name
	}

	fields := []zap.Field{
		zap.Uint64("start_block", request.StartBlockNum),
		zap.Uint64("stop_block", request.StopBlockNum),
		zap.Strings("modules", moduleNames),
		zap.String("output_module", request.OutputModule),
	}
	logger.Info("incoming substreams ProcessRange request", fields...)

	respFunc := tier2ResponseHandler(logger, streamSrv)
	err = s.processRange(ctx, request, respFunc, tracing.GetTraceID(ctx).String())
	grpcError = toGRPCError(err)

	if grpcError != nil && status.Code(grpcError) == codes.Internal {
		logger.Info("unexpected termination of stream of blocks", zap.Error(err))
	}

	return grpcError
}

func (s *Tier2Service) processRange(ctx context.Context, request *pbssinternal.ProcessRangeRequest, respFunc substreams.ResponseFunc, traceID string) error {
	logger := reqctx.Logger(ctx)

	if err := outputmodules.ValidateTier2Request(request, s.blockType); err != nil {
		return stream.NewErrInvalidArg(fmt.Errorf("validate request: %w", err).Error())
	}

	outputGraph, err := outputmodules.NewOutputModuleGraph(request.OutputModule, true, request.Modules)
	if err != nil {
		return stream.NewErrInvalidArg(err.Error())
	}

	ctx, requestStats := setupRequestStats(ctx, logger, s.runtimeConfig.WithRequestStats, true)

	// bytesMeter := tracking.NewBytesMeter(ctx)
	// bytesMeter.Launch(ctx, respFunc)
	// ctx = tracking.WithBytesMeter(ctx, bytesMeter)

	requestDetails := pipeline.BuildRequestDetailsFromSubrequest(request)
	ctx = reqctx.WithRequest(ctx, requestDetails)
	if s.runtimeConfig.ModuleExecutionTracing {
		ctx = reqctx.WithModuleExecutionTracing(ctx)
	}

	if err := outputGraph.ValidateRequestStartBlock(requestDetails.ResolvedStartBlockNum); err != nil {
		return stream.NewErrInvalidArg(err.Error())
	}

	wasmRuntime := wasm.NewRegistry(s.wasmExtensions, s.runtimeConfig.MaxWasmFuel)

	execOutputConfigs, err := execout.NewConfigs(s.runtimeConfig.BaseObjectStore, outputGraph.UsedModules(), outputGraph.ModuleHashes(), s.runtimeConfig.CacheSaveInterval, logger)
	if err != nil {
		return fmt.Errorf("new config map: %w", err)
	}

	storeConfigs, err := store.NewConfigMap(s.runtimeConfig.BaseObjectStore, outputGraph.Stores(), outputGraph.ModuleHashes(), traceID)
	if err != nil {
		return fmt.Errorf("configuring stores: %w", err)
	}
	stores := pipeline.NewStores(storeConfigs, s.runtimeConfig.CacheSaveInterval, requestDetails.ResolvedStartBlockNum, request.StopBlockNum, true, "tier2")

	// TODO(abourget): why would this start at the LinearHandoffBlockNum ?
	//  * in direct mode, this would mean we start writing files after the handoff,
	//    but it's not so useful to write those files, as they're partials
	//    and the OutputWriter doesn't know if that `initialBlockBoundary` is the  module's init Block?
	//  *
	outputModule := outputGraph.OutputModule()
	execOutWriter := execout.NewWriter(
		requestDetails.ResolvedStartBlockNum,
		requestDetails.StopBlockNum,
		outputModule.Name,
		execOutputConfigs,
		true,
	)

	execOutputCacheEngine, err := cache.NewEngine(ctx, s.runtimeConfig, execOutWriter, s.blockType)
	if err != nil {
		return fmt.Errorf("error building caching engine: %w", err)
	}

	opts := s.buildPipelineOptions(ctx, request)
	opts = append(opts, pipeline.WithFinalBlocksOnly())

	pipe := pipeline.New(
		ctx,
		outputGraph,
		stores,
		execOutputConfigs,
		wasmRuntime,
		execOutputCacheEngine,
		s.runtimeConfig,
		respFunc,
		"tier2",
		// This must always be the parent/global trace id, the one that comes from tier1
		traceID,
		opts...,
	)

	if requestStats != nil {
		requestStats.Start(10 * time.Second)
		defer requestStats.Shutdown()
	}
	logger.Info("initializing pipeline",
		zap.Uint64("request_start_block", requestDetails.ResolvedStartBlockNum),
		zap.Uint64("request_stop_block", request.StopBlockNum),
		zap.String("output_module", request.OutputModule),
	)
	if err := pipe.InitStoresAndBackprocess(ctx); err != nil {
		return fmt.Errorf("error building pipeline: %w", err)
	}

	if err := pipe.InitWASM(ctx); err != nil {
		return fmt.Errorf("error building pipeline WASM: %w", err)
	}

	var streamErr error
	blockStream, err := s.streamFactoryFunc(
		ctx,
		pipe,
		int64(requestDetails.ResolvedStartBlockNum),
		request.StopBlockNum,
		"",
		true,
		false,
		logger.Named("stream"),
	)
	if err != nil {
		return fmt.Errorf("error getting stream: %w", err)
	}

	ctx, span := reqctx.WithSpan(ctx, "substreams/tier2/pipeline/blocks_stream")
	streamErr = blockStream.Run(ctx)
	span.EndWithErr(&streamErr)

	return pipe.OnStreamTerminated(ctx, streamErr)
}

func (s *Tier2Service) buildPipelineOptions(ctx context.Context, request *pbssinternal.ProcessRangeRequest) (opts []pipeline.Option) {
	requestDetails := reqctx.Details(ctx)
	for _, pipeOpts := range s.pipelineOptions {
		opts = append(opts, pipeOpts.PipelineOptions(ctx, request.StartBlockNum, request.StopBlockNum, requestDetails.UniqueIDString())...)
	}
	return
}

func tier2ResponseHandler(logger *zap.Logger, streamSrv pbssinternal.Substreams_ProcessRangeServer) func(substreams.ResponseFromAnyTier) error {
	return func(respAny substreams.ResponseFromAnyTier) error {
		resp := respAny.(*pbssinternal.ProcessRangeResponse)
		if err := streamSrv.Send(resp); err != nil {
			logger.Info("unable to send block probably due to client disconnecting", zap.Error(err))
			return status.Error(codes.Unavailable, err.Error())
		}
		return nil
	}
}

package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	"connectrpc.com/connect"
	"github.com/streamingfast/bstream"
	"github.com/streamingfast/bstream/stream"
	"github.com/streamingfast/dauth"
	"github.com/streamingfast/dgrpc"
	"github.com/streamingfast/dmetering"
	"github.com/streamingfast/dstore"
	"github.com/streamingfast/logging"
	tracing "github.com/streamingfast/sf-tracing"

	pbbstream "github.com/streamingfast/bstream/pb/sf/bstream/v1"
	"github.com/streamingfast/substreams"
	"github.com/streamingfast/substreams/metrics"
	pbssinternal "github.com/streamingfast/substreams/pb/sf/substreams/intern/v2"
	"github.com/streamingfast/substreams/pipeline"
	"github.com/streamingfast/substreams/pipeline/cache"
	"github.com/streamingfast/substreams/pipeline/exec"
	"github.com/streamingfast/substreams/pipeline/outputmodules"
	"github.com/streamingfast/substreams/reqctx"
	"github.com/streamingfast/substreams/service/config"
	"github.com/streamingfast/substreams/storage/execout"
	"github.com/streamingfast/substreams/storage/store"
	"github.com/streamingfast/substreams/wasm"
	"go.opentelemetry.io/otel/attribute"
	ttrace "go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type Tier2Service struct {
	blockType         string
	wasmExtensions    []wasm.WASMExtensioner
	pipelineOptions   []pipeline.PipelineOptioner
	streamFactoryFunc StreamFactoryFunc
	runtimeConfig     config.RuntimeConfig
	tracer            ttrace.Tracer
	logger            *zap.Logger

	setReadyFunc              func(bool)
	currentConcurrentRequests int64
	connectionCountMutex      sync.RWMutex
}

const protoPkfPrefix = "type.googleapis.com/"

func getBlockTypeFromMergedBlocks(store dstore.Store) (string, error) {
	var out string
	fs := bstream.NewFileSource(store, bstream.GetProtocolFirstStreamableBlock, bstream.HandlerFunc(func(blk *pbbstream.Block, obj interface{}) error {
		out = blk.Payload.TypeUrl
		return io.EOF
	}), zlog)
	fs.Run()

	if err := fs.Err(); err != io.EOF {
		return "", err
	}
	return strings.TrimPrefix(out, protoPkfPrefix), nil
}

func NewTier2(
	logger *zap.Logger,
	mergedBlocksStore dstore.Store,

	stateStore dstore.Store,
	defaultCacheTag string,
	stateBundleSize uint64,

	opts ...Option,

) (*Tier2Service, error) {

	runtimeConfig := config.NewRuntimeConfig(
		stateBundleSize,
		0, // tier2 don't send subrequests
		0, // tier2 don't send subrequests
		0,
		stateStore,
		defaultCacheTag,
		nil,
	)

	blockType, err := getBlockTypeFromMergedBlocks(mergedBlocksStore)
	if err != nil {
		return nil, fmt.Errorf("getting block type from merged-blocks-store: %w", err)
	}

	logger.Debug("launching tier2 service", zap.String("block_type", blockType))
	s := &Tier2Service{
		runtimeConfig: runtimeConfig,
		blockType:     blockType,
		tracer:        tracing.GetTracer(),
		logger:        logger,
	}

	sf := &StreamFactory{
		mergedBlocksStore: mergedBlocksStore,
	}

	s.streamFactoryFunc = sf.New

	metrics.RegisterMetricSet(logger)

	for _, opt := range opts {
		opt(s)
	}

	return s, nil
}

func (s *Tier2Service) ProcessRange(request *pbssinternal.ProcessRangeRequest, streamSrv pbssinternal.Substreams_ProcessRangeServer) (grpcError error) {
	// We keep `err` here as the unaltered error from `blocks` call, this is used in the EndSpan to record the full error
	// and not only the `grpcError` one which is a subset view of the full `err`.
	var err error
	ctx := streamSrv.Context()

	overloaded := true
	s.connectionCountMutex.Lock()
	if s.runtimeConfig.MaxConcurrentRequests == 0 || s.currentConcurrentRequests < s.runtimeConfig.MaxConcurrentRequests {
		overloaded = false
	}
	if overloaded {
		defer s.connectionCountMutex.Unlock()
		return connect.NewError(connect.CodeUnavailable, fmt.Errorf("service overloaded"))
	}

	s.currentConcurrentRequests++
	s.setReadyFunc(s.runtimeConfig.MaxConcurrentRequests == 0 || s.currentConcurrentRequests <= s.runtimeConfig.MaxConcurrentRequests)
	defer func() {
		s.connectionCountMutex.Lock()
		s.currentConcurrentRequests--
		s.setReadyFunc(s.runtimeConfig.MaxConcurrentRequests == 0 || s.currentConcurrentRequests < s.runtimeConfig.MaxConcurrentRequests)
		s.connectionCountMutex.Unlock()
	}()
	s.connectionCountMutex.Unlock()

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
	ctx = dmetering.WithBytesMeter(ctx)
	ctx = dmetering.WithCounter(ctx, "wasm_input_bytes")
	ctx = reqctx.WithTracer(ctx, s.tracer)

	ctx, span := reqctx.WithSpan(ctx, "substreams/tier2/request")
	defer span.EndWithErr(&err)
	span.SetAttributes(attribute.Int64("substreams.tier", 2))

	hostname := updateStreamHeadersHostname(streamSrv.SetHeader, logger)
	span.SetAttributes(attribute.String("hostname", hostname))

	if request.Modules == nil {
		return connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("missing modules in request"))
	}
	moduleNames := make([]string, len(request.Modules.Modules))
	for i := 0; i < len(moduleNames); i++ {
		moduleNames[i] = request.Modules.Modules[i].Name
	}

	fields := []zap.Field{
		zap.Uint64("start_block", request.StartBlockNum),
		zap.Uint64("stop_block", request.StopBlockNum),
		zap.Uint32("stage", request.Stage),
		zap.Strings("modules", moduleNames),
		zap.String("output_module", request.OutputModule),
	}

	if auth := dauth.FromContext(ctx); auth != nil {
		fields = append(fields,
			zap.String("user_id", auth.UserID()),
			zap.String("key_id", auth.APIKeyID()),
			zap.String("ip_address", auth.RealIP()),
		)
		if cacheTag := auth.Get("X-Sf-Substreams-Cache-Tag"); cacheTag != "" {
			fields = append(fields,
				zap.String("cache_tag", cacheTag),
			)
		}
	}

	logger.Info("incoming substreams ProcessRange request", fields...)

	respFunc := tier2ResponseHandler(ctx, logger, streamSrv)
	err = s.processRange(ctx, request, respFunc, tracing.GetTraceID(ctx).String())
	grpcError = toGRPCError(ctx, err)

	switch status.Code(grpcError) {
	case codes.Unknown, codes.Internal, codes.Unavailable:
		logger.Info("unexpected termination of stream of blocks", zap.Error(err))
	}

	return grpcError
}

func (s *Tier2Service) processRange(ctx context.Context, request *pbssinternal.ProcessRangeRequest, respFunc substreams.ResponseFunc, traceID string) error {
	logger := reqctx.Logger(ctx)

	if err := outputmodules.ValidateTier2Request(request, s.blockType); err != nil {
		return stream.NewErrInvalidArg(fmt.Errorf("validate request: %w", err).Error())
	}

	// FIXME: here, we validate that we have only modules on the same
	// stage, otherwise we fall back.
	outputGraph, err := outputmodules.NewOutputModuleGraph(request.OutputModule, true, request.Modules)
	if err != nil {
		return stream.NewErrInvalidArg(err.Error())
	}

	requestDetails := pipeline.BuildRequestDetailsFromSubrequest(request)
	ctx = reqctx.WithRequest(ctx, requestDetails)
	if s.runtimeConfig.ModuleExecutionTracing {
		ctx = reqctx.WithModuleExecutionTracing(ctx)
	}

	requestDetails.CacheTag = s.runtimeConfig.DefaultCacheTag
	if auth := dauth.FromContext(ctx); auth != nil {
		if cacheTag := auth.Get("X-Sf-Substreams-Cache-Tag"); cacheTag != "" {
			if IsValidCacheTag(cacheTag) {
				requestDetails.CacheTag = cacheTag
			} else {
				return fmt.Errorf("invalid value for X-Sf-Substreams-Cache-Tag %s, should only contain letters, numbers, hyphens and undescores", cacheTag)
			}
		}
	}

	var requestStats *metrics.Stats
	ctx, requestStats = setupRequestStats(ctx, requestDetails, outputGraph, true)
	defer requestStats.LogAndClose()

	if err := outputGraph.ValidateRequestStartBlock(requestDetails.ResolvedStartBlockNum); err != nil {
		return stream.NewErrInvalidArg(err.Error())
	}

	wasmRuntime := wasm.NewRegistry(s.wasmExtensions, s.runtimeConfig.MaxWasmFuel)

	cacheStore, err := s.runtimeConfig.BaseObjectStore.SubStore(requestDetails.CacheTag)
	if err != nil {
		return fmt.Errorf("internal error setting store: %w", err)
	}

	execOutputConfigs, err := execout.NewConfigs(cacheStore, outputGraph.UsedModules(), outputGraph.ModuleHashes(), s.runtimeConfig.StateBundleSize, logger)
	if err != nil {
		return fmt.Errorf("new config map: %w", err)
	}

	storeConfigs, err := store.NewConfigMap(cacheStore, outputGraph.Stores(), outputGraph.ModuleHashes(), traceID)
	if err != nil {
		return fmt.Errorf("configuring stores: %w", err)
	}
	stores := pipeline.NewStores(ctx, storeConfigs, s.runtimeConfig.StateBundleSize, requestDetails.ResolvedStartBlockNum, request.StopBlockNum, true)

	outputModule := outputGraph.OutputModule()

	var execOutWriter *execout.Writer
	if !outputGraph.StagedUsedModules()[request.Stage].LastLayer().IsStoreLayer() {
		execOutWriter = execout.NewWriter(
			requestDetails.ResolvedStartBlockNum,
			requestDetails.StopBlockNum,
			outputModule.Name,
			execOutputConfigs,
		)
	}

	execOutputCacheEngine, err := cache.NewEngine(ctx, s.runtimeConfig, execOutWriter, s.blockType)
	if err != nil {
		return fmt.Errorf("error building caching engine: %w", err)
	}

	opts := s.buildPipelineOptions(ctx, request)
	opts = append(opts, pipeline.WithFinalBlocksOnly())
	opts = append(opts, pipeline.WithHighestStage(request.Stage))

	pipe := pipeline.New(
		ctx,
		outputGraph,
		stores,
		execOutputConfigs,
		wasmRuntime,
		execOutputCacheEngine,
		s.runtimeConfig,
		respFunc,
		// This must always be the parent/global trace id, the one that comes from tier1
		traceID,
		opts...,
	)

	logger.Debug("initializing tier2 pipeline",
		zap.Uint64("request_start_block", requestDetails.ResolvedStartBlockNum),
		zap.Uint64("request_stop_block", request.StopBlockNum),
		zap.String("output_module", request.OutputModule),
		zap.Uint32("stage", request.Stage),
	)
	if err := pipe.InitTier2Stores(ctx); err != nil {
		return fmt.Errorf("error building pipeline: %w", err)
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

func tier2ResponseHandler(ctx context.Context, logger *zap.Logger, streamSrv pbssinternal.Substreams_ProcessRangeServer) substreams.ResponseFunc {
	meter := dmetering.GetBytesMeter(ctx)
	auth := dauth.FromContext(ctx)
	userID := auth.UserID()
	apiKeyID := auth.APIKeyID()
	userMeta := auth.Meta()
	ip := auth.RealIP()

	return func(respAny substreams.ResponseFromAnyTier) error {
		resp := respAny.(*pbssinternal.ProcessRangeResponse)
		if err := streamSrv.Send(resp); err != nil {
			logger.Info("unable to send block probably due to client disconnecting", zap.Error(err))
			return connect.NewError(connect.CodeUnavailable, err)
		}

		sendMetering(meter, userID, apiKeyID, ip, userMeta, "sf.substreams.internal.v2/ProcessRange", resp, logger)
		return nil
	}
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
// If the `err` has its in chain any error constructed through `connect.NewError` (and its variants), then
// we return the first found error of such type directly, because it's already a gRPC error.
//
// Otherwise, the error is assumed to be an internal error and turned backed into a proper
// `connect.NewError(connect.CodeInternal, err)`.

func toGRPCError(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}

	// already GRPC error
	if grpcError := dgrpc.AsGRPCError(err); grpcError != nil {
		return grpcError.Err()
	}

	// GRPC to connect error
	connectError := &connect.Error{}
	if errors.As(err, &connectError) {
		switch connectError.Code() {
		case connect.CodeCanceled:
			return status.Error(codes.Canceled, err.Error())
		case connect.CodeUnavailable:
			return status.Error(codes.Canceled, err.Error())
		case connect.CodeInvalidArgument:
			return status.Error(codes.InvalidArgument, err.Error())
		case connect.CodeUnknown:
			return status.Error(codes.Unknown, err.Error())
		}
	}

	if errors.Is(err, context.Canceled) {
		if context.Cause(ctx) != nil {
			err = context.Cause(ctx)
			if err == errShuttingDown {
				return status.Error(codes.Unavailable, err.Error())
			}
		}
		return status.Error(codes.Canceled, err.Error())
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return status.Error(codes.DeadlineExceeded, err.Error())
	}
	if store.StoreAboveMaxSizeRegexp.MatchString(err.Error()) {
		return status.Error(codes.InvalidArgument, err.Error())
	}
	if errors.Is(err, exec.ErrWasmDeterministicExec) {
		return status.Error(codes.InvalidArgument, err.Error())
	}

	var errInvalidArg *stream.ErrInvalidArg
	if errors.As(err, &errInvalidArg) {
		return status.Error(codes.InvalidArgument, err.Error())
	}
	return status.Error(codes.Internal, err.Error())
}

package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/streamingfast/bstream"

	"github.com/streamingfast/bstream/hub"
	"github.com/streamingfast/bstream/stream"
	bsstream "github.com/streamingfast/bstream/stream"
	"github.com/streamingfast/dauth"
	"github.com/streamingfast/dgrpc"
	"github.com/streamingfast/dmetering"
	"github.com/streamingfast/dstore"
	"github.com/streamingfast/logging"
	tracing "github.com/streamingfast/sf-tracing"
	"github.com/streamingfast/shutter"

	"github.com/bufbuild/connect-go"
	"github.com/streamingfast/substreams"
	"github.com/streamingfast/substreams/client"
	"github.com/streamingfast/substreams/metrics"
	"github.com/streamingfast/substreams/orchestrator/plan"
	"github.com/streamingfast/substreams/orchestrator/work"
	pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
	ssconnect "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2/pbsubstreamsrpcconnect"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
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
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

type Tier1Service struct {
	*shutter.Shutter
	ssconnect.UnimplementedStreamHandler

	blockType          string
	wasmExtensions     []wasm.WASMExtensioner
	pipelineOptions    []pipeline.PipelineOptioner
	failedRequestsLock sync.RWMutex
	failedRequests     map[string]*recordedFailure
	streamFactoryFunc  StreamFactoryFunc
	runtimeConfig      config.RuntimeConfig
	tracer             ttrace.Tracer
	logger             *zap.Logger

	getRecentFinalBlock func() (uint64, error)
	resolveCursor       pipeline.CursorResolver
	getHeadBlock        func() (uint64, error)
}

func NewTier1(
	logger *zap.Logger,
	mergedBlocksStore dstore.Store,
	forkedBlocksStore dstore.Store,
	hub *hub.ForkableHub,

	stateStore dstore.Store,
	defaultCacheTag string,

	blockType string,

	parallelSubRequests uint64,
	stateBundleSize uint64,

	substreamsClientConfig *client.SubstreamsClientConfig,
	opts ...Option,
) *Tier1Service {

	logger.Info("creating grpc client factory", zap.Reflect("config", substreamsClientConfig))
	clientFactory := client.NewInternalClientFactory(substreamsClientConfig)

	runtimeConfig := config.NewRuntimeConfig(
		stateBundleSize,
		parallelSubRequests,
		10,
		0,
		stateStore,
		defaultCacheTag,
		func(logger *zap.Logger) work.Worker {
			return work.NewRemoteWorker(clientFactory, logger)
		},
	)
	s := &Tier1Service{
		Shutter:        shutter.New(),
		runtimeConfig:  runtimeConfig,
		blockType:      blockType,
		tracer:         tracing.GetTracer(),
		failedRequests: make(map[string]*recordedFailure),
		resolveCursor:  pipeline.NewCursorResolver(hub, mergedBlocksStore, forkedBlocksStore),
		logger:         logger,
	}

	sf := &StreamFactory{
		mergedBlocksStore: mergedBlocksStore,
		forkedBlocksStore: forkedBlocksStore,
		hub:               hub,
	}
	s.streamFactoryFunc = sf.New
	s.getRecentFinalBlock = sf.GetRecentFinalBlock
	s.getHeadBlock = sf.GetHeadBlock

	metrics.RegisterMetricSet(logger)

	for _, opt := range opts {
		opt(s)
	}

	return s
}

func (s *Tier1Service) BlockType() string {
	return s.blockType
}

func (s *Tier1Service) Blocks(
	ctx context.Context,
	req *connect.Request[pbsubstreamsrpc.Request],
	stream *connect.ServerStream[pbsubstreamsrpc.Response],
) error {

	// We keep `err` here as the unaltered error from `blocks` call, this is used in the EndSpan to record the full error
	// and not only the `grpcError` one which is a subset view of the full `err`.
	var err error

	logger := reqctx.Logger(ctx).Named("tier1")

	ctx = logging.WithLogger(ctx, logger)
	ctx = reqctx.WithTracer(ctx, s.tracer)
	ctx = dmetering.WithBytesMeter(ctx)

	ctx, span := reqctx.WithSpan(ctx, "substreams/tier1/request")
	defer span.EndWithErr(&err)

	// We need to ensure that the response function is NEVER used after this Blocks handler has returned.
	// We use a context that will be canceled on defer, and a lock to prevent races. The respFunc is used in various threads
	mut := sync.Mutex{}
	respContext, cancel := context.WithCancel(ctx)
	defer func() {
		mut.Lock()
		cancel()
		mut.Unlock()
	}()

	respFunc := tier1ResponseHandler(respContext, &mut, logger, stream)

	span.SetAttributes(attribute.Int64("substreams.tier", 1))

	request := req.Msg
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

	logger.Info("incoming Substreams Blocks request", fields...)
	metrics.SubstreamsCounter.Inc()
	metrics.ActiveSubstreams.Inc()
	defer metrics.ActiveSubstreams.Dec()

	if err := outputmodules.ValidateTier1Request(request, s.blockType); err != nil {
		return status.Error(codes.InvalidArgument, fmt.Errorf("validate request: %w", err).Error())
	}

	outputGraph, err := outputmodules.NewOutputModuleGraph(request.OutputModule, request.ProductionMode, request.Modules)
	if err != nil {
		return bsstream.NewErrInvalidArg(err.Error())
	}

	requestID := fmt.Sprintf("%s:%d:%d:%s:%t:%t:%s",
		outputGraph.ModuleHashes().Get(request.OutputModule),
		request.StartBlockNum,
		request.StopBlockNum,
		request.StartCursor,
		request.ProductionMode,
		request.FinalBlocksOnly,
		strings.Join(request.DebugInitialStoreSnapshotForModules, ","),
	)

	//	s.resolveCursor
	if err := s.errorFromRecordedFailure(requestID, request.ProductionMode, request.StartBlockNum, request.StartCursor); err != nil {
		logger.Debug("failing fast on known failing request", zap.String("request_id", requestID))
		return err
	}

	// On app shutdown, we cancel the running '.blocks()' command,
	// we catch this situation via IsTerminating() to return a special error.
	runningContext, cancelRunning := context.WithCancelCause(ctx)
	go func() {
		select {
		case <-ctx.Done():
			return
		case <-s.Terminating():
			cancelRunning(fmt.Errorf("endpoint is shutting down, please reconnect"))
		}
	}()

	err = s.blocks(runningContext, request, outputGraph, respFunc)

	if grpcError := toGRPCError(runningContext, err); grpcError != nil {
		switch status.Code(grpcError) {
		case codes.Internal:
			logger.Info("unexpected termination of stream of blocks", zap.String("stream_processor", "tier1"), zap.Error(err))
		case codes.InvalidArgument:
			logger.Debug("recording failure on request", zap.String("request_id", requestID))
			s.recordFailure(requestID, grpcError)
		case codes.Canceled:
			logger.Info("Blocks request canceled by user", zap.Error(grpcError))
		default:
			logger.Info("Blocks request completed with error", zap.Error(grpcError))
		}
		return grpcError
	}

	logger.Info("Blocks request completed witout error")
	return nil
}

func (s *Tier1Service) writePackage(ctx context.Context, request *pbsubstreamsrpc.Request, outputGraph *outputmodules.Graph) error {
	asPackage := &pbsubstreams.Package{
		Modules:    request.Modules,
		ModuleMeta: []*pbsubstreams.ModuleMetadata{},
	}

	cnt, err := proto.Marshal(asPackage)
	if err != nil {
		return fmt.Errorf("marshalling package: %w", err)
	}

	modulePath := filepath.Join(reqctx.Details(ctx).CacheTag, outputGraph.ModuleHashes().Get(request.OutputModule))
	moduleStore, err := s.runtimeConfig.BaseObjectStore.SubStore(modulePath)
	if err != nil {
		return fmt.Errorf("getting substore: %w", err)
	}
	exists, err := moduleStore.FileExists(ctx, "substreams.partial.spkg")
	if err != nil {
		return fmt.Errorf("error checking fileExists: %w", err)
	}
	if !exists {
		if err := moduleStore.WriteObject(ctx, "substreams.partial.spkg", bytes.NewReader(cnt)); err != nil {
			return fmt.Errorf("writing substreams.partial object")
		}
	}
	return nil
}

var IsValidCacheTag = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`).MatchString

func (s *Tier1Service) blocks(ctx context.Context, request *pbsubstreamsrpc.Request, outputGraph *outputmodules.Graph, respFunc substreams.ResponseFunc) error {
	chainFirstStreamableBlock := bstream.GetProtocolFirstStreamableBlock
	if request.StartBlockNum > 0 && request.StartBlockNum < int64(chainFirstStreamableBlock) {
		return stream.NewErrInvalidArg("invalid start block %d, must be >= %d (the first streamable block of the chain)", request.StartBlockNum, chainFirstStreamableBlock)
	} else if request.StartBlockNum < 0 && request.StopBlockNum > 0 {
		if int64(request.StopBlockNum)+int64(request.StartBlockNum) < int64(chainFirstStreamableBlock) {
			request.StartBlockNum = int64(chainFirstStreamableBlock)
		}
	} else if request.StartBlockNum == 0 {
		request.StartBlockNum = int64(chainFirstStreamableBlock)
	}

	logger := reqctx.Logger(ctx)

	requestDetails, undoSignal, err := pipeline.BuildRequestDetails(ctx, request, s.getRecentFinalBlock, s.resolveCursor, s.getHeadBlock)
	if err != nil {
		return fmt.Errorf("build request details: %w", err)
	}

	requestDetails.MaxParallelJobs = s.runtimeConfig.DefaultParallelSubrequests
	requestDetails.CacheTag = s.runtimeConfig.DefaultCacheTag
	if auth := dauth.FromContext(ctx); auth != nil {
		if parallelJobs := auth.Get("X-Sf-Substreams-Parallel-Jobs"); parallelJobs != "" {
			if ll, err := strconv.ParseUint(parallelJobs, 10, 64); err == nil {
				requestDetails.MaxParallelJobs = ll
			}
		}
		if cacheTag := auth.Get("X-Sf-Substreams-Cache-Tag"); cacheTag != "" {
			if IsValidCacheTag(cacheTag) {
				requestDetails.CacheTag = cacheTag
			} else {
				return fmt.Errorf("invalid value for X-Sf-Substreams-Cache-Tag %s, should only contain letters, numbers, hyphens and undescores", cacheTag)
			}
		}

	}

	var requestStats *metrics.Stats
	ctx, requestStats = setupRequestStats(ctx, requestDetails, outputGraph, false)
	defer requestStats.LogAndClose()

	traceId := tracing.GetTraceID(ctx).String()
	respFunc(&pbsubstreamsrpc.Response{
		Message: &pbsubstreamsrpc.Response_Session{
			Session: &pbsubstreamsrpc.SessionInit{
				TraceId:            traceId,
				ResolvedStartBlock: requestDetails.ResolvedStartBlockNum,
				LinearHandoffBlock: requestDetails.LinearHandoffBlockNum,
				MaxParallelWorkers: requestDetails.MaxParallelJobs,
			},
		},
	})

	ctx = reqctx.WithRequest(ctx, requestDetails)
	if s.runtimeConfig.ModuleExecutionTracing {
		ctx = reqctx.WithModuleExecutionTracing(ctx)
	}

	if err := s.writePackage(ctx, request, outputGraph); err != nil {
		logger.Warn("cannot write package", zap.Error(err))
	}

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

	storeConfigs, err := store.NewConfigMap(cacheStore, outputGraph.Stores(), outputGraph.ModuleHashes(), tracing.GetTraceID(ctx).String())
	if err != nil {
		return fmt.Errorf("configuring stores: %w", err)
	}

	stores := pipeline.NewStores(ctx, storeConfigs, s.runtimeConfig.StateBundleSize, requestDetails.LinearHandoffBlockNum, request.StopBlockNum, false)

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
		tracing.GetTraceID(ctx).String(),
		opts...,
	)

	// FIXME: eventually, we could use the `orchestrator/plan.RequestPlan` object to
	// tackle the `LinearHandoffBlockNum == StopBlockNum`, and the linear segment that
	// needs to be produced.
	// But it seems a bit more involved in here.

	scheduleStores := outputGraph.StagedUsedModules()[0].LastLayer().IsStoreLayer()

	reqPlan, err := plan.BuildTier1RequestPlan(
		requestDetails.ProductionMode,
		s.runtimeConfig.StateBundleSize,
		outputGraph.LowestInitBlock(),
		requestDetails.ResolvedStartBlockNum,
		requestDetails.LinearHandoffBlockNum,
		requestDetails.StopBlockNum,
		scheduleStores,
	)
	if err != nil {
		return fmt.Errorf("error building request plan: %w", err)
	}

	logger.Info("initializing tier1 pipeline",
		zap.Stringer("plan", reqPlan),
		zap.Int64("request_start_block", request.StartBlockNum),
		zap.Uint64("resolved_start_block", requestDetails.ResolvedStartBlockNum),
		zap.Uint64("request_stop_block", request.StopBlockNum),
		zap.String("request_start_cursor", request.StartCursor),
		zap.String("resolved_cursor", requestDetails.ResolvedCursor),
		zap.Uint64("max_parallel_jobs", requestDetails.MaxParallelJobs),
		zap.String("output_module", request.OutputModule),
	)

	if err := pipe.InitTier1StoresAndBackprocess(ctx, reqPlan); err != nil {
		return fmt.Errorf("error during init_stores_and_backprocess: %w", err)
	}
	if reqPlan.LinearPipeline == nil {
		return pipe.OnStreamTerminated(ctx, nil)
	}

	var streamErr error
	cursor := requestDetails.ResolvedCursor
	var cursorIsTarget bool
	if requestDetails.ResolvedStartBlockNum != requestDetails.LinearHandoffBlockNum {
		// FIXME(abourget): how is that different from reqPlan.LinearPipeline being set?
		// and what does the cursor have to do here?
		// This will also be true when we've done backprocessing.. is the cursor affected
		// in that case?!
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

func tier1ResponseHandler(ctx context.Context, mut *sync.Mutex, logger *zap.Logger, streamSrv *connect.ServerStream[pbsubstreamsrpc.Response]) substreams.ResponseFunc {
	auth := dauth.FromContext(ctx)
	userID := auth.UserID()
	apiKeyID := auth.APIKeyID()
	userMeta := auth.Meta()
	ip := auth.RealIP()
	meter := dmetering.GetBytesMeter(ctx)

	return func(respAny substreams.ResponseFromAnyTier) error {
		resp := respAny.(*pbsubstreamsrpc.Response)
		mut.Lock()
		defer mut.Unlock()

		// this reponse handler is used in goroutines, sending to streamSrv on closed ctx would panic
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err := streamSrv.Send(resp); err != nil {
			logger.Info("unable to send block probably due to client disconnecting", zap.Error(err))
			return status.Error(codes.Unavailable, err.Error())
		}

		sendMetering(meter, userID, apiKeyID, ip, userMeta, "sf.substreams.rpc.v2/Blocks", resp)
		return nil
	}
}

func setupRequestStats(ctx context.Context, requestDetails *reqctx.RequestDetails, graph *outputmodules.Graph, tier2 bool) (context.Context, *metrics.Stats) {
	logger := reqctx.Logger(ctx)
	auth := dauth.FromContext(ctx)
	stats := metrics.NewReqStats(&metrics.Config{
		UserID:           auth.UserID(),
		ApiKeyID:         auth.APIKeyID(),
		Tier2:            tier2,
		OutputModule:     requestDetails.OutputModule,
		OutputModuleHash: graph.ModuleHashes().Get(requestDetails.OutputModule),
		ProductionMode:   requestDetails.ProductionMode,
	}, logger)
	return reqctx.WithReqStats(ctx, stats), stats
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
func toGRPCError(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}

	if grpcError := dgrpc.AsGRPCError(err); grpcError != nil {
		return grpcError.Err()
	}

	if errors.Is(err, context.Canceled) {
		if context.Cause(ctx) != nil {
			err = context.Cause(ctx)
		}
		return status.Error(codes.Canceled, err.Error())
	}

	if errors.Is(err, context.DeadlineExceeded) {
		return status.Error(codes.DeadlineExceeded, "source deadline exceeded")
	}

	if errors.Is(err, exec.ErrWasmDeterministicExec) {
		return status.Error(codes.InvalidArgument, err.Error())
	}

	var errInvalidArg *stream.ErrInvalidArg
	if errors.As(err, &errInvalidArg) {
		return status.Error(codes.InvalidArgument, errInvalidArg.Error())
	}

	// Do we want to print the full cause as coming from Golang? Would we like to maybe trim off "operational"
	// data?
	return status.Error(codes.Internal, err.Error())
}

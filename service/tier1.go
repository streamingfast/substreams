package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/streamingfast/substreams/metering"

	"github.com/streamingfast/bstream"
	"github.com/streamingfast/dgrpc"

	"github.com/streamingfast/bstream/hub"
	pbbstream "github.com/streamingfast/bstream/pb/sf/bstream/v1"
	bsstream "github.com/streamingfast/bstream/stream"
	"github.com/streamingfast/dauth"
	"github.com/streamingfast/dmetering"
	"github.com/streamingfast/dstore"
	"github.com/streamingfast/logging"
	tracing "github.com/streamingfast/sf-tracing"
	"github.com/streamingfast/shutter"

	"connectrpc.com/connect"
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
	"github.com/streamingfast/substreams/reqctx"
	"github.com/streamingfast/substreams/service/config"
	"github.com/streamingfast/substreams/storage/execout"
	"github.com/streamingfast/substreams/storage/store"
	"github.com/streamingfast/substreams/wasm"
	"go.opentelemetry.io/otel/attribute"
	ttrace "go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/proto"
)

var errShuttingDown = errors.New("endpoint is shutting down, please reconnect")

type Tier1Service struct {
	*shutter.Shutter
	ssconnect.UnimplementedStreamHandler

	blockType             string
	wasmExtensions        map[string]map[string]wasm.WASMExtension
	wasmParams            map[string]string
	failedRequestsLock    sync.RWMutex
	failedRequests        map[string]*recordedFailure
	streamFactoryFunc     StreamFactoryFunc
	blockExecutionTimeout time.Duration
	runtimeConfig         config.RuntimeConfig
	tracer                ttrace.Tracer
	logger                *zap.Logger

	getRecentFinalBlock func() (uint64, error)
	resolveCursor       pipeline.CursorResolver
	getHeadBlock        func() (uint64, error)

	tier2RequestParameters reqctx.Tier2RequestParameters
}

func getBlockTypeFromStreamFactory(sf *StreamFactory) (string, error) {
	var out string
	ctx := context.Background()
	stream, err := sf.New(
		ctx,
		bstream.HandlerFunc(func(blk *pbbstream.Block, obj interface{}) error {
			out = blk.Payload.TypeUrl
			return io.EOF
		}),
		int64(bstream.GetProtocolFirstStreamableBlock),
		bstream.GetProtocolFirstStreamableBlock,
		"", false, false, zlog,
	)
	if err != nil {
		return "", err
	}

	done := make(chan struct{})
	go func() {
		for {
			select {
			case <-done:
				return
			case <-time.After(10 * time.Second):
				zlog.Info("waiting to get the block type")
			}
		}
	}()

	err = stream.Run(ctx)
	close(done)
	if err != io.EOF && err != nil {
		return "", fmt.Errorf("getting block type: %w", err)
	}

	zlog.Info("block type fetched", zap.String("type", out))

	return strings.TrimPrefix(out, protoPkfPrefix), nil
}
func NewTier1(
	logger *zap.Logger,
	mergedBlocksStore dstore.Store,
	forkedBlocksStore dstore.Store,
	hub *hub.ForkableHub,

	stateStore dstore.Store,
	defaultCacheTag string,

	parallelSubRequests uint64,
	stateBundleSize uint64,
	blockType string,

	substreamsClientConfig *client.SubstreamsClientConfig,
	tier2RequestParameters reqctx.Tier2RequestParameters,
	opts ...Option,
) (*Tier1Service, error) {

	clientFactory := client.NewInternalClientFactory(substreamsClientConfig)

	runtimeConfig := config.NewTier1RuntimeConfig(
		stateBundleSize,
		parallelSubRequests,
		10,
		stateStore,
		defaultCacheTag,
		func(logger *zap.Logger) work.Worker {
			return work.NewRemoteWorker(clientFactory, logger)
		},
		clientFactory,
	)

	sf := &StreamFactory{
		mergedBlocksStore: mergedBlocksStore,
		forkedBlocksStore: forkedBlocksStore,
		hub:               hub,
	}

	var err error
	if blockType == "" {
		blockType, err = getBlockTypeFromStreamFactory(sf)
		if err != nil {
			return nil, fmt.Errorf("getting block type from stream factory: %w", err)
		}
	}

	tier2RequestParameters.BlockType = blockType
	tier2RequestParameters.StateBundleSize = runtimeConfig.SegmentSize

	logger.Info("launching tier1 service", zap.Reflect("client_config", substreamsClientConfig), zap.String("block_type", blockType), zap.Bool("with_live", hub != nil))
	s := &Tier1Service{
		Shutter:                shutter.New(),
		runtimeConfig:          runtimeConfig,
		blockType:              blockType,
		tracer:                 tracing.GetTracer(),
		failedRequests:         make(map[string]*recordedFailure),
		resolveCursor:          pipeline.NewCursorResolver(hub, mergedBlocksStore, forkedBlocksStore),
		logger:                 logger,
		tier2RequestParameters: tier2RequestParameters,
		blockExecutionTimeout:  3 * time.Minute,
	}

	s.streamFactoryFunc = sf.New
	s.getRecentFinalBlock = sf.GetRecentFinalBlock
	s.getHeadBlock = sf.GetHeadBlock

	metrics.RegisterMetricSet(logger)

	for _, opt := range opts {
		opt(s)
	}

	return s, nil
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
	ctx = metering.WithMetricsSender(ctx)
	ctx = reqctx.WithTier2RequestParameters(ctx, s.tier2RequestParameters)

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

	request := req.Msg
	if request.Modules == nil {
		return connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("missing modules in request"))
	}

	execGraph, err := exec.NewOutputModuleGraph(request.OutputModule, request.ProductionMode, request.Modules, bstream.GetProtocolFirstStreamableBlock)
	if err != nil {
		return bsstream.NewErrInvalidArg(err.Error())
	}
	outputModuleHash := execGraph.ModuleHashes().Get(request.OutputModule)
	ctx = reqctx.WithOutputModuleHash(ctx, outputModuleHash)

	respFunc := tier1ResponseHandler(respContext, &mut, logger, stream)

	span.SetAttributes(attribute.Int64("substreams.tier", 1))

	if err := ValidateTier1Request(request, s.blockType); err != nil {
		return connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("validate request: %w", err))
	}

	moduleNames := make([]string, len(request.Modules.Modules))
	for i := 0; i < len(moduleNames); i++ {
		moduleNames[i] = request.Modules.Modules[i].Name
	}
	var compressed bool
	for _, vv := range req.Header().Values("grpc-Accept-Encoding") {
		for _, v := range strings.Split(vv, ",") {
			if v == "gzip" {
				compressed = true
				break
			}
		}
	}

	fields := []zap.Field{
		zap.Int64("start_block", request.StartBlockNum),
		zap.Uint64("stop_block", request.StopBlockNum),
		zap.String("cursor", request.StartCursor),
		zap.Strings("modules", moduleNames),
		zap.String("output_module", request.OutputModule),
		zap.String("output_module_hash", outputModuleHash),
		zap.Bool("compressed", compressed),
		zap.Bool("final_blocks_only", request.FinalBlocksOnly),
	}
	fields = append(fields, zap.Bool("production_mode", request.ProductionMode))
	fields = append(fields, zap.Bool("noop_mode", request.NoopMode))
	if auth := dauth.FromContext(ctx); auth != nil {
		fields = append(fields,
			zap.String("user_id", auth.UserID()),
			zap.String("key_id", auth.APIKeyID()),
			zap.String("ip_address", auth.RealIP()),
		)
		if auth["x-deployment-id"] != "" {
			fields = append(fields, zap.String("deployment_id", auth["x-deployment-id"]))
		}

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

	requestID := fmt.Sprintf("%s:%d:%d:%s:%t:%t:%s",
		outputModuleHash,
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
			cancelRunning(errShuttingDown)
		}
	}()

	err = s.blocks(runningContext, request, execGraph, respFunc)

	if connectError := toConnectError(runningContext, err); connectError != nil {
		switch connect.CodeOf(connectError) {
		case connect.CodeInternal:
			logger.Warn("unexpected termination of stream of blocks", zap.String("stream_processor", "tier1"), zap.Error(err))
		case connect.CodeInvalidArgument:
			logger.Debug("recording failure on request", zap.String("request_id", requestID))
			s.recordFailure(requestID, connectError)
		case connect.CodeCanceled:
			logger.Info("Blocks request canceled by user", zap.Error(connectError))
		default:
			logger.Warn("Blocks request completed with error", zap.Error(connectError))
		}
		return connectError
	}

	logger.Debug("Blocks request completed witout error")
	return nil
}

func (s *Tier1Service) writePackage(ctx context.Context, request *pbsubstreamsrpc.Request, execGraph *exec.Graph, cacheStore dstore.Store) error {
	asPackage := &pbsubstreams.Package{
		Modules:    request.Modules,
		ModuleMeta: []*pbsubstreams.ModuleMetadata{},
	}

	cnt, err := proto.Marshal(asPackage)
	if err != nil {
		return fmt.Errorf("marshalling package: %w", err)
	}

	moduleStore, err := cacheStore.SubStore(execGraph.ModuleHashes().Get(request.OutputModule))
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

func (s *Tier1Service) blocks(ctx context.Context, request *pbsubstreamsrpc.Request, execGraph *exec.Graph, respFunc substreams.ResponseFunc) error {
	chainFirstStreamableBlock := bstream.GetProtocolFirstStreamableBlock
	if request.StartBlockNum > 0 && request.StartBlockNum < int64(chainFirstStreamableBlock) {
		return bsstream.NewErrInvalidArg("invalid start block %d, must be >= %d (the first streamable block of the chain)", request.StartBlockNum, chainFirstStreamableBlock)
	} else if request.StartBlockNum < 0 && request.StopBlockNum > 0 {
		if int64(request.StopBlockNum)+int64(request.StartBlockNum) < int64(chainFirstStreamableBlock) {
			request.StartBlockNum = int64(chainFirstStreamableBlock)
		}
	} else if request.StartBlockNum == 0 {
		request.StartBlockNum = int64(chainFirstStreamableBlock)
	}

	logger := reqctx.Logger(ctx)

	requestDetails, undoSignal, err := pipeline.BuildRequestDetails(ctx, request, s.getRecentFinalBlock, s.resolveCursor, s.getHeadBlock, s.runtimeConfig.SegmentSize)
	if err != nil {
		return fmt.Errorf("build request details: %w", err)
	}

	if requestDetails.ResolvedStartBlockNum == request.StopBlockNum && request.StopBlockNum != 0 {
		return bsstream.NewErrInvalidArg("start block and stop block are the same")
	}

	requestDetails.MaxParallelJobs = s.runtimeConfig.DefaultParallelSubrequests
	cacheTag := s.runtimeConfig.DefaultCacheTag
	if auth := dauth.FromContext(ctx); auth != nil {
		if parallelJobs := auth.Get("X-Sf-Substreams-Parallel-Jobs"); parallelJobs != "" {
			if ll, err := strconv.ParseUint(parallelJobs, 10, 64); err == nil {
				requestDetails.MaxParallelJobs = ll
			}
		}
		if ct := auth.Get("X-Sf-Substreams-Cache-Tag"); ct != "" {
			if IsValidCacheTag(ct) {
				cacheTag = ct
			} else {
				return fmt.Errorf("invalid value for X-Sf-Substreams-Cache-Tag %s, should only contain letters, numbers, hyphens and undescores", ct)
			}
		}

	}

	var requestStats *metrics.Stats
	ctx, requestStats = setupRequestStats(ctx, requestDetails, execGraph.ModuleHashes().Get(requestDetails.OutputModule), false)
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

	if err := execGraph.ValidateRequestStartBlock(requestDetails.ResolvedStartBlockNum); err != nil {
		return bsstream.NewErrInvalidArg(err.Error())
	}

	wasmRuntime := wasm.NewRegistry(s.wasmExtensions)

	cacheStore, err := s.runtimeConfig.BaseObjectStore.SubStore(cacheTag)
	if err != nil {
		return fmt.Errorf("internal error setting store: %w", err)
	}

	if clonableStore, ok := cacheStore.(dstore.Clonable); ok {
		cloned, err := clonableStore.Clone(ctx, metering.WithBytesMeteringOptions(dmetering.GetBytesMeter(ctx), logger)...)
		if err != nil {
			return fmt.Errorf("cloning store: %w", err)
		}
		//todo: (deprecated)
		cloned.SetMeter(dmetering.GetBytesMeter(ctx))
		cacheStore = cloned
	}

	if err := s.writePackage(ctx, request, execGraph, cacheStore); err != nil {
		logger.Warn("cannot write package", zap.Error(err))
	}

	segmentSize := s.runtimeConfig.SegmentSize

	execOutputConfigs, err := execout.NewConfigs(cacheStore, execGraph.UsedModules(), execGraph.ModuleHashes(), segmentSize, chainFirstStreamableBlock, logger)
	if err != nil {
		return fmt.Errorf("new config map: %w", err)
	}

	storeConfigs, err := store.NewConfigMap(cacheStore, execGraph.Stores(), execGraph.ModuleHashes(), chainFirstStreamableBlock)
	if err != nil {
		return fmt.Errorf("configuring stores: %w", err)
	}

	stores := pipeline.NewStores(ctx, storeConfigs, segmentSize, requestDetails.LinearHandoffBlockNum, request.StopBlockNum, false, nil)

	execOutputCacheEngine, err := cache.NewEngine(ctx, nil, s.blockType, nil, nil) // we don't read or write ExecOuts on tier1
	if err != nil {
		return fmt.Errorf("error building caching engine: %w", err)
	}

	//opts := s.buildPipelineOptions(ctx)
	var opts []pipeline.Option
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
		execGraph,
		stores,
		nil,
		execOutputConfigs,
		wasmRuntime,
		execOutputCacheEngine,
		segmentSize,
		s.runtimeConfig.WorkerFactory,
		respFunc,
		s.blockExecutionTimeout,
		opts...,
	)

	// FIXME: eventually, we could use the `orchestrator/plan.RequestPlan` object to
	// tackle the `LinearHandoffBlockNum == StopBlockNum`, and the linear segment that
	// needs to be produced.
	// But it seems a bit more involved in here.

	scheduleStores := execGraph.StagedUsedModules()[0].LastLayer().IsStoreLayer()
	var lowestStoresInitBlock uint64
	if scheduleStores {
		lowestStoresInitBlock = *execGraph.LowestStoresInitBlock()
	}

	reqPlan, err := plan.BuildTier1RequestPlan(
		requestDetails.ProductionMode,
		segmentSize,
		execGraph.LowestInitBlock(),
		lowestStoresInitBlock,
		requestDetails.ResolvedStartBlockNum,
		requestDetails.LinearHandoffBlockNum,
		requestDetails.StopBlockNum,
		scheduleStores,
	)
	if err != nil {
		return fmt.Errorf("error building request plan: %w", err)
	}

	logger.Debug("initializing tier1 pipeline",
		zap.Stringer("plan", reqPlan),
		zap.Int64("request_start_block", request.StartBlockNum),
		zap.Uint64("resolved_start_block", requestDetails.ResolvedStartBlockNum),
		zap.Uint64("request_stop_block", request.StopBlockNum),
		zap.String("request_start_cursor", request.StartCursor),
		zap.String("resolved_cursor", requestDetails.ResolvedCursor),
		zap.Uint64("max_parallel_jobs", requestDetails.MaxParallelJobs),
		zap.String("output_module", request.OutputModule),
	)

	if err := pipe.Init(ctx); err != nil {
		return fmt.Errorf("error during pipeline init: %w", err)
	}
	if err := pipe.InitTier1StoresAndBackprocess(ctx, reqPlan); err != nil {
		return fmt.Errorf("error during init_stores_and_backprocess: %w", err)
	}
	if reqPlan.LinearPipeline == nil {
		return pipe.OnStreamTerminated(ctx, io.EOF)
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

	var streamHandler bstream.Handler
	if requestDetails.ProductionMode {
		liveBackFiller := NewLiveBackFiller(pipe, logger, execGraph.OutputModuleStageIndex(), segmentSize, requestDetails.LinearHandoffBlockNum, s.runtimeConfig.ClientFactory, RequestBackProcessing)

		// In noop mode, the pipe handler is overwritten by a NoopHandler which produces no outputs.
		if request.NoopMode {
			noopHandler := NewNoopHandler(respFunc)
			liveBackFiller = NewLiveBackFiller(noopHandler, logger, execGraph.OutputModuleStageIndex(), segmentSize, requestDetails.LinearHandoffBlockNum, s.runtimeConfig.ClientFactory, RequestBackProcessing)
		}

		go liveBackFiller.Start(ctx)
		streamHandler = liveBackFiller
	} else {
		streamHandler = pipe
	}

	blockStream, err := s.streamFactoryFunc(
		ctx,
		streamHandler,
		int64(requestDetails.LinearHandoffBlockNum),
		request.StopBlockNum,
		cursor,
		request.FinalBlocksOnly,
		cursorIsTarget,
		logger.Named("stream"),
		bsstream.WithLiveSourceHandlerMiddleware(metering.LiveSourceMiddlewareHandlerFactory(ctx)),
		bsstream.WithFileSourceHandlerMiddleware(metering.FileSourceMiddlewareHandlerFactory(ctx)),
	)
	if err != nil {
		return fmt.Errorf("error getting stream: %w", err)
	}

	ctx, span := reqctx.WithSpan(ctx, "substreams/tier1/pipeline/blocks_stream")
	streamErr = blockStream.Run(ctx)
	span.EndWithErr(&streamErr)

	return pipe.OnStreamTerminated(ctx, streamErr)
}

func tier1ResponseHandler(ctx context.Context, mut *sync.Mutex, logger *zap.Logger, streamSrv *connect.ServerStream[pbsubstreamsrpc.Response]) substreams.ResponseFunc {
	auth := dauth.FromContext(ctx)
	userID := auth.UserID()
	apiKeyID := auth.APIKeyID()
	userMeta := auth.Meta()
	ip := auth.RealIP()

	ctx = reqctx.WithEmitter(ctx, dmetering.GetDefaultEmitter())
	metericsSender := metering.GetMetricsSender(ctx)

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
			return connect.NewError(connect.CodeUnavailable, err)
		}

		metericsSender.Send(ctx, userID, apiKeyID, ip, userMeta, "sf.substreams.rpc.v2/Blocks", resp)
		return nil
	}
}

func setupRequestStats(ctx context.Context, requestDetails *reqctx.RequestDetails, outputModuleGraph string, tier2 bool) (context.Context, *metrics.Stats) {
	logger := reqctx.Logger(ctx)
	auth := dauth.FromContext(ctx)
	stats := metrics.NewReqStats(&metrics.Config{
		UserID:           auth.UserID(),
		ApiKeyID:         auth.APIKeyID(),
		Tier2:            tier2,
		OutputModule:     requestDetails.OutputModule,
		OutputModuleHash: outputModuleGraph,
		ProductionMode:   requestDetails.ProductionMode,
	}, logger)
	return reqctx.WithReqStats(ctx, stats), stats
}

// toConnectError turns an `err` into a connect error if it's non-nil, in the `nil` case,
// `nil` is returned right away.
//
// If the `err` has in its chain of error either `context.Canceled`, `context.DeadlineExceeded`
// or `stream.ErrInvalidArg`, error is turned into a proper connect error respectively of code
// `Canceled`, `DeadlineExceeded` or `InvalidArgument`.
//
// If the `err` has in its chain any error constructed through `connect.NewError` (and its variants), then
// we return the first found error of such type directly, because it's already a connect error.
//
// If the `err` has in its chain any error constructed through `grpc` or `status`, it will be converted to connect equivalent.
//
// Otherwise, the error is assumed to be an internal error and turned backed into a proper
// `connect.NewError(connect.CodeInternal, err)`.
func toConnectError(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}

	// GRPC to connect error
	if grpcError := dgrpc.AsGRPCError(err); grpcError != nil {
		switch grpcError.Code() {
		case codes.Canceled:
			return connect.NewError(connect.CodeCanceled, grpcError.Err())
		case codes.Unavailable:
			return connect.NewError(connect.CodeUnavailable, grpcError.Err())
		case codes.InvalidArgument:
			return connect.NewError(connect.CodeInvalidArgument, grpcError.Err())
		case codes.Unknown:
			return connect.NewError(connect.CodeUnknown, grpcError.Err())
		}
		return grpcError.Err()
	}

	// special case for context canceled when shutting down
	if errors.Is(err, context.Canceled) {
		if context.Cause(ctx) != nil {
			err = context.Cause(ctx)
			if err == errShuttingDown {
				return connect.NewError(connect.CodeUnavailable, err)
			}
		}
		return connect.NewError(connect.CodeCanceled, err)
	}

	// context deadline exceeded
	if errors.Is(err, context.DeadlineExceeded) {
		return connect.NewError(connect.CodeDeadlineExceeded, err)
	}

	if store.StoreAboveMaxSizeRegexp.MatchString(err.Error()) {
		return connect.NewError(connect.CodeInvalidArgument, err)
	}

	if errors.Is(err, exec.ErrWasmDeterministicExec) {
		return connect.NewError(connect.CodeInvalidArgument, err)
	}

	var errInvalidArg *bsstream.ErrInvalidArg
	if errors.As(err, &errInvalidArg) {
		return connect.NewError(connect.CodeInvalidArgument, errInvalidArg)
	}

	// Do we want to print the full cause as coming from Golang? Would we like to maybe trim off "operational"
	// data?
	return connect.NewError(connect.CodeInternal, err)
}

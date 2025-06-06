package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"connectrpc.com/connect"
	"github.com/streamingfast/bstream"
	"github.com/streamingfast/bstream/hub"
	pbbstream "github.com/streamingfast/bstream/pb/sf/bstream/v1"
	bsstream "github.com/streamingfast/bstream/stream"
	"github.com/streamingfast/dauth"
	"github.com/streamingfast/dgrpc"
	"github.com/streamingfast/dmetering"
	"github.com/streamingfast/dmetrics"
	"github.com/streamingfast/dstore"
	"github.com/streamingfast/logging"
	tracing "github.com/streamingfast/sf-tracing"
	"github.com/streamingfast/shutter"
	"github.com/streamingfast/substreams"
	"github.com/streamingfast/substreams/client"
	"github.com/streamingfast/substreams/manifest"
	"github.com/streamingfast/substreams/metering"
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
	pbworker "github.com/streamingfast/worker-pool-protocol/pb/sf/worker/v1"
	"go.opentelemetry.io/otel/attribute"
	ttrace "go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

var errShuttingDown = errors.New("endpoint is shutting down, please reconnect")

type Tier1Service struct {
	*shutter.Shutter
	ssconnect.UnimplementedStreamHandler
	activeRequests sync.WaitGroup

	blockType             string
	wasmExtensions        map[string]map[string]wasm.WASMExtension
	wasmParams            map[string]string
	failedRequestsLock    sync.RWMutex
	streamFactoryFunc     StreamFactoryFunc
	blockExecutionTimeout time.Duration
	runtimeConfig         config.RuntimeConfig
	tracer                ttrace.Tracer
	logger                *zap.Logger

	// You can call this function to switch the parent app to be ready or not ready influencing the health check,
	// it's provided by [app.Tier1App] and tied to the health check endpoint.
	appSetIsReadyState  func(isReady bool)
	getRecentFinalBlock func() (uint64, error)
	resolveCursor       pipeline.CursorResolver
	getHeadBlock        func() (uint64, error)

	enforceCompression      bool
	activeRequestsSoftLimit int
	activeRequestsHardLimit int
	tier2RequestParameters  reqctx.Tier2RequestParameters
	globalRequestPool       *GlobalRequestPool
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
	quickSaveStore dstore.Store,
	defaultCacheTag string,

	parallelSubRequests uint64,
	stateBundleSize uint64,
	blockType string,

	appSetIsReadyState func(isReady bool),
	substreamsClientConfig *client.SubstreamsClientConfig,
	tier2RequestParameters reqctx.Tier2RequestParameters,
	workerPoolFactory work.WorkerPoolFactory,

	enforceCompression bool,
	activeRequestsSoftLimit int,
	activeRequestsHardLimit int,
	sharedCacheSize uint64,
	globalRequestPool *GlobalRequestPool,
	opts ...Option,
) (*Tier1Service, error) {

	clientFactory := client.NewInternalClientFactory(substreamsClientConfig)

	runtimeConfig := config.NewTier1RuntimeConfig(
		stateBundleSize,
		parallelSubRequests,
		10,
		stateStore,
		quickSaveStore,
		defaultCacheTag,
		clientFactory,
		workerPoolFactory,
	)

	sf := &StreamFactory{
		mergedBlocksStore: mergedBlocksStore,
		forkedBlocksStore: forkedBlocksStore,
		hub:               hub,
	}

	setSubstreamsStoreSizeLimitFromEnv(logger)

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
		Shutter:                 shutter.New(),
		runtimeConfig:           runtimeConfig,
		blockType:               blockType,
		tracer:                  tracing.GetTracer(),
		resolveCursor:           pipeline.NewCursorResolver(hub, mergedBlocksStore, forkedBlocksStore),
		logger:                  logger,
		appSetIsReadyState:      appSetIsReadyState,
		tier2RequestParameters:  tier2RequestParameters,
		blockExecutionTimeout:   3 * time.Minute,
		enforceCompression:      enforceCompression,
		activeRequestsSoftLimit: activeRequestsSoftLimit,
		activeRequestsHardLimit: activeRequestsHardLimit,
		globalRequestPool:       globalRequestPool,
	}
	s.OnTerminating(func(_ error) {
		s.activeRequests.Wait()
	})

	go func() {
		if sharedCacheSize == 0 {
			zlog.Info("shared cache disabled")
			return
		}

		if hub == nil {
			zlog.Info("shared cache disabled, no live source configured")
			return
		}

		<-hub.Ready
		sharedCache := exec.NewSharedCache(sharedCacheSize)
		hubSrc := hub.SourceFromBlockNum(hub.HeadNum(), sharedCache)
		if hubSrc == nil {
			zlog.Error("cannot get blocks source from hub")
			return
		}
		exec.GlobalSharedCache = sharedCache
		hubSrc.Run()
		if err := hubSrc.Err(); err != nil {
			zlog.Info("shared cache source stopped", zap.Error(err))
		}
	}()

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
) (serverErr error) {

	if s.IsTerminating() {
		serverErr = connect.NewError(connect.CodeUnavailable, errShuttingDown)
		return
	}
	s.activeRequests.Add(1)
	defer func() {
		if reason, countAsRejected := metrics.IsRejectedRequestError(serverErr); countAsRejected {
			metrics.Tier1RejectedRequestCounter.Inc(reason)
		}
		s.activeRequests.Done()
	}()

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

	request := req.Msg
	var compressed bool
	if matchHeader(req.Header()) {
		compressed = true
	}

	fields := []zap.Field{
		zap.Int64("start_block", request.StartBlockNum),
		zap.Uint64("stop_block", request.StopBlockNum),
		zap.String("cursor", request.StartCursor),
		zap.String("output_module", request.OutputModule),
		zap.Bool("compressed", compressed),
		zap.Bool("final_blocks_only", request.FinalBlocksOnly),
		zap.Bool("production_mode", request.ProductionMode),
		zap.Bool("noop_mode", request.NoopMode),
	}

	if s.enforceCompression && !compressed {
		err := connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("your client does not accept gzip- or zstd-compressed streams. Check how to enable it on your gRPC or ConnectRPC client"))
		fields = append(fields, zap.Error(err))
		logger.Info("refusing Substreams Blocks request", fields...)
		return err
	}

	if request.Modules == nil {
		err := connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("missing modules in request"))
		fields = append(fields, zap.Error(err))
		logger.Info("refusing Substreams Blocks request", fields...)
		return err
	}

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

	status := s.getOverloadedStatus()

	// Set us as unready if the soft limit would be reached by this request
	if status.softLimitWouldBeReached() {
		s.logger.Debug("soft limit would be reached by this request, setting app as unready",
			append(fields, zap.Int("active_request_count", status.activeRequestCount), zap.Int("soft_limit", status.softLimit))...,
		)
		s.appSetIsReadyState(false)
	}

	// Refuse the request if the hard limit is currently reached by this instance
	if status.hardLimitReached() {
		err := connect.NewError(connect.CodeUnavailable, fmt.Errorf("service under heavy load, please try connecting again"))
		fields = append(fields, zap.Error(err), zap.Int("active_request_count", status.activeRequestCount), zap.Int("hard_limit", status.hardLimit))
		logger.Info("refusing Substreams Blocks request", fields...)
		return err
	}

	execGraph, err := exec.NewOutputModuleGraph(request.OutputModule, request.ProductionMode, request.Modules, bstream.GetProtocolFirstStreamableBlock)
	if err != nil {
		err := connect.NewError(connect.CodeInvalidArgument, err)
		fields = append(fields, zap.Error(err))
		logger.Info("refusing Substreams Blocks request", fields...)
		return err
	}

	outputModuleHash := execGraph.ModuleHashes()[request.OutputModule]

	ctx = reqctx.WithOutputModuleHash(ctx, outputModuleHash)
	fields = append(fields, zap.String("output_module_hash", outputModuleHash))

	usedModules := execGraph.UsedModules()

	var hasStores bool
	var hasFilter bool
	moduleNames := make([]string, len(usedModules))
	for i, module := range usedModules {
		moduleNames[i] = module.Name
		if module.GetKindStore() != nil {
			hasStores = true
		}
		if module.BlockFilter != nil {
			hasFilter = true
		}
	}
	fields = append(fields,
		zap.Strings("modules", moduleNames),
		zap.Bool("with_stores", hasStores),
		zap.Bool("with_blockfilter", hasFilter),
		zap.Int("module_count", len(usedModules)),
	)

	// We need to ensure that the response function is NEVER used after this Blocks handler has returned.
	// We use a context that will be canceled on defer, and a lock to prevent races. The respFunc is used in various threads
	mut := sync.Mutex{}
	respContext, cancel := context.WithCancel(ctx)
	defer func() {
		mut.Lock()
		cancel()
		mut.Unlock()
	}()

	span.SetAttributes(attribute.Int64("substreams.tier", 1))

	if err := ValidateTier1Request(request, s.blockType); err != nil {
		err := connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("validate request: %w", err))
		logger.Info("refusing Substreams Blocks request", append(fields, zap.Error(err))...)
		return err
	}

	var reqStats *metrics.Stats
	ctx, reqStats = setupRequestStats(ctx, request.OutputModule, outputModuleHash, request.ProductionMode, false)

	metrics.SubstreamsCounter.Inc()
	metrics.ActiveRequests.Inc()
	defer func() {
		metrics.ActiveRequests.Dec()

		if status := s.getOverloadedStatus(); status.canAcceptUpcomingRequests() {
			s.appSetIsReadyState(true)
		}
	}()

	// On app shutdown, we cancel the running '.blocks()' command,
	// we catch this situation via IsTerminating() to return a special error.
	runningContext, cancelRunning := context.WithCancelCause(ctx)
	go func() {
		select {
		case <-ctx.Done():
			return
		case <-s.Terminating():
			<-time.After(30 * time.Second) // max delay to wait for stuck connections
			cancelRunning(errShuttingDown)
		}
	}()

	respFunc := tier1ResponseHandler(respContext, &mut, logger, stream, request.NoopMode, reqStats)
	err = s.blocks(runningContext, request, execGraph, respFunc, reqStats, fields)

	if connectError := toConnectError(runningContext, err); connectError != nil {
		switch connect.CodeOf(connectError) {
		case connect.CodeInternal:
			logger.Warn("unexpected termination of stream of blocks", zap.String("stream_processor", "tier1"), zap.Error(err))
		case connect.CodeInvalidArgument:
			logger.Debug("invalid argument on request", zap.Error(connectError))
		case connect.CodeCanceled:
			logger.Debug("Blocks request canceled by user", zap.Error(connectError))
		default:
			logger.Warn("Blocks request completed with error", zap.Error(connectError))
		}
		return connectError
	}
	logger.Debug("Blocks request completed without error")
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

	moduleStore, err := cacheStore.SubStore(execGraph.ModuleHashes()[request.OutputModule])
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

func (s *Tier1Service) writeLastUsed(ctx context.Context, execGraph *exec.Graph, cacheStore dstore.Store) error {
	for _, module := range execGraph.UsedModules() {
		moduleStore, err := cacheStore.SubStore(execGraph.ModuleHashes()[module.Name])
		if err != nil {
			return fmt.Errorf("getting substore: %w", err)
		}
		moduleStore.SetOverwrite(true)
		if err := moduleStore.WriteObject(ctx, "last_used", strings.NewReader(time.Now().Format("2006-01-02"))); err != nil {
			return fmt.Errorf("writing last_used file")
		}
	}
	return nil
}

var IsValidCacheTag = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`).MatchString

func (s *Tier1Service) blocks(ctx context.Context, request *pbsubstreamsrpc.Request, execGraph *exec.Graph, respFunc substreams.ResponseFunc, reqStats *metrics.Stats, logFields []zap.Field) (err error) {
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
		err = fmt.Errorf("build request details: %w", err)
		logger.Info("refusing Substreams Blocks request", append(logFields, zap.Error(err))...)
		return err
	}

	if request.StopBlockNum != 0 {
		if requestDetails.ResolvedStartBlockNum == request.StopBlockNum {
			err := bsstream.NewErrInvalidArg("start block and stop block are the same")
			logger.Info("refusing Substreams Blocks request", append(logFields, zap.Error(err))...)
			return err
		}

		if requestDetails.ResolvedStartBlockNum > request.StopBlockNum {
			err := bsstream.NewErrInvalidArg("resolved start block %d is below stop block %d", requestDetails.ResolvedStartBlockNum, request.StopBlockNum)
			logger.Info("refusing Substreams Blocks request", append(logFields, zap.Error(err))...)
			return err
		}
	}

	requestDetails.MaxParallelJobs = s.runtimeConfig.DefaultParallelSubrequests
	cacheTag := s.runtimeConfig.DefaultCacheTag
	if auth := dauth.FromContext(ctx); auth != nil {
		if parallelJobs := auth.Get("X-Sf-Substreams-Parallel-Jobs"); parallelJobs != "" {
			if count, err := strconv.ParseUint(parallelJobs, 10, 64); err == nil {
				requestDetails.MaxParallelJobs = count
			}
		}
		if tag := auth.Get("X-Sf-Substreams-Cache-Tag"); tag != "" {
			if IsValidCacheTag(tag) {
				cacheTag = tag
			} else {
				return fmt.Errorf("invalid value for X-Sf-Substreams-Cache-Tag %s, should only contain letters, numbers, hyphens and underscores", tag)
			}
		}

		requestDetails.SetStageLayerParallelExecutorCountFromContext(ctx)
	}

	ctx = reqctx.WithRequest(ctx, requestDetails)
	if s.runtimeConfig.ModuleExecutionTracing {
		ctx = reqctx.WithModuleExecutionTracing(ctx)
	}

	if err := execGraph.ValidateRequestStartBlock(requestDetails.ResolvedStartBlockNum); err != nil {
		err = bsstream.NewErrInvalidArg("%s", err.Error())
		logger.Info("refusing Substreams Blocks request", append(logFields, zap.Error(err))...)
		return err
	}

	cacheStore, err := s.runtimeConfig.BaseObjectStore.SubStore(cacheTag)
	if err != nil {
		err = fmt.Errorf("internal error setting store: %w", err)
		logger.Info("refusing Substreams Blocks request", append(logFields, zap.Error(err))...)
		return err
	}

	segmentSize := s.runtimeConfig.SegmentSize

	// determine if we should refuse the request because of a previously found deterministic error
	startBlockErrorCheck := requestDetails.ResolvedStartBlockNum
	stopBlockErrorCheck := request.StopBlockNum
	if request.StopBlockNum == 0 {
		stopBlockErrorCheck = requestDetails.LinearHandoffBlockNum
	}
	if requestDetails.LinearHandoffBlockNum > startBlockErrorCheck {
		startBlockErrorCheck = startBlockErrorCheck / segmentSize * segmentSize                     // round down to the nearest segment
		stopBlockErrorCheck = ((stopBlockErrorCheck - 1) / segmentSize * segmentSize) + segmentSize // round up to the nearest segment
	}
	if err := s.containsDeterministicError(ctx, startBlockErrorCheck, stopBlockErrorCheck, execGraph, cacheStore); err != nil {
		logger.Info("refusing Substreams Blocks request", append(logFields, zap.Error(err))...)

		// Calculate random sleep duration to prevent heavy reconnections on failed requests
		randomMillis := 1000 + rand.Intn(4000) // between 1 and 5 seconds
		sleepDuration := time.Duration(randomMillis) * time.Millisecond
		logger.Debug("sleeping before returning deterministic error", zap.Duration("duration", sleepDuration))
		time.Sleep(sleepDuration)

		return err
	}

	wasmRuntime := wasm.NewRegistry(s.wasmExtensions)

	quickSaveStore := s.runtimeConfig.QuickSaveStore

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

	if err := s.writeLastUsed(ctx, execGraph, cacheStore); err != nil {
		logger.Warn("cannot write 'last_used' file", zap.Error(err))
	}

	execOutputConfigs, err := execout.NewConfigs(cacheStore, execGraph.UsedModules(), execGraph.ModuleHashes(), segmentSize, chainFirstStreamableBlock, logger)
	if err != nil {
		return fmt.Errorf("new config map: %w", err)
	}

	storeConfigs, err := store.NewConfigMap(cacheStore, quickSaveStore, execGraph.Stores(), execGraph.ModuleHashes(), chainFirstStreamableBlock)
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

	if s.getHeadBlock != nil {
		opts = append(opts, pipeline.WithHeadBlockGetter(s.getHeadBlock))
	}

	pipe := pipeline.New(
		ctx,
		true,
		execGraph,
		stores,
		nil,
		execOutputConfigs,
		wasmRuntime,
		execOutputCacheEngine,
		segmentSize,
		s.runtimeConfig.WorkerPoolFactory,
		respFunc,
		s.blockExecutionTimeout,
		func() bool {
			return s.IsTerminating() // pipeline starts draining when the service is actually terminating, (after the global shutdown-signal-delay)
		},
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

	if s.globalRequestPool != nil {
		userID := dauth.FromContext(ctx).UserID()
		apiKeyID := dauth.FromContext(ctx).APIKeyID()
		r := s.globalRequestPool.BorrowRequest(ctx, userID, apiKeyID, tracing.GetTraceID(ctx).String())
		if r.status == pbworker.BorrowWorkerResponse_resource_exhausted {
			msg := strings.Builder{}
			msg.WriteString("Request quota exceeded.\n")
			msg.WriteString(fmt.Sprintf("Your allowed %d concurrent requests.\n", r.state.MaxWorkers))
			msg.WriteString(fmt.Sprintf("Each request has a minimal life time of %s\n", r.minimalWorkerLifeDuration.String()))
			return status.Errorf(codes.ResourceExhausted, "%s", msg.String())
		}

		defer func() {
			zlog.Info("returning request", zap.Bool("keep", false), zap.String("key", r.key))
			if s.IsTerminating() {
				s.logger.Info("returning request without minimal life time. Server is shutting down", zap.String("key", r.key))
				r.minimalWorkerLifeDuration = 0
			}
			s.globalRequestPool.ReturnRequest(r)
		}()
	}

	logger.Info("incoming Substreams Blocks request", logFields...)

	defer func() {
		switch {
		case errors.Is(err, context.Canceled):
			reqStats.SetError(context.Canceled)
		default:
			reqStats.SetError(err)
		}
		reqStats.LogAndClose(ctx, requestDetails.ResolvedStartBlockNum)
	}()

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
	loadedFromQuicksave, err := pipe.InitTier1StoresAndBackprocess(ctx, reqPlan, request.NoopMode)
	if err != nil {
		return fmt.Errorf("error during init_stores_and_backprocess: %w", err)
	}
	if reqPlan.LinearPipeline == nil {
		return pipe.OnStreamTerminated(ctx, io.EOF)
	}

	var streamErr error
	cursor := requestDetails.ResolvedCursor
	var processBlocksBeforeCursor bool
	if !loadedFromQuicksave &&
		request.StartCursor != "" &&
		requestDetails.ResolvedStartBlockNum != requestDetails.LinearHandoffBlockNum {
		// if we have a cursor and our linearHandoff is NOT specifically set to our resolved startBlock,
		// we ask the pipeline to process blocks before the cursor (between the linearHandoffBlockNum and the cursor)
		// so that the stores are correctly populated from the last boundary to our cursor
		processBlocksBeforeCursor = true
	}
	logger.Info("creating firehose stream",
		zap.Uint64("handoff_block", requestDetails.LinearHandoffBlockNum),
		zap.Uint64("stop_block", request.StopBlockNum),
		zap.String("cursor", cursor),
	)

	var streamHandler bstream.Handler
	if requestDetails.ProductionMode {
		liveBackFiller := NewLiveBackFiller(ctx, pipe, logger, execGraph.OutputModuleStageIndex(), segmentSize, requestDetails.LinearHandoffBlockNum, s.runtimeConfig.ClientFactory, RequestBackProcessing)

		// In noop mode, the pipe handler is overwritten by a NoopHandler which produces no outputs.
		if request.NoopMode {
			noopHandler := NewNoopHandler(respFunc)
			liveBackFiller = NewLiveBackFiller(ctx, noopHandler, logger, execGraph.OutputModuleStageIndex(), segmentSize, requestDetails.LinearHandoffBlockNum, s.runtimeConfig.ClientFactory, RequestBackProcessing)
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
		processBlocksBeforeCursor,
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

func tier1ResponseHandler(ctx context.Context, mut *sync.Mutex, logger *zap.Logger, streamSrv *connect.ServerStream[pbsubstreamsrpc.Response], noop bool, stats *metrics.Stats) substreams.ResponseFunc {
	auth := dauth.FromContext(ctx)
	userID := auth.UserID()
	apiKeyID := auth.APIKeyID()
	userMeta := auth.Meta()
	ip := auth.RealIP()

	outputModuleHash := reqctx.OutputModuleHash(ctx)

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

		if data := resp.GetBlockScopedData(); data != nil {
			stats.RecordDataSent()
			if noop {
				data.DebugMapOutputs = nil
				data.DebugStoreOutputs = nil
				data.Output = &pbsubstreamsrpc.MapModuleOutput{}
			}
		}

		if err := streamSrv.Send(resp); err != nil {
			logger.Info("unable to send block probably due to client disconnecting", zap.String("user_id", userID), zap.String("api_key_id", apiKeyID))
			return connect.NewError(connect.CodeUnavailable, err)
		}

		metering.AddEgressBytes(ctx, proto.Size(resp))
		metericsSender.Send(ctx, userID, apiKeyID, ip, userMeta, outputModuleHash, "sf.substreams.rpc.v2/Blocks")
		return nil
	}
}

func (s *Tier1Service) containsDeterministicError(ctx context.Context, startBlock, endBlock uint64, execGraph *exec.Graph, cacheStore dstore.Store) error {
	for _, module := range execGraph.UsedModules() {

		hash := execGraph.ModuleHashes()[module.Name]
		moduleStore, err := cacheStore.SubStore(hash)
		if err != nil {
			return fmt.Errorf("getting substore: %w", err)
		}

		extendedHash := manifest.ExtendedModuleHash(module, hash)

		if err := containsDeterministicError(ctx, moduleStore, module.Name, extendedHash, startBlock, endBlock, module.GetKindStore() != nil, s.logger); err != nil {
			return connect.NewError(connect.CodeInvalidArgument, err)
		}
	}
	return nil
}

func parseFilename(in string) (blockNum uint64, moduleExtendedHash string, err error) {
	in = strings.TrimPrefix(in, "errors.")

	if len(in) < 10 {
		return 0, "", err
	}

	blockNumStr := in[:10]
	blockNum, err = strconv.ParseUint(blockNumStr, 10, 64)
	if err != nil {
		return 0, "", err
	}

	if len(in) > 10 {
		moduleExtendedHash = in[11:] // ignore the '.' between blocknum and moduleExtendedHash
	}
	return blockNum, moduleExtendedHash, nil
}

func containsDeterministicError(ctx context.Context, moduleStore dstore.Store, moduleName, extendedHash string, startBlock, endBlock uint64, isStore bool, logger *zap.Logger) error {
	var lastError error

	startFile := fmt.Sprintf("errors.%010d", startBlock)
	if isStore {
		startFile = "" // for stores, any error preceding the startBlock will prevent execution
	}

	moduleStore.WalkFrom(ctx, "errors.", startFile, func(filename string) (err error) {

		blockNum, parsedExtendedHash, err := parseFilename(filename)
		if err != nil {
			logger.Warn("checking for errors: invalid filename", zap.String("filename", filename), zap.Error(err))
			return nil
		}

		if parsedExtendedHash == "" {
			logger.Info("deleting old deterministic error without extended hash", zap.String("filename", filename))
			moduleStore.DeleteObject(ctx, filename)
			return nil
		}

		if parsedExtendedHash != extendedHash {
			logger.Info("ignoring error on another version of the same module", zap.String("filename", filename), zap.String("parsedExtendedHash", parsedExtendedHash), zap.String("extendedHash", extendedHash))
			return nil
		}

		if endBlock != 0 && blockNum > endBlock {
			return io.EOF
		}

		obj, err := moduleStore.OpenObject(ctx, filename)
		if err != nil {
			logger.Warn("checking for errors: cannot open file", zap.String("filename", filename), zap.Error(err))
			return nil
		}

		cnt, err := io.ReadAll(obj)
		if err != nil {
			logger.Warn("checking for errors: cannot read file", zap.String("filename", filename), zap.Error(err))
			return nil
		}

		lastError = fmt.Errorf("error from block %d in module %s: %s", blockNum, moduleName, string(cnt))
		return nil
	})

	if lastError != nil && lastError != io.EOF {
		return lastError
	}
	return lastError
}

func setupRequestStats(ctx context.Context, outputModuleName, outputModuleHash string, productionMode, tier2 bool) (context.Context, *metrics.Stats) {
	logger := reqctx.Logger(ctx)
	auth := dauth.FromContext(ctx)
	stats := metrics.NewReqStats(&metrics.Config{
		UserID:           auth.UserID(),
		ApiKeyID:         auth.APIKeyID(),
		Tier2:            tier2,
		OutputModule:     outputModuleName,
		OutputModuleHash: outputModuleHash,
		ProductionMode:   productionMode,
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
		case codes.DeadlineExceeded:
			return connect.NewError(connect.CodeDeadlineExceeded, err)
		case codes.ResourceExhausted:
			return connect.NewError(connect.CodeResourceExhausted, grpcError.Err())
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

	// special case for "QuickSave" on shutdown
	if err == pipeline.ErrShuttingDown {
		return connect.NewError(connect.CodeUnavailable, err)
	}

	// context deadline exceeded
	if errors.Is(err, context.DeadlineExceeded) {
		return connect.NewError(connect.CodeDeadlineExceeded, err)
	}

	if errors.Is(err, wasm.ErrWasmDeterministicExec) || errors.Is(err, store.ErrStoreAboveMaxSize) {
		return connect.NewError(connect.CodeInvalidArgument, err)
	}

	var errInvalidArg *bsstream.ErrInvalidArg
	if errors.As(err, &errInvalidArg) {
		return connect.NewError(connect.CodeInvalidArgument, errInvalidArg)
	}

	connectError := new(connect.Error)
	if errors.As(err, &connectError) {
		return connectError
	}

	// Do we want to print the full cause as coming from Golang? Would we like to maybe trim off "operational"
	// data?
	return connect.NewError(connect.CodeInternal, err)
}

// must be lowercase
var compressionHeader = map[string]map[string]bool{
	"grpc-accept-encoding":    {"gzip": true, "zstd": true},
	"connect-accept-encoding": {"gzip": true, "zstd": true},
	"accept-encoding":         {"gzip": true}, // HTTP encoding for connect+proto in browser
}

func matchHeader(header http.Header) bool {
	for k, v := range header {
		if validEncodings, ok := compressionHeader[strings.ToLower(k)]; ok {
			for _, vv := range v {
				for _, vvv := range strings.Split(vv, ",") {
					if validEncodings[strings.TrimSpace(strings.ToLower(vvv))] {
						return true
					}
				}
			}
		}
	}
	return false
}

type overloadingStatus struct {
	// set only if either soft or hard limit set is > 0
	activeRequestCount int
	softLimit          int
	hardLimit          int
}

// softLimitWouldBeReached returns true if the soft limit would be reached if one more request was added.
func (s *overloadingStatus) softLimitWouldBeReached() bool {
	return s.softLimit > 0 && s.activeRequestCount+1 >= s.softLimit
}

// hardLimitReached returns true if the hard limit is actually reached from the active request count.
func (s *overloadingStatus) hardLimitReached() bool {
	return s.hardLimit > 0 && s.activeRequestCount >= s.hardLimit
}

// canAcceptUpcomingRequests returns true if the service can accept upcoming new requests.
func (s *overloadingStatus) canAcceptUpcomingRequests() bool {
	if s.softLimit <= 0 && s.hardLimit <= 0 {
		return true
	}

	if s.softLimit > 0 && s.activeRequestCount >= s.softLimit {
		return false
	}

	if s.hardLimit > 0 && s.activeRequestCount >= s.hardLimit {
		return false
	}

	return true
}

func (s *Tier1Service) getOverloadedStatus() (status overloadingStatus) {
	// Never overloaded if both soft & hard limit are 0, -1 or anything less
	if s.activeRequestsSoftLimit <= 0 && s.activeRequestsHardLimit <= 0 {
		return
	}

	activeRequestCount := s.getActiveRequestCount()

	return overloadingStatus{
		activeRequestCount: activeRequestCount,
		softLimit:          s.activeRequestsSoftLimit,
		hardLimit:          s.activeRequestsHardLimit,
	}
}

func (s *Tier1Service) getActiveRequestCount() int {
	return int(dmetrics.NewValueFromMetric(metrics.ActiveRequests, "requests").ValueUint())
}

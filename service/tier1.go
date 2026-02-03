package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"connectrpc.com/connect"
	"github.com/streamingfast/bstream"
	"github.com/streamingfast/bstream/hub"
	pbbstream "github.com/streamingfast/bstream/pb/sf/bstream/v1"
	"github.com/streamingfast/bstream/stream"
	bsstream "github.com/streamingfast/bstream/stream"
	"github.com/streamingfast/dauth"
	"github.com/streamingfast/dgrpc"
	"github.com/streamingfast/dmetering"
	"github.com/streamingfast/dmetrics"
	"github.com/streamingfast/dsession"
	"github.com/streamingfast/dstore"
	"github.com/streamingfast/logging"
	tracing "github.com/streamingfast/sf-tracing"
	"github.com/streamingfast/shutter"
	"github.com/streamingfast/substreams"
	"github.com/streamingfast/substreams/client"
	"github.com/streamingfast/substreams/debugapi"
	"github.com/streamingfast/substreams/manifest"
	"github.com/streamingfast/substreams/metering"
	"github.com/streamingfast/substreams/metrics"
	"github.com/streamingfast/substreams/orchestrator/plan"
	"github.com/streamingfast/substreams/orchestrator/work"
	pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
	pbsubstreamsrpcv2connect "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2/pbsubstreamsrpcv2connect"
	pbsubstreamsrpcv3 "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v3"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/pipeline"
	"github.com/streamingfast/substreams/pipeline/cache"
	"github.com/streamingfast/substreams/pipeline/exec"
	"github.com/streamingfast/substreams/reqctx"
	"github.com/streamingfast/substreams/service/active_requests"
	"github.com/streamingfast/substreams/service/config"
	"github.com/streamingfast/substreams/storage/execout"
	"github.com/streamingfast/substreams/storage/store"
	"github.com/streamingfast/substreams/wasm"
	"go.opentelemetry.io/otel/attribute"
	ttrace "go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/proto"
)

var errShuttingDown = errors.New("endpoint is shutting down, please reconnect")

type Tier1Service struct {
	*shutter.Shutter
	pbsubstreamsrpcv2connect.UnimplementedStreamHandler
	activeRequestsWG sync.WaitGroup

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
	foundationalEndpoints   map[string]string
	sessionPool             dsession.SessionPool
	activeRequestsManager   *active_requests.ActiveRequestsManager // we keep a list of current requests for the debugAPI and to manage memory
}

func getBlockTypeFromStreamFactory(sf *StreamFactory) (string, error) {
	var out string
	ctx := context.Background()
	stream, err := sf.New(
		ctx,
		bstream.HandlerFunc(func(blk *pbbstream.Block, obj any) error {
			out = blk.Payload.TypeUrl
			return io.EOF
		}),
		int64(bstream.GetProtocolFirstStreamableBlock),
		bstream.GetProtocolFirstStreamableBlock,
		"",
		false,
		false,
		false,
		zlog,
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
	enforceCompression bool,
	activeRequestsSoftLimit int,
	activeRequestsHardLimit int,
	sharedCacheSize uint64,
	sessionPool dsession.SessionPool,
	foundationalEndpoints map[string]string,
	opts ...Option,
) (*Tier1Service, error) {

	clientFactory := client.NewInternalClientFactory(substreamsClientConfig)

	// Create WorkerPoolFactory using the sessionPool
	workerPoolFactory := work.NewSessionWorkerPoolFactory(sessionPool, clientFactory)

	runtimeConfig := config.NewTier1RuntimeConfig(
		stateBundleSize,
		parallelSubRequests,
		10,
		stateStore,
		quickSaveStore,
		defaultCacheTag,
		clientFactory,
		workerPoolFactory.WorkerPool,
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
		foundationalEndpoints:   foundationalEndpoints,
		sessionPool:             sessionPool,
		activeRequestsManager:   active_requests.NewActiveRequestsManager(logger),
	}
	s.OnTerminating(func(_ error) {
		s.activeRequestsWG.Wait()
	})

	if debugAPIAddress := os.Getenv("SUBSTREAMS_TIER1_DEBUG_API_ADDR"); debugAPIAddress != "" {
		debugAPI := debugapi.New(
			debugAPIAddress,
			logger,
			nil, // not used on tier1
			nil,
			s.listActiveRecords,
			s.cancelRequest,
		)
		debugAPI.Start()
	}

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
			zlog.Error("shared cache: cannot get blocks source from hub")
			return
		}
		exec.GlobalSharedCache = sharedCache
		hubSrc.Run()
		if err := hubSrc.Err(); err != nil {
			zlog.Info("shared cache source stopped", zap.Error(err))
		}
	}()

	go func() {
		if hub == nil {
			zlog.Info("undo manager disabled, no live source configured")
			return
		}

		<-hub.Ready
		undoManager := exec.NewUndoManager()
		hubSrc := hub.SourceFromBlockNum(hub.HeadNum(), undoManager)
		if hubSrc == nil {
			zlog.Error("undoManager: cannot get blocks source from hub")
			return
		}
		exec.GlobalUndoManager = undoManager
		hubSrc.Run()
		if err := hubSrc.Err(); err != nil {
			zlog.Info("undo managersource stopped", zap.Error(err))
		}
	}()

	s.streamFactoryFunc = sf.New
	s.getRecentFinalBlock = sf.GetRecentFinalBlock
	s.getHeadBlock = sf.GetHeadBlock

	for _, opt := range opts {
		opt(s)
	}

	return s, nil
}

func (s *Tier1Service) BlocksV3(
	ctx context.Context,
	req *connect.Request[pbsubstreamsrpcv3.Request],
	stream *connect.ServerStream[pbsubstreamsrpc.Response],
) error {
	r := req.Msg

	_, err := manifest.ApplyPackageTransformations(r.Package, false, r.Network, r.OutputModule, r.Params)
	if err != nil {
		return err
	}

	ctx = reqctx.WithSpkg(ctx, r.Package) // passing by context is simpler for now, this could be cleaned up
	reqV2, err := r.ToV2()
	if err != nil {
		return fmt.Errorf("failed to convert request to v2: %w", err)
	}

	return s.BlocksAny(ctx, reqV2, req.Header(), "/sf.substreams.rpc.v3.Stream/Blocks", r.Package, stream)
}

type usedStore struct {
	Name string
	Hash string
}

func (s *usedStore) MarshalLogObject(e zapcore.ObjectEncoder) error {
	e.AddString("name", s.Name)
	e.AddString("hash", s.Hash)
	return nil
}

type UsedFoundationalStore struct {
	Identifier string
	ModuleHash string
}

func (s *UsedFoundationalStore) MarshalLogObject(e zapcore.ObjectEncoder) error {
	e.AddString("identifier", s.Identifier)
	e.AddString("module_hash", s.ModuleHash)

	return nil
}

func (s *Tier1Service) Blocks(
	ctx context.Context,
	req *connect.Request[pbsubstreamsrpc.Request],
	stream *connect.ServerStream[pbsubstreamsrpc.Response],
) (serverErr error) {
	return s.BlocksAny(ctx, req.Msg, req.Header(), "/sf.substreams.rpc.v2.Stream/Blocks", nil, stream)
}

func (s *Tier1Service) BlocksAny(
	ctx context.Context,
	request *pbsubstreamsrpc.Request,
	header http.Header,
	protocol string,
	pkg *pbsubstreams.Package,
	stream *connect.ServerStream[pbsubstreamsrpc.Response],
) (serverErr error) {

	if s.IsTerminating() {
		serverErr = connect.NewError(connect.CodeUnavailable, errShuttingDown)
		return
	}
	s.activeRequestsWG.Add(1)
	defer func() {
		if reason, countAsRejected := metrics.IsRejectedRequestError(serverErr); countAsRejected {
			s.logger.Debug("rejected request", zap.String("reason", reason), zap.Error(serverErr))
			metrics.Tier1RejectedRequestCounter.Inc(reason)
		}
		s.activeRequestsWG.Done()
	}()

	ctx = reqctx.WithPartialBlocks(ctx, request.PartialBlocks)

	// We keep `err` here as the unaltered error from `blocks` call, this is used in the EndSpan to record the full error
	// and not only the `grpcError` one which is a subset view of the full `err`.
	var err error

	logger := reqctx.Logger(ctx).Named("tier1")

	envEthCallFallbackToLatestDuration := os.Getenv(EnvEthCallFallbackToLatestDuration)
	fallbackDuration := time.Duration(0)
	if envEthCallFallbackToLatestDuration != "" {
		fallbackDuration, err = time.ParseDuration(envEthCallFallbackToLatestDuration)
		if err != nil {
			return fmt.Errorf("invalid value for env var %s: %w", EnvEthCallFallbackToLatestDuration, err)
		}
	}

	envEthCallUseBlockNumberDuration := os.Getenv(EnvEthCallUseBlockNumberDuration)
	useBlockNumberDuration := time.Duration(0)
	if envEthCallUseBlockNumberDuration != "" {
		useBlockNumberDuration, err = time.ParseDuration(envEthCallUseBlockNumberDuration)
		if err != nil {
			return fmt.Errorf("invalid value for env var %s: %w", EnvEthCallUseBlockNumberDuration, err)
		}
	}

	ctx = logging.WithLogger(ctx, logger)
	ctx = reqctx.WithTracer(ctx, s.tracer)
	ctx = dmetering.WithBytesMeter(ctx)
	ctx = metering.WithMetricsSender(ctx)
	ctx = reqctx.WithTier2RequestParameters(ctx, s.tier2RequestParameters)
	ctx = reqctx.WithEthCallFallbackToLatestDuration(ctx, fallbackDuration)
	ctx = reqctx.WithEthCallUseBlockNumberDuration(ctx, useBlockNumberDuration)

	ctx, span := reqctx.WithSpan(ctx, "substreams/tier1/request")
	defer span.EndWithErr(&err)

	var compressed bool
	if matchHeader(header) {
		compressed = true
	}

	fields := []zap.Field{
		zap.String("protocol", protocol),
		zap.Int64("start_block", request.StartBlockNum),
		zap.Uint64("stop_block", request.StopBlockNum),
		zap.String("cursor", request.StartCursor),
		zap.String("output_module", request.OutputModule),
		zap.Bool("compressed", compressed),
		zap.Bool("final_blocks_only", request.FinalBlocksOnly),
		zap.Bool("production_mode", request.ProductionMode),
		zap.Bool("noop_mode", request.NoopMode),
		zap.Strings("dev_output_modules", request.DevOutputModules),
		zap.Bool("partial_blocks", request.PartialBlocks),
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

		if cacheTag := auth.Get(reqctx.HeaderCacheTag); cacheTag != "" {
			fields = append(fields,
				zap.String("cache_tag", cacheTag),
			)
		}
	}

	stat := s.getOverloadedStatus()

	// Set us as unready if the soft limit would be reached by this request
	if stat.softLimitWouldBeReached() {
		s.logger.Debug("soft limit would be reached by this request, setting app as unready",
			append(fields, zap.Int("active_request_count", stat.activeRequestCount), zap.Int("soft_limit", stat.softLimit))...,
		)
		s.appSetIsReadyState(false)
	}

	// Refuse the request if the hard limit is currently reached by this instance
	if stat.hardLimitReached() {
		err := connect.NewError(connect.CodeUnavailable, fmt.Errorf("service under heavy load, please try connecting again"))
		fields = append(fields, zap.Error(err), zap.Int("active_request_count", stat.activeRequestCount), zap.Int("hard_limit", stat.hardLimit))
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

	usedStoreMap := map[string]*usedStore{}
	usedFstoreMap := map[string]*UsedFoundationalStore{}

	var hasStores bool
	var hasFilter bool
	moduleNames := make([]string, len(usedModules))
	for i, module := range usedModules {
		moduleNames[i] = module.Name
		if module.GetKindStore() != nil {
			hasStores = true
			h := execGraph.ModuleHashes()[module.Name]
			usedStoreMap[h] = &usedStore{
				Name: module.Name,
				Hash: h,
			}
		}
		for _, i := range module.Inputs {
			if fs := i.GetFoundationalStore(); fs != nil {
				usedFstoreMap[fs.Identifier] = &UsedFoundationalStore{
					Identifier: fs.Identifier,
					ModuleHash: execGraph.ModuleHashes()[module.Name],
				}
			}
		}

		if module.BlockFilter != nil {
			hasFilter = true
		}
	}

	stores := func() []*usedStore {
		var out []*usedStore
		for _, s := range usedStoreMap {
			out = append(out, s)
		}
		return out
	}

	fstores := func() []*UsedFoundationalStore {
		var out []*UsedFoundationalStore
		for _, s := range usedFstoreMap {
			out = append(out, s)
		}
		return out
	}

	fields = append(fields,
		zap.Strings("modules", moduleNames),
		zap.Bool("with_stores", hasStores),
		zap.Bool("with_blockfilter", hasFilter),
		zap.Int("module_count", len(usedModules)),
		zap.Objects("stores", stores()),
		zap.Objects("foundational_stores", fstores()),
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

	if envEthCallFallbackToLatestDuration != "" && hasEthCall(request.Modules.Binaries) {
		if header.Get("X-substreams-acknowledge-non-deterministic") != "true" {
			err := connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("header X-substreams-acknowledge-non-deterministic must be set to true when using eth_call or eth_get_balance on a non deterministic rpc provider"))
			logger.Info("refusing Substreams Blocks request", append(fields, zap.Error(err))...)
			return err
		}
	}

	var reqStats *metrics.Stats
	ctx, reqStats = setupRequestStats(ctx, request.OutputModule, outputModuleHash, execGraph, request.ProductionMode, false)

	metrics.Tier1RequestsCounter.Inc()
	metrics.Tier1ActiveRequests.Inc()
	defer func() {
		metrics.Tier1ActiveRequests.Dec()

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

	runningContext = reqctx.WithCancelFunc(runningContext, cancelRunning) // we pass this down so that the 'workerPool/requestPool' can cancel the running context

	respFunc := tier1ResponseHandler(respContext, &mut, logger, stream, request.NoopMode, reqStats, request.DevOutputModules)
	err = s.blocks(runningContext, cancelRunning, request, header, execGraph, respFunc, reqStats, fields)

	if connectError := toConnectError(runningContext, err); connectError != nil {
		switch connect.CodeOf(connectError) {
		case connect.CodeInternal:
			logger.Warn("unexpected termination of stream of blocks", zap.String("stream_processor", "tier1"), zap.Error(err))
		case connect.CodeInvalidArgument:
			logger.Debug("invalid argument on request", zap.Error(connectError))
		case connect.CodeCanceled:
			logger.Debug("Blocks request canceled by user", zap.Error(connectError))
		case connect.CodeResourceExhausted:
			logger.Debug("Blocks request failed with ResourceExhausted", zap.Error(connectError))
		default:
			logger.Warn("Blocks request completed with error", zap.Error(connectError))
		}
		return connectError
	}
	logger.Debug("Blocks request completed without error")
	return nil
}

// writePackage writes the spkg to the module cache if it doesn't exist:
//   - `substreams.spkg.zst` if it comes from a substreams.rpc.v3 request (package is complete with metadata)
//   - `substreams.partial.spkg.zst` if it comes from a substreams.rpc.v2 request (package is partial, missing protobuf definitions and other metadata)
func (s *Tier1Service) writePackage(ctx context.Context, request *pbsubstreamsrpc.Request, execGraph *exec.Graph, cacheStore dstore.Store) error {
	var pkg *pbsubstreams.Package
	var fileName string

	if receivedSpkg := reqctx.Spkg(ctx); receivedSpkg != nil {
		pkg = receivedSpkg
		fileName = "substreams.spkg"
	} else {
		pkg = &pbsubstreams.Package{
			Modules:    request.Modules,
			ModuleMeta: []*pbsubstreams.ModuleMetadata{},
		}
		fileName = "substreams.partial.spkg"
	}
	moduleStore, err := cacheStore.SubStore(execGraph.ModuleHashes()[request.OutputModule])
	if err != nil {
		return fmt.Errorf("getting substore: %w", err)
	}

	exists, err := moduleStore.FileExists(ctx, fileName)
	if err != nil {
		return fmt.Errorf("error checking fileExists: %w", err)
	}
	if !exists {
		cnt, err := proto.Marshal(pkg)
		if err != nil {
			return fmt.Errorf("marshalling package: %w", err)
		}
		if err := moduleStore.WriteObject(ctx, fileName, bytes.NewReader(cnt)); err != nil {
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

func (s *Tier1Service) blocks(ctx context.Context, cancelRunning context.CancelCauseFunc, request *pbsubstreamsrpc.Request, header http.Header, execGraph *exec.Graph, respFunc substreams.ResponseFunc, reqStats *metrics.Stats, logFields []zap.Field) (err error) {
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
			err := bsstream.NewErrInvalidArg("start block and stop block are the same: %d and %d", requestDetails.ResolvedStartBlockNum, request.StopBlockNum)
			logger.Info("refusing Substreams Blocks request", append(logFields, zap.Error(err))...)
			return err
		}

		if requestDetails.ResolvedStartBlockNum > request.StopBlockNum {
			err := bsstream.NewErrInvalidArg("stop block %d is below resolved start block %d", requestDetails.ResolvedStartBlockNum, request.StopBlockNum)
			logger.Info("refusing Substreams Blocks request", append(logFields, zap.Error(err))...)
			return err
		}
	}
	if request.ProgressMessagesIntervalMs != 0 && request.ProgressMessagesIntervalMs < 500 {
		err := bsstream.NewErrInvalidArg("Invalid progress_messages_interval_ms %q (minimum 500)", request.ProgressMessagesIntervalMs)
		logger.Info("refusing Substreams Blocks request", append(logFields, zap.Error(err))...)
		return err
	}
	requestDetails.UpdateInterval = time.Duration(request.ProgressMessagesIntervalMs) * time.Millisecond

	cacheTag := s.runtimeConfig.DefaultCacheTag
	if ct := dauth.FromContext(ctx).Get(reqctx.HeaderCacheTag); ct != "" {
		if IsValidCacheTag(ct) {
			cacheTag = ct
		}
	}

	parallelJobs, parallelExecutors := reqctx.GetEffectiveHeaderValues(ctx, header, s.runtimeConfig.DefaultParallelSubrequests, reqctx.DefaultMaxStageLayerParallelExecutorCount)
	requestDetails.MaxParallelJobs = parallelJobs
	requestDetails.MaxStageLayerParallelExecutor = parallelExecutors

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
		s.blockType,
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
		s.foundationalEndpoints,
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
		if lowestStoresInitBlock > requestDetails.LinearHandoffBlockNum {
			lowestStoresInitBlock = requestDetails.LinearHandoffBlockNum
		}
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
		return stream.NewErrInvalidArg("%s", err.Error())
	}

	if s.sessionPool != nil {
		auth := dauth.FromContext(ctx)
		userID := auth.UserID()
		apiKeyID := auth.APIKeyID()
		traceID := tracing.GetTraceID(ctx).String()
		service := "t1r"

		sessionID, err := s.sessionPool.Get(ctx, service, userID, apiKeyID, traceID, func(err error) {
			if cancelRunning != nil { // in tests, this might be nil
				cancelRunning(err)
			}
		})

		if err != nil {
			switch {
			case errors.Is(err, dsession.ErrConcurrentStreamLimitExceeded),
				errors.Is(err, dsession.ErrPermissionDenied),
				errors.Is(err, dsession.ErrQuotaExceeded):
				s.logger.Info("session denied to user", zap.String("user_id", userID), zap.String("api_key_id", apiKeyID), zap.String("trace_id", traceID), zap.Error(err))
			default:
				s.logger.Error("failed to acquire session", zap.Error(err), zap.String("service", service), zap.String("user_id", userID), zap.String("api_key_id", apiKeyID), zap.String("trace_id", traceID))
			}
			return err
		}

		s.logger.Debug("acquired session", zap.String("session_id", sessionID))

		// Pass sessionKey through context for WorkerPool
		ctx = reqctx.WithSessionKey(ctx, sessionID)

		defer func() {
			s.logger.Debug("releasing session", zap.String("session_id", sessionID))
			s.sessionPool.Release(sessionID)
		}()
	}

	traceID := tracing.GetTraceID(ctx).String()

	if s.activeRequestsManager != nil {
		activeReqHandler := s.activeRequestsManager.Add(
			cancelRunning,
			traceID,
			reqctx.OutputModuleHash(ctx),
			0, // not used on tier1
			0,
			0,
		)
		defer func() {
			s.activeRequestsManager.Remove(activeReqHandler)
			cancelRunning(context.Canceled) // in case nothing canceled it before
		}()
		ctx = reqctx.WithActiveRequestsHandler(ctx, activeReqHandler)
	} else {
		// we put this here, even if it is unrelated, to avoid setting more than one function to defer
		defer func() {
			if cancelRunning != nil { // in tests, this might be nil
				cancelRunning(err)
			}
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

	var wrappedPipe bstream.Handler
	if requestDetails.ProductionMode {
		liveBackFiller := NewLiveBackFiller(ctx, pipe, logger, execGraph.OutputModuleStageIndex(), segmentSize, requestDetails.LinearHandoffBlockNum, s.runtimeConfig.ClientFactory, RequestBackProcessing)

		// In noop mode, the pipe handler is overwritten by a NoopHandler which produces no outputs.
		if request.NoopMode {
			noopHandler := NewNoopHandler(respFunc)
			liveBackFiller = NewLiveBackFiller(ctx, noopHandler, logger, execGraph.OutputModuleStageIndex(), segmentSize, requestDetails.LinearHandoffBlockNum, s.runtimeConfig.ClientFactory, RequestBackProcessing)
		}

		if requestDetails.FromQuickload {
			if err := configureLiveBackFillerFromQuickload(ctx, segmentSize, requestDetails.LinearHandoffBlockNum, execGraph.UsedStoreModules(), storeConfigs, liveBackFiller); err != nil {
				return err
			}
		}

		go liveBackFiller.Start(ctx)
		wrappedPipe = liveBackFiller
	} else {
		wrappedPipe = pipe
	}

	blockStream, err := s.streamFactoryFunc(
		ctx,
		wrappedPipe,
		int64(requestDetails.LinearHandoffBlockNum),
		request.StopBlockNum,
		cursor,
		request.FinalBlocksOnly,
		reqctx.PartialBlocks(ctx),
		processBlocksBeforeCursor,
		logger.Named("stream"),
		bsstream.WithLiveSourceHandlerMiddleware(metering.LiveSourceMiddlewareHandlerFactory(ctx)),
		bsstream.WithFileSourceHandlerMiddleware(metering.FileSourceMiddlewareHandlerFactory(ctx)),
	)
	if err != nil {
		return fmt.Errorf("error getting stream: %w", err)
	}

	ctx, span := reqctx.WithSpan(ctx, "substreams/tier1/pipeline/blocks_stream")
	for {
		streamErr = blockStream.Run(ctx)
		if errors.Is(streamErr, hub.ErrSubscriptionChannelFull) {
			cur := pipe.LastCursor()
			if cur == nil {
				logger.Warn("subscription channel at max capacity, but no cursor was found to reconnect")
				break
			}

			logger.Warn("subscription channel at max capacity, creating new stream", zap.String("last_block", cur.Block.String()))
			blockStream, err = s.streamFactoryFunc(
				ctx,
				wrappedPipe,
				int64(requestDetails.LinearHandoffBlockNum),
				request.StopBlockNum,
				cur.ToOpaque(),
				request.FinalBlocksOnly,
				reqctx.PartialBlocks(ctx),
				false, // processBlocksBeforeCursor always false here
				logger.Named("stream"),
				bsstream.WithLiveSourceHandlerMiddleware(metering.LiveSourceMiddlewareHandlerFactory(ctx)),
				bsstream.WithFileSourceHandlerMiddleware(metering.FileSourceMiddlewareHandlerFactory(ctx)),
			)
			if err != nil {
				streamErr = fmt.Errorf("error getting stream: %w", err)
				break
			}
			continue
		}
		break
	}

	span.EndWithErr(&streamErr)

	return pipe.OnStreamTerminated(ctx, streamErr)
}

// configureLiveBackFillerFromQuickload will ensure that any used store
func configureLiveBackFillerFromQuickload(ctx context.Context, segmentSize uint64, linearHandoffBlockNum uint64, usedStores []*pbsubstreams.Module, storeConfigs store.ConfigMap, liveBackFiller *LiveBackFiller) error {
	backfillFromIndex := linearHandoffBlockNum / segmentSize

	for _, mod := range usedStores {
		lowestIndex := mod.InitialBlock / segmentSize
		for i := 0; backfillFromIndex > lowestIndex; i++ {

			listUpTo := backfillFromIndex * segmentSize
			for range 3 { // a bit faster to check for 3 files than to check for 1 file more often. usually, only a single file would be missed
				if backfillFromIndex > lowestIndex {
					backfillFromIndex--
				}
			}
			if i > 4 {
				backfillFromIndex = lowestIndex
			}

			files, err := storeConfigs[mod.Name].ListSnapshotFiles(ctx, backfillFromIndex*segmentSize, &listUpTo)
			if err != nil {
				return err
			}
			if files != nil {
				backfillFromIndex = files[len(files)-1].Range.ExclusiveEndBlock / segmentSize
				break
			}
		}
		liveBackFiller.Rewind(backfillFromIndex)

	}
	return nil
}

func tier1ResponseHandler(ctx context.Context, mut *sync.Mutex, logger *zap.Logger, streamSrv *connect.ServerStream[pbsubstreamsrpc.Response], noop bool, stats *metrics.Stats, debugOutputForModules []string) substreams.ResponseFunc {
	auth := dauth.FromContext(ctx)
	organizationID := auth.OrganizationID()
	apiKeyID := auth.APIKeyID()
	userMeta := auth.Meta()
	ip := auth.RealIP()

	outputModuleHash := reqctx.OutputModuleHash(ctx)

	endpoint := "sf.substreams.rpc.v2/Blocks"
	if reqctx.Spkg(ctx) != nil { // if we got the full spkg, we are on the v3 endpoint
		endpoint = "sf.substreams.rpc.v3/Blocks"
	}

	ctx = reqctx.WithEmitter(ctx, dmetering.GetDefaultEmitter())
	metericsSender := metering.GetMetricsSender(ctx)
	var debugOutputs map[string]struct{}
	if len(debugOutputForModules) != 0 {
		debugOutputs = make(map[string]struct{})
		for _, module := range debugOutputForModules {
			debugOutputs[module] = struct{}{}
		}
	}

	return func(respAny substreams.ResponseFromAnyTier) error {
		resp := respAny.(*pbsubstreamsrpc.Response)
		mut.Lock()
		defer mut.Unlock()

		// this response handler is used in goroutines, sending to streamSrv on closed ctx would panic
		if ctx.Err() != nil {
			return ctx.Err()
		}

		var isData bool
		if data := resp.GetBlockScopedData(); data != nil {
			isData = true
			if noop {
				data.DebugMapOutputs = nil
				data.DebugStoreOutputs = nil
				data.Output = &pbsubstreamsrpc.MapModuleOutput{}
			}
			if debugOutputs != nil {
				// Filter DebugMapOutputs
				var filteredMapOutputs []*pbsubstreamsrpc.MapModuleOutput
				for _, output := range data.DebugMapOutputs {
					if _, exists := debugOutputs[output.Name]; exists {
						filteredMapOutputs = append(filteredMapOutputs, output)
					}
				}
				data.DebugMapOutputs = filteredMapOutputs

				// Filter DebugStoreOutputs
				var filteredStoreOutputs []*pbsubstreamsrpc.StoreModuleOutput
				for _, output := range data.DebugStoreOutputs {
					if _, exists := debugOutputs[output.Name]; exists {
						filteredStoreOutputs = append(filteredStoreOutputs, output)
					}
				}
				data.DebugStoreOutputs = filteredStoreOutputs
			}
		}
		egressBytes := proto.Size(resp)

		begin := time.Now()
		if err := streamSrv.Send(resp); err != nil {
			logger.Info("unable to send block probably due to client disconnecting", zap.String("user_id", organizationID), zap.String("api_key_id", apiKeyID), zap.Error(err))
			return connect.NewError(connect.CodeUnavailable, err)
		}
		stats.RecordReadTime(begin)

		if isData {
			stats.RecordDataSent()
		}
		stats.RecordEgress(egressBytes)
		metering.AddEgressBytes(ctx, egressBytes)

		metericsSender.Send(ctx, organizationID, apiKeyID, ip, userMeta, outputModuleHash, endpoint)
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

func setupRequestStats(ctx context.Context, outputModuleName, outputModuleHash string, execGraph *exec.Graph, productionMode, tier2 bool) (context.Context, *metrics.Stats) {
	logger := reqctx.Logger(ctx)
	auth := dauth.FromContext(ctx)
	stats := metrics.NewReqStats(&metrics.Config{
		UserID:           auth.UserID(),
		ApiKeyID:         auth.APIKeyID(),
		Tier2:            tier2,
		OutputModule:     outputModuleName,
		OutputModuleHash: outputModuleHash,
		ProductionMode:   productionMode,
	}, execGraph.Stores(), execGraph.ModuleHashes(), logger)
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

	if errors.Is(err, context.Canceled) {
		if contextCause := context.Cause(ctx); contextCause != nil {
			err = contextCause // unwrap errors in canceled contexts
			if errors.Is(err, context.Canceled) {
				return connect.NewError(connect.CodeCanceled, err)
			}
		} else {
			return connect.NewError(connect.CodeCanceled, err)
		}
	}

	if err, ok := dsession.ToConnectError(err); ok {
		return err
	}
	// special case for context canceled when shutting down
	if err == errShuttingDown {
		return connect.NewError(connect.CodeUnavailable, err)
	}

	// GRPC to connect error
	if grpcError := dgrpc.AsGRPCError(err); grpcError != nil {
		switch grpcError.Code() {
		case codes.Canceled:
			return connect.NewError(connect.CodeCanceled, errors.New(grpcError.Message()))
		case codes.Unavailable:
			return connect.NewError(connect.CodeUnavailable, errors.New(grpcError.Message()))
		case codes.InvalidArgument:
			return connect.NewError(connect.CodeInvalidArgument, errors.New(grpcError.Message()))
		case codes.DeadlineExceeded:
			return connect.NewError(connect.CodeDeadlineExceeded, err)
		case codes.ResourceExhausted:
			return connect.NewError(connect.CodeResourceExhausted, errors.New(grpcError.Message()))
		case codes.Unknown:
			return connect.NewError(connect.CodeUnknown, errors.New(grpcError.Message()))
		}
		return grpcError.Err()
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
		return connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("%w (deterministic error)", err))
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

func (s *Tier1Service) listActiveRecords() string {
	b, err := json.Marshal(s.activeRequestsManager.List())
	if err != nil {
		return err.Error()
	}
	return string(b)
}

func (s *Tier1Service) cancelRequest(traceID string, outputModuleHash string, segmentNumber, segmentSize *uint64, stage *uint32) []string {
	return s.activeRequestsManager.CancelRequest(
		traceID,
		outputModuleHash,
		segmentNumber,
		segmentSize,
		stage,
	)
}

func (s *Tier1Service) getActiveRequestCount() int {
	return int(dmetrics.NewValueFromMetric(metrics.Tier1ActiveRequests, "requests").ValueUint())
}

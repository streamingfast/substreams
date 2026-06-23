package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"connectrpc.com/connect"
	"github.com/RoaringBitmap/roaring/roaring64"
	"github.com/streamingfast/bstream/stream"
	bsstream "github.com/streamingfast/bstream/stream"
	"github.com/streamingfast/dauth"
	"github.com/streamingfast/dgrpc"
	"github.com/streamingfast/dmetering"
	"github.com/streamingfast/dstore"
	"github.com/streamingfast/logging"
	"github.com/streamingfast/logging/zapx"
	tracing "github.com/streamingfast/sf-tracing"
	"github.com/streamingfast/shutter"
	"github.com/streamingfast/substreams"
	"github.com/streamingfast/substreams/block"
	"github.com/streamingfast/substreams/debugapi"
	"github.com/streamingfast/substreams/metering"
	"github.com/streamingfast/substreams/metrics"
	pbssinternal "github.com/streamingfast/substreams/pb/sf/substreams/intern/v2"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/pipeline"
	"github.com/streamingfast/substreams/pipeline/cache"
	"github.com/streamingfast/substreams/pipeline/distributor"
	"github.com/streamingfast/substreams/pipeline/exec"
	"github.com/streamingfast/substreams/reqctx"
	"github.com/streamingfast/substreams/service/active_requests"
	"github.com/streamingfast/substreams/storage/execout"
	"github.com/streamingfast/substreams/storage/index"
	"github.com/streamingfast/substreams/storage/store"
	"github.com/streamingfast/substreams/wasm"
	"go.opentelemetry.io/otel/attribute"
	ttrace "go.opentelemetry.io/otel/trace"
	"go.uber.org/atomic"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/anypb"
)

var slowQueryNotificationFrequency = 30 * time.Second
var slowQueryNotificationThreshold = 300 * time.Second
var ErrRequestActiveForTooLong = errors.New("request active for too long")

// sendHostnameHeader mirrors SUBSTREAMS_SEND_HOSTNAME, read once at startup
// instead of on every request (updateStreamHeadersHostname runs per ProcessRange).
var sendHostnameHeader = os.Getenv("SUBSTREAMS_SEND_HOSTNAME") == "true"

type ModuleExecutionConfig struct {
	name       string
	moduleHash string
	objStore   dstore.Store

	skipExecution bool
	cachedOutputs map[string][]byte // ??
	blockFilter   *BlockFilter

	modKind            pbsubstreams.ModuleKind
	moduleInitialBlock uint64

	logger *zap.Logger
}

type BlockFilter struct {
	preexistingExecOuts map[uint64]struct{}
}

type Tier2Service struct {
	*shutter.Shutter
	wasmExtensions func(map[string]string) (map[string]map[string]wasm.WASMExtension, error) //todo: rename
	tracer         ttrace.Tracer
	logger         *zap.Logger

	streamFactoryFuncOverride StreamFactoryFunc

	// You can call this function to switch the parent app to be ready or not ready influencing the health check,
	// it's provided by [app.Tier1App] and tied to the health check endpoint.
	appSetIsReadyState         func(isReady bool)
	currentConcurrentRequests  int64
	maxConcurrentRequests      uint64
	moduleExecutionTracing     bool
	connectionCountMutex       sync.RWMutex
	blockExecutionTimeout      time.Duration
	segmentExecutionTimeout    time.Duration
	foundationalEndpoints      map[string]string
	HostedStoreRegistryAddress string

	checkPendingShutdown func() bool

	tier2RequestParameters *reqctx.Tier2RequestParameters

	simulateOverloaded *atomic.Bool                           // when true, the service will pretent that it is overloaded and refuse incoming requests
	activeRequests     *active_requests.ActiveRequestsManager // we keep a list of current requests for the debugAPI and to manage memory
}

const protoPkfPrefix = "type.googleapis.com/"

func NewTier2(
	ctx context.Context,
	logger *zap.Logger,
	checkPendingShutdown func() bool,
	opts ...Option,
) (*Tier2Service, error) {

	s := &Tier2Service{
		Shutter:                 shutter.New(),
		checkPendingShutdown:    checkPendingShutdown,
		tracer:                  tracing.GetTracer(),
		logger:                  logger,
		blockExecutionTimeout:   3 * time.Minute,
		segmentExecutionTimeout: 60 * time.Minute,

		simulateOverloaded: atomic.NewBool(false),

		activeRequests: active_requests.NewActiveRequestsManager(logger),
	}

	setSubstreamsStoreSizeLimitFromEnv(logger)

	if debugAPIAddress := os.Getenv("SUBSTREAMS_TIER2_DEBUG_API_ADDR"); debugAPIAddress != "" {
		debugAPI := debugapi.New(
			debugAPIAddress,
			logger,
			func(v bool) {
				s.simulateOverloaded.Store(v)
				s.appSetIsReadyState(!v)
			},
			func() bool {
				return s.simulateOverloaded.Load()
			},
			s.listActiveRecords,
			s.cancelRequest,
		)
		debugAPI.Start()
	}

	for _, opt := range opts {
		opt(s)
	}

	metrics.Tier2MaxConcurrentRequests.SetFloat64(float64(s.maxConcurrentRequests))

	return s, nil
}

func (s *Tier2Service) isOverloaded() bool {
	s.connectionCountMutex.RLock()
	defer s.connectionCountMutex.RUnlock()
	if s.simulateOverloaded.Load() {
		return true
	}

	isOverloaded := s.maxConcurrentRequests > 0 && uint64(s.currentConcurrentRequests) >= s.maxConcurrentRequests
	return isOverloaded
}

func (s *Tier2Service) listActiveRecords() string {
	b, err := json.Marshal(s.activeRequests.List())
	if err != nil {
		return err.Error()
	}
	return string(b)
}

func (s *Tier2Service) cancelRequest(traceID string, outputModuleHash string, segmentNumber, segmentSize *uint64, stage *uint32) []string {
	return s.activeRequests.CancelRequest(
		traceID,
		outputModuleHash,
		segmentNumber,
		segmentSize,
		stage,
	)
}

func (s *Tier2Service) incrementConcurrentRequests() {
	s.connectionCountMutex.Lock()
	defer s.connectionCountMutex.Unlock()

	s.currentConcurrentRequests++
	s.setOverloaded()
}

func (s *Tier2Service) decrementConcurrentRequests() {
	s.connectionCountMutex.Lock()
	defer s.connectionCountMutex.Unlock()

	s.currentConcurrentRequests--
	s.setOverloaded()
}

func (s *Tier2Service) setOverloaded() {
	overloaded := s.maxConcurrentRequests != 0 && uint64(s.currentConcurrentRequests) >= s.maxConcurrentRequests
	s.appSetIsReadyState(!overloaded)
}

func (s *Tier2Service) ProcessRange(request *pbssinternal.ProcessRangeRequest, streamSrv pbssinternal.Substreams_ProcessRangeServer) (serverErr error) {
	metrics.Tier2ActiveRequests.Inc()
	metrics.Tier2RequestCounter.Inc()
	defer func() {
		metrics.Tier2ActiveRequests.Dec()
		if reason, countAsRejected := metrics.IsRejectedRequestError(serverErr); countAsRejected {
			metrics.Tier2RejectedRequestCounter.Inc(reason)
		}
	}()

	// We keep `err` here as the unaltered error from `blocks` call, this is used in the EndSpan to record the full error
	// and not only the `grpcError` one which is a subset view of the full `err`.
	var err error
	ctx := streamSrv.Context()

	if request.EthCallFallbackToLatestDuration > 0 {
		ctx = reqctx.WithEthCallFallbackToLatestDuration(ctx, time.Duration(request.EthCallFallbackToLatestDuration))
	}
	if request.EthCallFallbackToNumberDuration > 0 {
		ctx = reqctx.WithEthCallUseBlockNumberDuration(ctx, time.Duration(request.EthCallFallbackToNumberDuration))
	}
	if request.StoreSizeLimit != 0 {
		ctx = reqctx.WithStoreSizeLimit(ctx, request.StoreSizeLimit)
	}

	stage := request.OutputModule
	logger := reqctx.Logger(ctx).Named("tier2").With(zap.String("output_module", stage), zap.Uint64("segment_number", request.SegmentNumber))

	fields := []zap.Field{
		zap.Uint64("segment_size", request.SegmentSize),
		zap.Uint32("stage", request.Stage),
		zap.String("output_module", request.OutputModule),
		zap.Uint64("first_streamable_block", request.FirstStreamableBlock),
		zap.String("metering_config", request.MeteringConfig),
		zap.Bool("stream_output", request.StreamOutput),
	}

	if auth := dauth.FromContext(ctx); auth != nil {
		fields = append(fields,
			zap.String("user_id", auth.UserID()),
			zap.String("key_id", auth.APIKeyID()),
			zap.String("ip_address", auth.RealIP()),
		)
		if cacheTag := auth.Get(reqctx.HeaderCacheTag); cacheTag != "" {
			fields = append(fields,
				zap.String("cache_tag", cacheTag),
			)
		}
	} else {
		logger.Warn("no auth information available")
		fields = append(fields,
			zap.String("user_id", ""),
			zap.String("key_id", ""),
			zap.String("ip_address", ""),
		)
	}

	if s.isOverloaded() {
		err := status.Error(codes.ResourceExhausted, "service currently overloaded")
		fields = append(fields, zap.Error(err))
		logger.Info("refusing Substreams ProcessRange request", fields...)
		return err
	}

	s.incrementConcurrentRequests()
	defer func() {
		s.decrementConcurrentRequests()
	}()

	ctx = logging.WithLogger(ctx, logger)
	ctx = dmetering.WithBytesMeter(ctx)
	ctx = reqctx.WithTracer(ctx, s.tracer)
	ctx = metering.WithMetricsSender(ctx)

	ctx, span := reqctx.WithSpan(ctx, "substreams/tier2/request")
	defer span.EndWithErr(&err)
	span.SetAttributes(attribute.Int64("substreams.tier", 2))

	hostname := updateStreamHeadersHostname(streamSrv.SetHeader, logger)
	span.SetAttributes(attribute.String("hostname", hostname))

	if request.Modules == nil {
		err := status.Error(codes.InvalidArgument, "missing modules in request")
		fields = append(fields, zap.Error(err))
		logger.Info("refusing Substreams ProcessRange request", fields...)
	}
	moduleNames := make([]string, len(request.Modules.Modules))
	for i := 0; i < len(moduleNames); i++ {
		moduleNames[i] = request.Modules.Modules[i].Name
	}
	fields = append(fields, zap.Strings("modules", moduleNames))

	if err := ValidateTier2Request(request); err != nil {
		err = status.Errorf(codes.InvalidArgument, "validate request: %s", err)
		fields = append(fields, zap.Error(err))
		logger.Info("refusing Substreams ProcessRange request", fields...)
		return err
	}

	execGraph, err := exec.NewOutputModuleGraph(request.OutputModule, true, request.Modules, request.FirstStreamableBlock) //production-mode flag is irrelevant here because it isn't used to calculate the hashes
	if err != nil {
		err = bsstream.NewErrInvalidArg("%s", err.Error())
		fields = append(fields, zap.Error(err))
		logger.Info("refusing Substreams ProcessRange request", fields...)
		return err
	}
	outputModuleHash := execGraph.ModuleHashes()[request.OutputModule]

	logger.Info("incoming substreams ProcessRange request", fields...)

	var reqStats *metrics.Stats
	ctx, reqStats = setupRequestStats(ctx, request.OutputModule, outputModuleHash, execGraph, true, true)
	defer reqStats.LogAndClose(ctx, request.StartBlock())

	ctx = reqctx.WithOutputModuleHash(ctx, outputModuleHash)

	emitter, err := dmetering.New(request.MeteringConfig, logger)
	if err != nil {
		reqStats.SetError(err)
		return status.Errorf(codes.Internal, "unable to initialize dmetering: %s", err)
	}

	ctx, cancel := context.WithCancelCause(ctx)
	traceID := tracing.GetTraceID(ctx).String()
	activeReqHandler := s.activeRequests.Add(
		cancel,
		traceID,
		outputModuleHash,
		request.SegmentNumber,
		request.SegmentSize,
		request.Stage,
	)

	defer func() {
		emitter.Shutdown(nil)
		s.activeRequests.Remove(activeReqHandler)
		cancel(nil)
	}()

	ctx = reqctx.WithEmitter(ctx, emitter)
	ctx = reqctx.WithActiveRequestsHandler(ctx, activeReqHandler)

	respFunc := tier2ResponseHandler(ctx, logger, streamSrv)
	err = s.processRange(ctx, request, respFunc)
	grpcError := toGRPCError(ctx, err)

	switch status.Code(grpcError) {
	case codes.Unknown, codes.Internal, codes.Unavailable:
		logger.Info("unexpected termination of stream of blocks", zap.Error(err))
	}

	return grpcError
}

func (s *Tier2Service) getWASMRegistry(wasmExtensionConfigs map[string]string) (*wasm.Registry, error) {
	var exts map[string]map[string]wasm.WASMExtension
	if s.wasmExtensions != nil {
		x, err := s.wasmExtensions(wasmExtensionConfigs) // sets eth_call extensions to wasm machine, ex., for ethereum
		if err != nil {
			return nil, fmt.Errorf("loading wasm extensions: %w", err)
		}
		exts = x
	}
	return wasm.NewRegistry(exts), nil
}

func (s *Tier2Service) processRange(ctx context.Context, request *pbssinternal.ProcessRangeRequest, respFunc substreams.ResponseFunc) error {
	logger := reqctx.Logger(ctx)
	start := time.Now()
	defer func() {
		logger.Info("processRange completed", zapx.HumanDuration("duration", time.Since(start)))
	}()

	mergedBlocksStore, cacheStore, unmeteredCacheStore, err := s.getStores(ctx, request)
	if err != nil {
		return err
	}

	execGraph, err := exec.NewOutputModuleGraph(request.OutputModule, true, request.Modules, request.FirstStreamableBlock)
	if err != nil {
		return stream.NewErrInvalidArg("%s", err.Error())
	}

	requestDetails := pipeline.BuildRequestDetailsFromSubrequest(ctx, request)
	ctx = reqctx.WithRequest(ctx, requestDetails)
	if s.moduleExecutionTracing {
		ctx = reqctx.WithModuleExecutionTracing(ctx)
	}

	wasmRegistry, err := s.getWASMRegistry(request.WasmExtensionConfigs)
	if err != nil {
		return err
	}

	startBlock := request.StartBlock()
	stopBlock := request.StopBlock()

	execOutputConfigs, err := execout.NewConfigs(
		cacheStore,
		execGraph.UsedModulesUpToStage(int(request.Stage)),
		execGraph.ModuleHashes(),
		request.SegmentSize,
		request.FirstStreamableBlock,
		logger)
	if err != nil {
		return fmt.Errorf("new config map: %w", err)
	}

	storeSizeLimit := reqctx.StoreSizeLimit(ctx)
	storeConfigs, err := store.NewConfigMap(cacheStore, nil, execGraph.Stores(), execGraph.ModuleHashes(), request.FirstStreamableBlock, storeSizeLimit)
	if err != nil {
		return fmt.Errorf("configuring stores: %w", err)
	}

	// indexes are not metered: we want users to use them as much as possible
	indexConfigs, err := index.NewConfigs(unmeteredCacheStore, execGraph.UsedIndexesModulesUpToStage(int(request.Stage)), execGraph.ModuleHashes(), request.FirstStreamableBlock, logger)
	if err != nil {
		return fmt.Errorf("configuring indexes: %w", err)
	}

	executionPlan, err := GetExecutionPlan(ctx, logger, execGraph, request.Stage, startBlock, stopBlock, request.OutputModule, execOutputConfigs, indexConfigs, storeConfigs)
	if err != nil {
		return fmt.Errorf("creating execution plan: %w", err)
	}

	if executionPlan.Skippable && request.StreamOutput {
		logger.Info("no modules required to run, skipping, but sending outputs anyway because StreamOutput is set")
		module := execGraph.OutputModule()
		execOuts := executionPlan.ExistingExecOuts[request.OutputModule]
		for cacheItem, err := range execOuts.Iter() {
			if err != nil {
				return fmt.Errorf("error iterating existing execouts: %w", err)
			}

			clock := &pbsubstreams.Clock{
				Id:        cacheItem.BlockId,
				Number:    cacheItem.BlockNum,
				Timestamp: cacheItem.Timestamp,
			}
			outputType := strings.TrimPrefix(module.Output.Type, "proto:")
			out := &anypb.Any{TypeUrl: "type.googleapis.com/" + outputType, Value: cacheItem.Payload}

			if err := respFunc(substreams.NewBlockScopedDataInternResponse(&pbssinternal.BlockScopedData{
				Output: out,
				Clock:  clock,
			})); err != nil {
				return fmt.Errorf("error sending response: %w", err)
			}
		}
		return nil
	}

	if executionPlan == nil || len(executionPlan.RequiredModules) == 0 {
		logger.Info("no modules required to run, skipping")
		return nil
	}
	stores := pipeline.NewStores(ctx, storeConfigs, request.SegmentSize, requestDetails.ResolvedStartBlockNum, stopBlock, true, executionPlan.StoresToWrite)

	// this engine will keep the ExistingExecOuts to optimize the execution (for inputs from modules that skip execution)
	execOutputCacheEngine, err := cache.NewEngine(ctx, executionPlan.ExecoutWriters, request.BlockType, executionPlan.ExistingExecOuts, executionPlan.IndexWriters)
	if err != nil {
		return fmt.Errorf("error building caching engine: %w", err)
	}

	ctx, cancelCause := context.WithCancelCause(ctx)
	defer cancelCause(nil)

	now := time.Now()
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-time.After(slowQueryNotificationFrequency):
				if s.segmentExecutionTimeout != 0 && time.Since(now) > s.segmentExecutionTimeout {
					cancelCause(connect.NewError(connect.CodeDeadlineExceeded, ErrRequestActiveForTooLong))
				}
				if time.Since(now) > slowQueryNotificationThreshold {

					modStats := reqctx.ReqStats(ctx).AggregatedModulesStats()
					modStrings := make([]string, len(modStats))
					for i, mod := range modStats {
						var storeSizeStr string
						if mod.StoreSizeBytes != 0 {
							storeSizeStr = fmt.Sprintf(" store_size_bytes:%d,", mod.StoreSizeBytes)
						}
						var externalCallMetricsStr string
						if mod.ExternalCallMetrics != nil {
							externalCallMetricsStr = "{"
							for i, metric := range mod.ExternalCallMetrics {
								if i > 0 {
									externalCallMetricsStr += ", "
								}
								externalCallMetricsStr += fmt.Sprintf(`"%s": {"count": %d, "duration_ms": %d}`, metric.Name, metric.Count, metric.TimeMs)
							}
							externalCallMetricsStr += "}"
						}

						modStrings[i] = fmt.Sprintf("%s: processing_time_ms:%d,%s %s", mod.Name, mod.TotalProcessingTimeMs, storeSizeStr, externalCallMetricsStr)
					}

					userID := "UNKNOWN"
					if auth := dauth.FromContext(ctx); auth != nil {
						userID = fmt.Sprintf("user_id:%s,", auth.UserID())
					}
					sort.Strings(modStrings)
					logger.Info("request active for a long time", zap.Duration("duration", time.Since(now)), zap.Strings("modules", modStrings), zap.Uint64("processed_blocks", reqctx.ReqStats(ctx).GetBlocksProcessed()), zap.String("user_id", userID))
				}
			}
		}
	}()

	//opts := s.buildPipelineOptions(ctx, request)
	var opts []pipeline.Option
	opts = append(opts, pipeline.WithFinalBlocksOnly())
	opts = append(opts, pipeline.WithHighestStage(request.Stage))

	pipe := pipeline.New(
		ctx,
		false,
		execGraph,
		request.BlockType,
		stores,
		executionPlan.ExistingIndices,
		execOutputConfigs,
		wasmRegistry,
		execOutputCacheEngine,
		request.SegmentSize,
		nil,
		respFunc,
		s.blockExecutionTimeout,
		s.checkPendingShutdown,
		request.FoundationalStoreEndpoints,
		0,
		false,
		s.HostedStoreRegistryAddress,
		opts...,
	)

	logger.Debug("initializing tier2 pipeline",
		zap.Uint64("request_start_block", requestDetails.ResolvedStartBlockNum),
		zap.String("output_module", request.OutputModule),
		zap.Uint32("stage", request.Stage),
	)

	if err := pipe.Init(ctx); err != nil {
		return fmt.Errorf("error during pipeline init: %w", err)
	}

	if err := pipe.InitTier2Stores(ctx); err != nil {
		return fmt.Errorf("error building pipeline: %w", err)
	}

	if err := pipe.BuildModuleExecutors(ctx); err != nil {
		return fmt.Errorf("error building module executors: %w", err)
	}

	allExecutorsExcludedByBlockIndex := true
excludable:
	for _, stage := range pipe.StagedModuleExecutors {
		for _, executor := range stage {
			switch executor := executor.(type) {
			case *exec.MapperModuleExecutor:
				if executionPlan.ExistingExecOuts[executor.Name()] != nil {
					continue
				}
			case *exec.IndexModuleExecutor:
				if executionPlan.ExistingIndices[executor.Name()] != nil {
					continue
				}
			case *exec.StoreModuleExecutor:
				if executionPlan.ExistingExecOuts[executor.Name()] != nil {
					if _, ok := executionPlan.StoresToWrite[executor.Name()]; !ok {
						continue
					}
				}
			}
			if !executor.BlockIndex().ExcludesAllBlocks() {
				allExecutorsExcludedByBlockIndex = false
				break excludable
			}
		}
	}
	if allExecutorsExcludedByBlockIndex {
		logger.Info("all executors are excluded by block index. Skipping execution of segment")
		return pipe.OnStreamTerminated(ctx, io.EOF)
	}

	var streamErr error
	if canSkipBlockSource(executionPlan.ExistingExecOuts, executionPlan.RequiredModules, request.BlockType) {

		ctx, span := reqctx.WithSpan(ctx, "substreams/tier2/pipeline/mapper_stream")

		outputModuleInputs := execGraph.OutputModule().Inputs
		dist := distributor.NewClockDistributor(
			executionPlan.ExistingExecOuts,
			startBlock,
			stopBlock,
			outputModuleInputs,
			pipe.ModuleBlockIndexes,
		)

		for clock, err := range dist.Iter(ctx) {
			if err != nil {
				span.EndWithErr(&err)
				return pipe.OnStreamTerminated(ctx, err)
			}
			cursor := irreversibleCursorFromClock(clock)

			if err := pipe.ProcessFromExecOutput(ctx, clock, cursor); err != nil {
				span.EndWithErr(&err)
				return pipe.OnStreamTerminated(ctx, err)
			}
		}
		streamErr = io.EOF
		span.EndWithErr(&streamErr)
		return pipe.OnStreamTerminated(ctx, streamErr)
	}
	sf := &StreamFactory{
		mergedBlocksStore: mergedBlocksStore,
	}
	streamFactoryFunc := sf.New

	if s.streamFactoryFuncOverride != nil { //this is only for testing purposes.
		streamFactoryFunc = s.streamFactoryFuncOverride
	}

	blockStream, err := streamFactoryFunc(
		ctx,
		pipe,
		int64(requestDetails.ResolvedStartBlockNum),
		stopBlock,
		"",
		true,
		false, // includePartialBlocks should be false in tier2
		false, // cursorIsTarget
		logger.Named("stream"),
		bsstream.WithFileSourceHandlerMiddleware(metering.FileSourceMiddlewareHandlerFactory(ctx)),
	)
	if err != nil {
		return pipe.OnStreamTerminated(ctx, fmt.Errorf("error getting stream: %w", err))
	}

	ctx, span := reqctx.WithSpan(ctx, "substreams/tier2/pipeline/blocks_stream")
	streamErr = blockStream.Run(ctx)
	span.EndWithErr(&streamErr)

	if errors.Is(context.Canceled, streamErr) {
		streamErr = context.Cause(ctx)
	}

	return pipe.OnStreamTerminated(ctx, streamErr)
}

func (s *Tier2Service) getStores(ctx context.Context, request *pbssinternal.ProcessRangeRequest) (mergedBlocksStore, cacheStore, unmeteredCacheStore dstore.Store, err error) {
	mergedBlocksStore, err = dstore.NewDBinStore(request.MergedBlocksStore)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("setting up block store from url %q: %w", request.MergedBlocksStore, err)
	}

	stateStore, err := dstore.NewStore(request.StateStore, "zst", "zstd", false)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("getting store: %w", err)
	}

	cacheTag := request.StateStoreDefaultTag
	if auth := dauth.FromContext(ctx); auth != nil {
		if ct := auth.Get(reqctx.HeaderCacheTag); ct != "" {
			if IsValidCacheTag(ct) {
				cacheTag = ct
			} else {
				return nil, nil, nil, fmt.Errorf("invalid value for %s: %q, should only contain letters, numbers, hyphens and underscores", reqctx.HeaderCacheTag, ct)
			}
		}
	}

	unmeteredCacheStore, err = stateStore.SubStore(cacheTag)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("internal error setting store: %w", err)
	}

	if clonableStore, ok := unmeteredCacheStore.(dstore.Clonable); ok {
		cloned, err := clonableStore.Clone(ctx, metering.WithBytesMeteringOptions(dmetering.GetBytesMeter(ctx), reqctx.Logger(ctx))...)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("cloning store: %w", err)
		}
		//todo: (deprecated)
		cloned.SetMeter(dmetering.GetBytesMeter(ctx))
		cacheStore = cloned
	}

	return
}

func canSkipBlockSource(existingExecOuts map[string]execout.FileReader, requiredModules map[string]*pbsubstreams.Module, blockType string) bool {
	if len(existingExecOuts) == 0 {
		return false
	}
	for name, module := range requiredModules {
		if existingExecOuts[name] != nil {
			continue
		}
		for _, input := range module.Inputs {
			if src := input.GetSource(); src != nil && src.Type == blockType {
				return false
			}
		}
	}
	return true
}

func tier2ResponseHandler(ctx context.Context, logger *zap.Logger, streamSrv pbssinternal.Substreams_ProcessRangeServer) substreams.ResponseFunc {
	var userID, apiKeyID, ip string
	if auth := dauth.FromContext(ctx); auth != nil {
		userID = auth.UserID()
		apiKeyID = auth.APIKeyID()
		ip = auth.RealIP()
		logger.Info("auth information available in tier2 response handler", zap.String("user_id", userID), zap.String("key_id", apiKeyID), zap.String("ip_address", ip))
	} else {
		logger.Warn("no auth information available in tier2 response handler")
	}

	return func(respAny substreams.ResponseFromAnyTier) error {
		resp := respAny.(*pbssinternal.ProcessRangeResponse)
		if err := streamSrv.Send(resp); err != nil {
			logger.Info("unable to send block probably due to client disconnecting", zap.Error(err), zap.String("user_id", userID), zap.String("key_id", apiKeyID), zap.Error(err))
			return status.Error(codes.Unavailable, err.Error())
		}

		// note: we don't increase EgressBytes on tier2 responses, these bytes are only sent to tier1
		return nil
	}
}

func updateStreamHeadersHostname(setHeader func(metadata.MD) error, logger *zap.Logger) string {
	hostname, err := os.Hostname()
	if err != nil {
		logger.Warn("cannot find hostname, using 'unknown'", zap.Error(err))
		hostname = "unknown host"
	}
	if sendHostnameHeader {
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
		case connect.CodeDeadlineExceeded:
			return status.Error(codes.DeadlineExceeded, err.Error())
		}
	}

	if errors.Is(err, context.Canceled) {
		if context.Cause(ctx) != nil {
			err = context.Cause(ctx)
			if err == errShuttingDown {
				return status.Error(codes.Unavailable, err.Error())
			}
			return toGRPCError(context.TODO(), err) // get error parsing from above for the error encapsulated in the cause
		}
		return status.Error(codes.Canceled, err.Error())
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return status.Error(codes.DeadlineExceeded, err.Error())
	}
	if errors.Is(err, wasm.ErrWasmDeterministicExec) || errors.Is(err, store.ErrStoreAboveMaxSize) {
		return status.Error(codes.InvalidArgument, fmt.Sprintf("%s (deterministic error)", err.Error()))
	}

	var errInvalidArg *stream.ErrInvalidArg
	if errors.As(err, &errInvalidArg) {
		return status.Error(codes.InvalidArgument, err.Error())
	}
	return status.Error(codes.Internal, err.Error())
}

type ExecutionPlan struct {
	ExistingExecOuts map[string]execout.FileReader
	ExecoutWriters   map[string]*execout.Writer
	ExistingIndices  map[string]map[string]*roaring64.Bitmap
	IndexWriters     map[string]*index.Writer
	RequiredModules  map[string]*pbsubstreams.Module
	StoresToWrite    map[string]struct{}
	Skippable        bool
}

func GetExecutionPlan(
	ctx context.Context,
	logger *zap.Logger,
	execGraph *exec.Graph,
	stage uint32,
	startBlock uint64,
	stopBlock uint64,
	outputModule string,
	execoutConfigs *execout.Configs,
	indexConfigs *index.Configs,
	storeConfigs store.ConfigMap,
) (plan *ExecutionPlan, err error) {
	storesToWrite := make(map[string]struct{})
	existingExecOuts := make(map[string]execout.FileReader)
	existingIndices := make(map[string]map[string]*roaring64.Bitmap)
	requiredModules := make(map[string]*pbsubstreams.Module)
	execoutWriters := make(map[string]*execout.Writer) // this affects stores and mappers, per-block data
	indexWriters := make(map[string]*index.Writer)     // write the full index file
	// storeWriters := .... // write the snapshots
	usedModules := make(map[string]*pbsubstreams.Module)
	for _, module := range execGraph.UsedModulesUpToStage(int(stage)) {
		usedModules[module.Name] = module
	}

	stageUsedModules := execGraph.StagedUsedModules()[stage]
	runningLastStage := stageUsedModules.IsLastStage()
	stageUsedModulesName := make(map[string]bool)
	for _, layer := range stageUsedModules {
		for _, mod := range layer {
			stageUsedModulesName[mod.Name] = true
		}
	}

	go func() {
		<-ctx.Done()
		for _, eo := range existingExecOuts {
			if err := eo.Close(); err != nil {
				logger.Info("error closing reader", zap.String("filename", eo.Filename()), zap.Error(err))
			}
		}
		// Writers don't need to be closed, canceling the context is enough
	}()

	for _, mod := range usedModules {
		if mod.InitialBlock >= stopBlock {
			continue
		}

		name := mod.Name

		c := execoutConfigs.ConfigMap[name]

		moduleStartBlock := startBlock
		if mod.InitialBlock > startBlock {
			moduleStartBlock = mod.InitialBlock
		}

		switch mod.ModuleKind() {
		case pbsubstreams.ModuleKindBlockIndex:
			indexFile := indexConfigs.ConfigMap[name].NewFile(&block.Range{StartBlock: moduleStartBlock, ExclusiveEndBlock: stopBlock})
			err := indexFile.Load(ctx)
			if err != nil {
				if !errors.Is(err, dstore.ErrNotFound) {
					return nil, fmt.Errorf("reading mapper output file: %w", err)
				}
				requiredModules[name] = usedModules[name]
				indexWriters[name] = index.NewWriter(indexFile)
				break
			}

			existingIndices[name] = indexFile.Indices

		case pbsubstreams.ModuleKindMap:
			file, readErr := c.OpenFileReader(ctx, &block.Range{StartBlock: moduleStartBlock, ExclusiveEndBlock: stopBlock})
			if readErr != nil {
				if !errors.Is(readErr, dstore.ErrNotFound) {
					return nil, fmt.Errorf("reading mapper output file: %w", readErr)
				}
				requiredModules[name] = usedModules[name]
				break
			}
			existingExecOuts[name] = file

		case pbsubstreams.ModuleKindStore:
			file, readErr := c.OpenFileReader(ctx, &block.Range{StartBlock: moduleStartBlock, ExclusiveEndBlock: stopBlock})
			if readErr != nil {
				if !errors.Is(readErr, dstore.ErrNotFound) {
					return nil, fmt.Errorf("reading mapper output file: %w", readErr)
				}
				requiredModules[name] = usedModules[name]
			} else {
				existingExecOuts[name] = file
			}

			// if either full or partial kv exists, we can skip the module
			// some stores may already exist completely on this stage, but others do not, so we keep going but ignore those
			storeExists, err := storeConfigs[name].ExistsFullKV(ctx, stopBlock)
			if err != nil {
				return nil, fmt.Errorf("checking fullkv file existence: %w", err)
			}
			if !storeExists {
				partialStoreExists, err := storeConfigs[name].ExistsPartialKV(ctx, moduleStartBlock, stopBlock)
				if err != nil {
					return nil, fmt.Errorf("checking partial file existence: %w", err)
				}
				// when running last stage, we want to make sure that we write all the fullKV stores.
				// in other scenarios, a partial store is enough so we won't produce it again if it's not needed.
				if !partialStoreExists || runningLastStage {
					storesToWrite[name] = struct{}{}
					requiredModules[name] = usedModules[name]
				}
			}

		default:
			return nil, fmt.Errorf("invalid module type: %s", name)
		}

	}

	if runningLastStage && len(storesToWrite) == 0 && existingExecOuts[outputModule] != nil {
		logger.Info("found existing exec output for output_module and no stores to produce, skipping run", zap.String("output_module", outputModule))

		return &ExecutionPlan{
			ExistingExecOuts: existingExecOuts,
			ExecoutWriters:   nil,
			ExistingIndices:  nil,
			IndexWriters:     nil,
			RequiredModules:  nil,
			StoresToWrite:    nil,
			Skippable:        true,
		}, nil
	}

	for name, module := range requiredModules {
		if _, exists := existingExecOuts[name]; exists {
			continue // for stores that need to be run for the partials, but already have cached execution outputs
		}

		writerStartBlock := startBlock
		if module.InitialBlock > startBlock {
			writerStartBlock = module.InitialBlock
		}

		switch module.ModuleKind() {
		case pbsubstreams.ModuleKindBlockIndex:
			file := indexConfigs.ConfigMap[name].NewFile(&block.Range{StartBlock: writerStartBlock, ExclusiveEndBlock: stopBlock})
			indexWriters[name] = index.NewWriter(file)
		case pbsubstreams.ModuleKindStore, pbsubstreams.ModuleKindMap:
			execoutWriters[name] = execout.NewWriter(
				ctx,
				writerStartBlock,
				stopBlock,
				name,
				execoutConfigs,
			)
		default:
			return nil, fmt.Errorf("invalid module type: %s", name)
		}

	}

	return &ExecutionPlan{
		ExistingExecOuts: existingExecOuts,
		ExecoutWriters:   execoutWriters,
		ExistingIndices:  existingIndices,
		IndexWriters:     indexWriters,
		RequiredModules:  requiredModules,
		StoresToWrite:    storesToWrite,
	}, nil
}

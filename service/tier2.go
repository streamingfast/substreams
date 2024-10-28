package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/streamingfast/bstream"

	"connectrpc.com/connect"
	"github.com/RoaringBitmap/roaring/roaring64"
	"github.com/streamingfast/bstream/stream"
	bsstream "github.com/streamingfast/bstream/stream"
	"github.com/streamingfast/dauth"
	"github.com/streamingfast/dgrpc"
	"github.com/streamingfast/dmetering"
	"github.com/streamingfast/dstore"
	"github.com/streamingfast/logging"
	tracing "github.com/streamingfast/sf-tracing"
	"github.com/streamingfast/substreams"
	"github.com/streamingfast/substreams/block"
	"github.com/streamingfast/substreams/metering"
	"github.com/streamingfast/substreams/metrics"
	pbssinternal "github.com/streamingfast/substreams/pb/sf/substreams/intern/v2"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/pipeline"
	"github.com/streamingfast/substreams/pipeline/cache"
	"github.com/streamingfast/substreams/pipeline/exec"
	"github.com/streamingfast/substreams/reqctx"
	"github.com/streamingfast/substreams/storage/execout"
	"github.com/streamingfast/substreams/storage/index"
	"github.com/streamingfast/substreams/storage/store"
	"github.com/streamingfast/substreams/wasm"
	"go.opentelemetry.io/otel/attribute"
	ttrace "go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

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
	wasmExtensions func(map[string]string) (map[string]map[string]wasm.WASMExtension, error) //todo: rename
	tracer         ttrace.Tracer
	logger         *zap.Logger

	streamFactoryFuncOverride StreamFactoryFunc

	setReadyFunc              func(bool)
	currentConcurrentRequests int64
	maxConcurrentRequests     uint64
	moduleExecutionTracing    bool
	connectionCountMutex      sync.RWMutex
	blockExecutionTimeout     time.Duration

	tier2RequestParameters *reqctx.Tier2RequestParameters
}

const protoPkfPrefix = "type.googleapis.com/"

func NewTier2(
	logger *zap.Logger,
	opts ...Option,
) (*Tier2Service, error) {

	s := &Tier2Service{
		tracer:                tracing.GetTracer(),
		logger:                logger,
		blockExecutionTimeout: 3 * time.Minute,
	}

	metrics.RegisterMetricSet(logger)

	for _, opt := range opts {
		opt(s)
	}

	return s, nil
}

func (s *Tier2Service) isOverloaded() bool {
	s.connectionCountMutex.RLock()
	defer s.connectionCountMutex.RUnlock()

	isOverloaded := s.maxConcurrentRequests > 0 && uint64(s.currentConcurrentRequests) >= s.maxConcurrentRequests
	return isOverloaded
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
	s.setReadyFunc(!overloaded)
}

func (s *Tier2Service) ProcessRange(request *pbssinternal.ProcessRangeRequest, streamSrv pbssinternal.Substreams_ProcessRangeServer) error {
	metrics.Tier2ActiveRequests.Inc()
	metrics.Tier2RequestCounter.Inc()
	defer metrics.Tier2ActiveRequests.Dec()

	// We keep `err` here as the unaltered error from `blocks` call, this is used in the EndSpan to record the full error
	// and not only the `grpcError` one which is a subset view of the full `err`.
	var err error
	ctx := streamSrv.Context()

	if s.isOverloaded() {
		return connect.NewError(connect.CodeUnavailable, fmt.Errorf("service currently overloaded"))
	}

	s.incrementConcurrentRequests()
	defer func() {
		s.decrementConcurrentRequests()
	}()

	stage := request.OutputModule

	logger := reqctx.Logger(ctx).Named("tier2").With(zap.String("output_module", stage), zap.Uint64("segment_number", request.SegmentNumber))

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
		return connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("missing modules in request"))
	}
	moduleNames := make([]string, len(request.Modules.Modules))
	for i := 0; i < len(moduleNames); i++ {
		moduleNames[i] = request.Modules.Modules[i].Name
	}

	fields := []zap.Field{
		zap.Uint64("segment_size", request.SegmentSize),
		zap.Uint32("stage", request.Stage),
		zap.Strings("modules", moduleNames),
		zap.String("output_module", request.OutputModule),
		zap.Uint64("first_streamable_block", request.FirstStreamableBlock),
		zap.String("metering_config", request.MeteringConfig),
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
	} else {
		logger.Warn("no auth information available")
		fields = append(fields,
			zap.String("user_id", ""),
			zap.String("key_id", ""),
			zap.String("ip_address", ""),
		)
	}

	logger.Info("incoming substreams ProcessRange request", fields...)

	if err := ValidateTier2Request(request); err != nil {
		return connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("validate request: %w", err))
	}

	execGraph, err := exec.NewOutputModuleGraph(request.OutputModule, true, request.Modules, bstream.GetProtocolFirstStreamableBlock)
	if err != nil {
		return bsstream.NewErrInvalidArg(err.Error())
	}
	outputModuleHash := execGraph.ModuleHashes().Get(request.OutputModule)
	ctx = reqctx.WithOutputModuleHash(ctx, outputModuleHash)

	emitter, err := dmetering.New(request.MeteringConfig, logger)
	if err != nil {
		return connect.NewError(connect.CodeInternal, fmt.Errorf("unable to initialize dmetering: %w", err))
	}
	defer func() {
		emitter.Shutdown(nil)
	}()

	ctx = reqctx.WithEmitter(ctx, emitter)

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

	mergedBlocksStore, cacheStore, unmeteredCacheStore, err := s.getStores(ctx, request)
	if err != nil {
		return err
	}

	execGraph, err := exec.NewOutputModuleGraph(request.OutputModule, true, request.Modules, request.FirstStreamableBlock)
	if err != nil {
		return stream.NewErrInvalidArg(err.Error())
	}

	requestDetails := pipeline.BuildRequestDetailsFromSubrequest(request)
	ctx = reqctx.WithRequest(ctx, requestDetails)
	if s.moduleExecutionTracing {
		ctx = reqctx.WithModuleExecutionTracing(ctx)
	}

	var requestStats *metrics.Stats
	ctx, requestStats = setupRequestStats(ctx, requestDetails, execGraph.ModuleHashes().Get(requestDetails.OutputModule), true)
	defer requestStats.LogAndClose()

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

	storeConfigs, err := store.NewConfigMap(cacheStore, execGraph.Stores(), execGraph.ModuleHashes(), request.FirstStreamableBlock)
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

	//opts := s.buildPipelineOptions(ctx, request)
	var opts []pipeline.Option
	opts = append(opts, pipeline.WithFinalBlocksOnly())
	opts = append(opts, pipeline.WithHighestStage(request.Stage))

	pipe := pipeline.New(
		ctx,
		execGraph,
		stores,
		executionPlan.ExistingIndices,
		execOutputConfigs,
		wasmRegistry,
		execOutputCacheEngine,
		request.SegmentSize,
		nil,
		respFunc,
		s.blockExecutionTimeout,
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
	for _, stage := range pipe.ModuleExecutors {
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
		maxDistributorLength := int(stopBlock - requestDetails.ResolvedStartBlockNum)
		clocksDistributor := make(map[uint64]*pbsubstreams.Clock)
		for _, execOutput := range executionPlan.ExistingExecOuts {
			execOutput.ExtractClocks(clocksDistributor)
			if len(clocksDistributor) >= maxDistributorLength {
				break
			}
		}

		sortedClocksDistributor := sortClocksDistributor(clocksDistributor)
		ctx, span := reqctx.WithSpan(ctx, "substreams/tier2/pipeline/mapper_stream")
		for _, clock := range sortedClocksDistributor {
			if clock.Number < startBlock || clock.Number >= stopBlock {
				panic("reading from mapper, block was out of range") // we don't want to have this case undetected
			}
			cursor := irreversibleCursorFromClock(clock)

			if err := pipe.ProcessFromExecOutput(ctx, clock, cursor); err != nil {
				span.EndWithErr(&err)
				return err
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
		false,
		logger.Named("stream"),
		bsstream.WithFileSourceHandlerMiddleware(metering.FileSourceMiddlewareHandlerFactory(ctx)),
	)
	if err != nil {
		return fmt.Errorf("error getting stream: %w", err)
	}

	ctx, span := reqctx.WithSpan(ctx, "substreams/tier2/pipeline/blocks_stream")
	streamErr = blockStream.Run(ctx)
	span.EndWithErr(&streamErr)

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
		if ct := auth.Get("X-Sf-Substreams-Cache-Tag"); ct != "" {
			if IsValidCacheTag(ct) {
				cacheTag = ct
			} else {
				return nil, nil, nil, fmt.Errorf("invalid value for X-Sf-Substreams-Cache-Tag %s, should only contain letters, numbers, hyphens and undescores", ct)
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

func canSkipBlockSource(existingExecOuts map[string]*execout.File, requiredModules map[string]*pbsubstreams.Module, blockType string) bool {
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
	var userID, apiKeyID, userMeta, ip string
	if auth := dauth.FromContext(ctx); auth != nil {
		userID = auth.UserID()
		apiKeyID = auth.APIKeyID()
		userMeta = auth.Meta()
		ip = auth.RealIP()
		logger.Info("auth information available in tier2 response handler", zap.String("user_id", userID), zap.String("key_id", apiKeyID), zap.String("ip_address", ip))
	} else {
		logger.Warn("no auth information available in tier2 response handler")
	}

	metricsSender := metering.GetMetricsSender(ctx)

	return func(respAny substreams.ResponseFromAnyTier) error {
		resp := respAny.(*pbssinternal.ProcessRangeResponse)
		if err := streamSrv.Send(resp); err != nil {
			logger.Info("unable to send block probably due to client disconnecting", zap.Error(err))
			return connect.NewError(connect.CodeUnavailable, err)
		}

		logger.Debug("sending metering event",
			zap.String("user_id", userID),
			zap.String("key_id", apiKeyID),
			zap.String("ip_address", ip),
			zap.String("user_meta", userMeta),
			zap.String("endpoint", "sf.substreams.internal.v2/ProcessRange"),
		)
		metricsSender.Send(ctx, userID, apiKeyID, ip, userMeta, "sf.substreams.internal.v2/ProcessRange", resp)
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

type ExecutionPlan struct {
	ExistingExecOuts map[string]*execout.File
	ExecoutWriters   map[string]*execout.Writer
	ExistingIndices  map[string]map[string]*roaring64.Bitmap
	IndexWriters     map[string]*index.Writer
	RequiredModules  map[string]*pbsubstreams.Module
	StoresToWrite    map[string]struct{}
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
) (*ExecutionPlan, error) {
	storesToWrite := make(map[string]struct{})
	existingExecOuts := make(map[string]*execout.File)
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
				requiredModules[name] = usedModules[name]
				indexWriters[name] = index.NewWriter(indexFile)
				break
			}

			existingIndices[name] = indexFile.Indices

		case pbsubstreams.ModuleKindMap:
			file, readErr := c.ReadFile(ctx, &block.Range{StartBlock: moduleStartBlock, ExclusiveEndBlock: stopBlock})
			if readErr != nil {
				requiredModules[name] = usedModules[name]
				break
			}
			existingExecOuts[name] = file

			if runningLastStage && name == outputModule {
				logger.Info("found existing exec output for output_module, skipping run", zap.String("output_module", name))
				return nil, nil
			}

		case pbsubstreams.ModuleKindStore:
			file, readErr := c.ReadFile(ctx, &block.Range{StartBlock: moduleStartBlock, ExclusiveEndBlock: stopBlock})
			if readErr != nil {
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
				if !partialStoreExists {
					storesToWrite[name] = struct{}{}
					requiredModules[name] = usedModules[name]
				}
			}

		}

	}

	for name, module := range requiredModules {
		if _, exists := existingExecOuts[name]; exists {
			continue // for stores that need to be run for the partials, but already have cached execution outputs
		}

		writerStartBlock := startBlock
		if module.InitialBlock > startBlock {
			writerStartBlock = module.InitialBlock
		}

		var isIndexWriter bool
		if module.ModuleKind() == pbsubstreams.ModuleKindBlockIndex {
			isIndexWriter = true
		}

		execoutWriters[name] = execout.NewWriter(
			writerStartBlock,
			stopBlock,
			name,
			execoutConfigs,
			isIndexWriter,
		)

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

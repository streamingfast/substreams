package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"

	"connectrpc.com/connect"
	"github.com/streamingfast/bstream/stream"
	"github.com/streamingfast/dauth"
	"github.com/streamingfast/dgrpc"
	"github.com/streamingfast/dmetering"
	"github.com/streamingfast/dstore"
	"github.com/streamingfast/logging"
	tracing "github.com/streamingfast/sf-tracing"
	"github.com/streamingfast/substreams"
	"github.com/streamingfast/substreams/block"
	"github.com/streamingfast/substreams/metrics"
	pbssinternal "github.com/streamingfast/substreams/pb/sf/substreams/intern/v2"
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
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type Tier2Service struct {
	wasmExtensions func(map[string]string) (map[string]map[string]wasm.WASMExtension, error) //todo: rename
	runtimeConfig  config.RuntimeConfig
	tracer         ttrace.Tracer
	logger         *zap.Logger

	streamFactoryFuncOverride StreamFactoryFunc

	setReadyFunc              func(bool)
	currentConcurrentRequests int64
	connectionCountMutex      sync.RWMutex

	tier2RequestParameters *reqctx.Tier2RequestParameters
}

const protoPkfPrefix = "type.googleapis.com/"

func NewTier2(
	logger *zap.Logger,
	opts ...Option,
) (*Tier2Service, error) {
	runtimeConfig := config.NewTier2RuntimeConfig()

	s := &Tier2Service{
		runtimeConfig: runtimeConfig,
		tracer:        tracing.GetTracer(),
		logger:        logger,
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

	isOverloaded := s.runtimeConfig.MaxConcurrentRequests > 0 && s.currentConcurrentRequests >= s.runtimeConfig.MaxConcurrentRequests
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
	overloaded := s.runtimeConfig.MaxConcurrentRequests != 0 && s.currentConcurrentRequests >= s.runtimeConfig.MaxConcurrentRequests
	s.setReadyFunc(!overloaded)
}

func (s *Tier2Service) ProcessRange(request *pbssinternal.ProcessRangeRequest, streamSrv pbssinternal.Substreams_ProcessRangeServer) (grpcError error) {
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

	switch {
	case request.MeteringConfig == "":
		return fmt.Errorf("metering config is required in request")
	case request.BlockType == "":
		return fmt.Errorf("block type is required in request")
	case request.StateStore == "":
		return fmt.Errorf("state store is required in request")
	case request.MergedBlocksStore == "":
		return fmt.Errorf("merged blocks store is required in request")
	case request.StateBundleSize == 0:
		return fmt.Errorf("a non-zero state bundle size is required in request")
	}

	emitter, err := dmetering.New(request.MeteringConfig, logger)
	if err != nil {
		return connect.NewError(connect.CodeInternal, fmt.Errorf("unable to initialize dmetering: %w", err))
	}
	defer func() {
		emitter.Shutdown(nil)
	}()

	ctx = context.WithValue(ctx, "event_emitter", emitter)

	respFunc := tier2ResponseHandler(ctx, logger, streamSrv)
	err = s.processRange(ctx, request, respFunc)
	grpcError = toGRPCError(ctx, err)

	switch status.Code(grpcError) {
	case codes.Unknown, codes.Internal, codes.Unavailable:
		logger.Info("unexpected termination of stream of blocks", zap.Error(err))
	}

	return grpcError
}

func (s *Tier2Service) processRange(ctx context.Context, request *pbssinternal.ProcessRangeRequest, respFunc substreams.ResponseFunc) error {
	logger := reqctx.Logger(ctx)

	s.runtimeConfig.DefaultCacheTag = request.StateStoreDefaultTag
	s.runtimeConfig.StateBundleSize = request.StateBundleSize

	mergedBlocksStore, err := dstore.NewDBinStore(request.MergedBlocksStore)
	if err != nil {
		return fmt.Errorf("setting up block store from url %q: %w", request.MergedBlocksStore, err)
	}

	if cloned, ok := mergedBlocksStore.(dstore.Clonable); ok {
		mergedBlocksStore, err = cloned.Clone(ctx)
		if err != nil {
			return fmt.Errorf("cloning store: %w", err)
		}
		mergedBlocksStore.SetMeter(dmetering.GetBytesMeter(ctx))
	}

	stateStore, err := dstore.NewStore(request.StateStore, "zst", "zstd", false)
	if cloned, ok := stateStore.(dstore.Clonable); ok {
		stateStore, err = cloned.Clone(ctx)
		if err != nil {
			return fmt.Errorf("cloning store: %w", err)
		}
		stateStore.SetMeter(dmetering.GetBytesMeter(ctx))
	}

	if err := outputmodules.ValidateTier2Request(request); err != nil {
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

	var exts map[string]map[string]wasm.WASMExtension
	if s.wasmExtensions != nil {
		x, err := s.wasmExtensions(request.WasmModules)
		if err != nil {
			return fmt.Errorf("loading wasm extensions: %w", err)
		}
		exts = x
	}
	wasmRuntime := wasm.NewRegistry(exts, s.runtimeConfig.MaxWasmFuel)

	cacheStore, err := stateStore.SubStore(requestDetails.CacheTag)
	if err != nil {
		return fmt.Errorf("internal error setting store: %w", err)
	}

	if clonableStore, ok := cacheStore.(dstore.Clonable); ok {
		cloned, err := clonableStore.Clone(ctx)
		if err != nil {
			return fmt.Errorf("cloning store: %w", err)
		}
		cloned.SetMeter(dmetering.GetBytesMeter(ctx))
		cacheStore = cloned
	}

	execOutputConfigs, err := execout.NewConfigs(cacheStore, outputGraph.UsedModulesUpToStage(int(request.Stage)), outputGraph.ModuleHashes(), request.StateBundleSize, logger)
	if err != nil {
		return fmt.Errorf("new config map: %w", err)
	}

	storeConfigs, err := store.NewConfigMap(cacheStore, outputGraph.Stores(), outputGraph.ModuleHashes())
	if err != nil {
		return fmt.Errorf("configuring stores: %w", err)
	}
	stores := pipeline.NewStores(ctx, storeConfigs, request.StateBundleSize, requestDetails.ResolvedStartBlockNum, request.StopBlockNum, true)
	isCompleteRange := request.StopBlockNum%request.StateBundleSize == 0

	// note all modules that are not in 'modulesRequiredToRun' are still iterated in 'pipeline.executeModules', but they will skip actual execution when they see that the cache provides the data
	// This way, stores get updated at each block from the cached execouts without the actual execution of the module
	modulesRequiredToRun, existingExecOuts, execOutWriters, err := evaluateModulesRequiredToRun(ctx, logger, outputGraph, request.Stage, request.StartBlockNum, request.StopBlockNum, isCompleteRange, request.OutputModule, execOutputConfigs, storeConfigs)
	if err != nil {
		return fmt.Errorf("evaluating required modules: %w", err)
	}

	if len(modulesRequiredToRun) == 0 {
		logger.Info("no modules required to run, skipping")
		return nil
	}

	// this engine will keep the existingExecOuts to optimize the execution (for inputs from modules that skip execution)
	execOutputCacheEngine, err := cache.NewEngine(ctx, s.runtimeConfig, execOutWriters, request.BlockType, existingExecOuts)
	if err != nil {
		return fmt.Errorf("error building caching engine: %w", err)
	}

	//opts := s.buildPipelineOptions(ctx, request)
	var opts []pipeline.Option
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
		opts...,
	)

	logger.Debug("initializing tier2 pipeline",
		zap.Uint64("request_start_block", requestDetails.ResolvedStartBlockNum),
		zap.Uint64("request_stop_block", request.StopBlockNum),
		zap.String("output_module", request.OutputModule),
		zap.Uint32("stage", request.Stage),
	)

	if err := pipe.Init(ctx); err != nil {
		return fmt.Errorf("error during pipeline init: %w", err)
	}
	if err := pipe.InitTier2Stores(ctx); err != nil {
		return fmt.Errorf("error building pipeline: %w", err)
	}

	var streamFactoryFunc StreamFactoryFunc
	if s.streamFactoryFuncOverride != nil { //this is only for testing purposes.
		streamFactoryFunc = s.streamFactoryFuncOverride
	} else {
		sf := &StreamFactory{
			mergedBlocksStore: mergedBlocksStore,
		}
		streamFactoryFunc = sf.New
	}

	s.runtimeConfig.StateBundleSize = request.StateBundleSize

	var streamErr error
	if canSkipBlockSource(existingExecOuts, modulesRequiredToRun, request.BlockType) {
		maxDistributorLength := int(request.StopBlockNum - requestDetails.ResolvedStartBlockNum)
		clocksDistributor := make(map[uint64]*pbsubstreams.Clock)
		for _, execOutput := range existingExecOuts {
			execOutput.ExtractClocks(clocksDistributor)
			if len(clocksDistributor) >= maxDistributorLength {
				break
			}
		}

		sortedClocksDistributor := sortClocksDistributor(clocksDistributor)
		ctx, span := reqctx.WithSpan(ctx, "substreams/tier2/pipeline/mapper_stream")
		for _, clock := range sortedClocksDistributor {
			if clock.Number < request.StartBlockNum || clock.Number >= request.StopBlockNum {
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
	} else {
		blockStream, err := streamFactoryFunc(
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
	}

	return pipe.OnStreamTerminated(ctx, streamErr)
}

// evaluateModulesRequiredToRun will also load the existing execution outputs to be used as cache
// if it returns no modules at all, it means that we can skip the whole thing
func evaluateModulesRequiredToRun(
	ctx context.Context,
	logger *zap.Logger,
	outputGraph *outputmodules.Graph,
	stage uint32,
	startBlock uint64,
	stopBlock uint64,
	isCompleteRange bool,
	outputModule string,
	execoutConfigs *execout.Configs,
	storeConfigs store.ConfigMap,
) (requiredModules map[string]*pbsubstreams.Module, existingExecOuts map[string]*execout.File, execoutWriters map[string]*execout.Writer, err error) {
	existingExecOuts = make(map[string]*execout.File)
	requiredModules = make(map[string]*pbsubstreams.Module)
	execoutWriters = make(map[string]*execout.Writer)
	usedModules := make(map[string]*pbsubstreams.Module)
	for _, module := range outputGraph.UsedModulesUpToStage(int(stage)) {
		usedModules[module.Name] = module
	}

	stageUsedModules := outputGraph.StagedUsedModules()[stage]
	runningLastStage := stageUsedModules.IsLastStage()
	stageUsedModulesName := make(map[string]bool)
	for _, layer := range stageUsedModules {
		for _, mod := range layer {
			stageUsedModulesName[mod.Name] = true
		}
	}
	for name, c := range execoutConfigs.ConfigMap {
		if _, found := usedModules[name]; !found { // skip modules that are only present in later stages
			continue
		}

		file, readErr := c.ReadFile(ctx, &block.Range{StartBlock: startBlock, ExclusiveEndBlock: stopBlock})
		if readErr != nil {
			requiredModules[name] = usedModules[name]
			continue
		}
		existingExecOuts[name] = file

		if c.ModuleKind() == pbsubstreams.ModuleKindMap {
			if runningLastStage && name == outputModule {
				// WARNING be careful, if we want to force producing module outputs/stores states for ALL STAGES on the first block range,
				// this optimization will be in our way..
				logger.Info("found existing exec output for output_module, skipping run", zap.String("output_module", name))
				return nil, nil, nil, nil
			}
			continue
		}

		// if either full or partial kv exists, we can skip the module
		storeExists, err := storeConfigs[name].ExistsFullKV(ctx, stopBlock)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("checking fullkv file existence: %w", err)
		}
		if !storeExists {
			partialStoreExists, err := storeConfigs[name].ExistsPartialKV(ctx, startBlock, stopBlock)
			if err != nil {
				return nil, nil, nil, fmt.Errorf("checking partial file existence: %w", err)
			}
			if !partialStoreExists {
				// some stores may already exist completely on this stage, but others do not, so we keep going but ignore those
				requiredModules[name] = usedModules[name]
			}
		}
	}

	for name, module := range requiredModules {
		if _, exists := existingExecOuts[name]; exists {
			continue // for stores that need to be run for the partials, but already have cached execution outputs
		}
		if !isCompleteRange && name != outputModule {
			// if we are not running a complete range, we can skip writing the outputs of every module except the requested outputModule if it's in our stage
			continue
		}
		if module.ModuleKind() == pbsubstreams.ModuleKindStore {
			if _, found := stageUsedModulesName[name]; !found {
				continue
			}
		}

		execoutWriters[name] = execout.NewWriter(
			startBlock,
			stopBlock,
			name,
			execoutConfigs,
		)
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

//func (s *Tier2Service) buildPipelineOptions(ctx context.Context, request *pbssinternal.ProcessRangeRequest) (opts []pipeline.Option) {
//	requestDetails := reqctx.Details(ctx)
//	for _, pipeOpts := range s.pipelineOptions {
//		opts = append(opts, pipeOpts.PipelineOptions(ctx, request.StartBlockNum, request.StopBlockNum, requestDetails.UniqueIDString())...)
//	}
//	return
//}

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

		sendMetering(ctx, meter, userID, apiKeyID, ip, userMeta, "sf.substreams.internal.v2/ProcessRange", resp, logger)
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

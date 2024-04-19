package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/streamingfast/substreams/storage/index"

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

/*

// layer is one verticle with dependencies

stage 1:
* map1
* store1

[[map1,store1]]

stage 2: preload store1
* map2
* store2(reads: store1, map2)          * store3 (reads: store1)
  -> each block, apply changes to store1

[[store2],[store3]]

stage 3:
[[mapout]]

*/

type tier2ExecutionPlan struct {
	stagedLayeredModules [][][]*ModuleExecutionConfig
	// stages // layers // modules
}

/*
	for _, block := range blocks {

		for layer := range layeredModules {
			wg.Add(1)
			go run executeWASMModuleSequencially(layer, block)
		}
		wg.Wait()
	}
*/

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

	logger := reqctx.Logger(ctx).Named("tier2").With(zap.String("stage", stage), zap.Uint64("segment_number", request.SegmentNumber))

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
		zap.Uint64("segment_size", request.SegmentSize),
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

	if err := ValidateTier2Request(request); err != nil {
		return connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("validate request: %w", err))
	}

	emitter, err := dmetering.New(request.MeteringConfig, logger)
	if err != nil {
		return connect.NewError(connect.CodeInternal, fmt.Errorf("unable to initialize dmetering: %w", err))
	}
	defer func() {
		emitter.Shutdown(nil)
	}()

	ctx = reqctx.WithEmitter(ctx, dmetering.GetDefaultEmitter())

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
	return wasm.NewRegistry(exts, s.runtimeConfig.MaxWasmFuel), nil
}

func (s *Tier2Service) processRange(ctx context.Context, request *pbssinternal.ProcessRangeRequest, respFunc substreams.ResponseFunc) error {
	logger := reqctx.Logger(ctx)
	s.runtimeConfig.SegmentSize = request.SegmentSize

	mergedBlocksStore, cacheStore, err := s.getStores(ctx, request)
	if err != nil {
		return err
	}

	execGraph, err := exec.NewOutputModuleGraph(request.OutputModule, true, request.Modules)
	if err != nil {
		return stream.NewErrInvalidArg(err.Error())
	}

	requestDetails := pipeline.BuildRequestDetailsFromSubrequest(request)
	ctx = reqctx.WithRequest(ctx, requestDetails)
	if s.runtimeConfig.ModuleExecutionTracing {
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
		logger)
	if err != nil {
		return fmt.Errorf("new config map: %w", err)
	}

	storeConfigs, err := store.NewConfigMap(cacheStore, execGraph.Stores(), execGraph.ModuleHashes())
	if err != nil {
		return fmt.Errorf("configuring stores: %w", err)
	}

	// FIXME implement this instead of GenearteBlockIndexWriters
	//	indexConfigs, err := index.NewConfigMap(cacheStore, execGraph.UsedIndexesModulesUpToStage(int(request.Stage)), execGraph.ModuleHashes())

	// 3. we don't have a index.Config
	indexWriters, blockIndices, err := index.GenerateBlockIndexWriters(
		ctx,
		cacheStore,
		execGraph.UsedIndexesModulesUpToStage(int(request.Stage)),
		execGraph.ModuleHashes(),
		logger,
		&block.Range{StartBlock: startBlock, ExclusiveEndBlock: stopBlock},
		request.SegmentSize,
	)
	if err != nil {
		return fmt.Errorf("generating block index writers: %w", err)
	}

	// map[string]executionConfig
	//
	// type executionConfig struct{
	// 	runExecution bool
	//  produceDownstreamOutputs bool
	//  produceSnapshotOutputs bool --> we need a writer
	//  produceEventOutputs bool --> we need a writer
	//  blockFilter BlockFilter
	// }

	// type BlockFilter {
	//	 preexistingExecOuts map[uint64]struct{}
	// }

	//	bf := BlockFilter{}
	//
	//	bf.Apply(keys) // NOOP if preexistingExecOuts ...
	//	if bf.ShouldSkip(block.Number) {
	//		return
	//	}

	// for each module:

	// * execute it for real ?
	// * produce outputs for the next modules ? (store: deltas, map: outputs, index: keys)
	//     --> RunModule() returns the moduleOutput that goes into the cache for next module to read (store: kvdelta, mapper: outputs, index: keys)
	//   if we don't execute a required module, it probably already has its outputs in the downstreamOutput cache

	// * produce outputs to write in the cache store
	//     --> snapshotOutput() -> store: snapshot (full or partial!), mapper: nil, index: roaringBitmap
	//      --> eventOutput()  -> store: kvops,  mapper: outputs, index: nil

	// initialBlock (block at which we start executing that specific module, also used to determine the filenames of the outputs, ex: 00001012-10002000)

	stores := pipeline.NewStores(ctx, storeConfigs, request.SegmentSize, requestDetails.ResolvedStartBlockNum, stopBlock, true)
	isCompleteRange := stopBlock%request.SegmentSize == 0

	// 4. what defines the list of modules that should be executed ?? right now : only the presence of the existingExecOuts

	// TODO: replace this with generation of an execution Config, that is initialized using the indexConfig, execOutputConfig and the storeConfig
	//
	// note all modules that are not in 'modulesRequiredToRun' are still iterated in 'pipeline.executeModules', but they will skip actual execution when they see that the cache provides the data
	// This way, stores get updated at each block from the cached execouts without the actual execution of the module
	modulesRequiredToRun, existingExecOuts, execOutWriters, err := evaluateModulesRequiredToRun(ctx, logger, execGraph, request.Stage, startBlock, stopBlock, isCompleteRange, request.OutputModule, execOutputConfigs, storeConfigs)
	if err != nil {
		return fmt.Errorf("evaluating required modules: %w", err)
	}

	//// END

	if len(modulesRequiredToRun) == 0 {
		logger.Info("no modules required to run, skipping")
		return nil
	}

	// this engine will keep the existingExecOuts to optimize the execution (for inputs from modules that skip execution)
	execOutputCacheEngine, err := cache.NewEngine(ctx, s.runtimeConfig, execOutWriters, request.BlockType, existingExecOuts, indexWriters)
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
		blockIndices,
		execOutputConfigs,
		wasmRegistry,
		execOutputCacheEngine,
		s.runtimeConfig,
		respFunc,
		// This must always be the parent/global trace id, the one that comes from tier1
		opts...,
	)

	logger.Debug("initializing tier2 pipeline",
		zap.Uint64("request_start_block", requestDetails.ResolvedStartBlockNum),
		zap.String("output_module", request.OutputModule),
		zap.Uint32("stage", request.Stage),
	)

	s.runtimeConfig.SegmentSize = request.SegmentSize

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
			if existingExecOuts[executor.Name()] != nil {
				continue
			}
			if !executor.BlockIndexExcludesAllBlocks() {
				allExecutorsExcludedByBlockIndex = false
				break excludable
			}
		}
	}
	if allExecutorsExcludedByBlockIndex {
		// TODO: when we have a way to skip the whole thing, we should do it here
		logger.Info("all executors are excluded by block index. We could skip the whole thing (but we still need the clocks in the outputs, so we won't.)")
	}

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
	)
	if err != nil {
		return fmt.Errorf("error getting stream: %w", err)
	}

	ctx, span := reqctx.WithSpan(ctx, "substreams/tier2/pipeline/blocks_stream")
	streamErr = blockStream.Run(ctx)
	span.EndWithErr(&streamErr)

	return pipe.OnStreamTerminated(ctx, streamErr)
}

func (s *Tier2Service) getStores(ctx context.Context, request *pbssinternal.ProcessRangeRequest) (mergedBlocksStore, cacheStore dstore.Store, err error) {

	mergedBlocksStore, err = dstore.NewDBinStore(request.MergedBlocksStore)
	if err != nil {
		return nil, nil, fmt.Errorf("setting up block store from url %q: %w", request.MergedBlocksStore, err)
	}

	if cloned, ok := mergedBlocksStore.(dstore.Clonable); ok {
		mergedBlocksStore, err = cloned.Clone(ctx)
		if err != nil {
			return nil, nil, fmt.Errorf("cloning store: %w", err)
		}
		mergedBlocksStore.SetMeter(dmetering.GetBytesMeter(ctx))
	}

	stateStore, err := dstore.NewStore(request.StateStore, "zst", "zstd", false)
	if err != nil {
		return nil, nil, fmt.Errorf("getting store: %w", err)
	}

	cacheTag := request.StateStoreDefaultTag
	if auth := dauth.FromContext(ctx); auth != nil {
		if ct := auth.Get("X-Sf-Substreams-Cache-Tag"); ct != "" {
			if IsValidCacheTag(ct) {
				cacheTag = ct
			} else {
				return nil, nil, fmt.Errorf("invalid value for X-Sf-Substreams-Cache-Tag %s, should only contain letters, numbers, hyphens and undescores", ct)
			}
		}
	}

	cacheStore, err = stateStore.SubStore(cacheTag)
	if err != nil {
		return nil, nil, fmt.Errorf("internal error setting store: %w", err)
	}

	if clonableStore, ok := cacheStore.(dstore.Clonable); ok {
		cloned, err := clonableStore.Clone(ctx)
		if err != nil {
			return nil, nil, fmt.Errorf("cloning store: %w", err)
		}
		cloned.SetMeter(dmetering.GetBytesMeter(ctx))
		cacheStore = cloned
	}

	return
}

// evaluateModulesRequiredToRun will also load the existing execution outputs to be used as cache
// if it returns no modules at all, it means that we can skip the whole thing
func evaluateModulesRequiredToRun(
	ctx context.Context,
	logger *zap.Logger,
	execGraph *exec.Graph,
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

		if c.ModuleKind() == pbsubstreams.ModuleKindBlockIndex {
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

		sendMetering(ctx, meter, userID, apiKeyID, ip, userMeta, "sf.substreams.internal.v2/ProcessRange", resp)
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

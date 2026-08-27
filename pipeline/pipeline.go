package pipeline

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"connectrpc.com/connect"
	"github.com/RoaringBitmap/roaring/roaring64"
	"github.com/abourget/llerrgroup"
	"github.com/dustin/go-humanize"
	"github.com/streamingfast/bstream"
	"github.com/streamingfast/dmetering"
	"github.com/streamingfast/logging"
	"github.com/streamingfast/logging/zapx"
	tracing "github.com/streamingfast/sf-tracing"
	"github.com/streamingfast/substreams"
	"github.com/streamingfast/substreams/foundational_store"
	"github.com/streamingfast/substreams/metering"
	"github.com/streamingfast/substreams/orchestrator"
	"github.com/streamingfast/substreams/orchestrator/plan"
	"github.com/streamingfast/substreams/orchestrator/response"
	"github.com/streamingfast/substreams/orchestrator/work"
	pbservice "github.com/streamingfast/substreams/pb/sf/substreams/foundational-store/service/v2"
	pbssinternal "github.com/streamingfast/substreams/pb/sf/substreams/intern/v2"
	pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
	pbsubstreamsrpcv4 "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v4"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/pipeline/cache"
	"github.com/streamingfast/substreams/pipeline/exec"
	"github.com/streamingfast/substreams/reqctx"
	"github.com/streamingfast/substreams/sqe"
	"github.com/streamingfast/substreams/storage/execout"
	"github.com/streamingfast/substreams/storage/index"
	"github.com/streamingfast/substreams/storage/store"
	"github.com/streamingfast/substreams/wasm"
	"go.opentelemetry.io/otel"
	"go.uber.org/atomic"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/anypb"
)

// moduleKey combines binary index and type to handle cases where the same binary is used with different runtime extensions (wasm-bindgen-shims)
type moduleKey struct {
	binaryIndex uint32
	binaryType  string
}

type processingModule struct {
	name            string
	initialBlockNum uint64
}

type partialProcessingState struct {
	num                        uint64
	lastBlockID                string
	processedPartials          []*pbsubstreams.Clock
	processedTransactionsCount int
	processedTransactionsHash  []byte
	previousBlockRef           bstream.BlockRef // used for some emergency UNDOs if we detect that blocks are not based on same beginning in partials
	highestIndex               int32
}

func newPartialProcessingState(num uint64, id string, idx int32, prevBlockRef bstream.BlockRef) *partialProcessingState {
	return &partialProcessingState{
		num:              num,
		lastBlockID:      id,
		highestIndex:     idx,
		previousBlockRef: prevBlockRef,
	}
}

type Pipeline struct {
	ctx              context.Context
	isTier1          bool
	stateBundleSize  uint64
	workerFactory    work.WorkerPoolFactory
	executionTimeout time.Duration

	pendingUndoMessage *pbsubstreamsrpc.Response
	preBlockHooks      []substreams.BlockHook
	postBlockHooks     []substreams.BlockHook
	postJobHooks       []substreams.PostJobHook

	wasmRuntime   *wasm.Registry
	execGraph     *exec.Graph
	loadedModules map[moduleKey]wasm.Module
	// StagedModuleExecutors represents all the modules within a stage that should be executed. The
	// first level of the 2D list represents layer within a stage to execute sequentially.
	// The second level contains modules to execute within a layer, those can be executed concurrently.
	StagedModuleExecutors [][]exec.ModuleExecutor
	ModuleBlockIndexes    map[string]*index.BlockIndex
	executionStages       exec.ExecutionStages

	mapModuleOutput          *pbsubstreamsrpc.MapModuleOutput
	mapModuleOutputSkippable bool

	extraMapModuleOutputs   []*pbsubstreamsrpc.MapModuleOutput
	extraStoreModuleOutputs []*pbsubstreamsrpc.StoreModuleOutput
	preexistingBlockIndices map[string]map[string]*roaring64.Bitmap

	partialProcessingState   *partialProcessingState
	previousLastPartialBlock bstream.BlockRef // when we get a partial with 'isLast', we nil out partialProcessingState and write the block ref here.
	undonePartialsBlockNum   uint64           // when partials for a block number were undone after a transactions mismatch, further non-last partials for that number are dropped until its last partial or full block arrives

	respFunc         substreams.ResponseFunc
	lastProgressSent time.Time

	startTime         time.Time
	stores            *Stores
	execoutStorage    *execout.Configs
	moduleNameToStage map[string]int

	foundationalClients   map[string][]pbservice.StoreClient
	foundationalClosers   map[string][]func() error
	foundationalEndpoints map[string]string

	processingModule *processingModule

	gate            *gate
	finalBlocksOnly bool
	getHeadBlockNum func() (uint64, error)
	highestStage    *int

	forkHandler     *ForkHandler
	insideReorgUpTo bstream.BlockRef

	execOutputCache *cache.Engine
	moduleCache     *cache.ModuleCache
	blockType       string

	// currentBlockNum is the block being processed right now. Written by the block loop and
	// read by the progress snapshot goroutine, hence the atomic: it is what tells tier1 where
	// a job is stuck when a block takes minutes to go through.
	currentBlockNum atomic.Uint64

	// lastFinalClock should always be either THE `stopBlock` or a block beyond that point
	// (for chains with potential block skips)
	lastFinalClock        *pbsubstreams.Clock
	lastProcessedBlockRef bstream.BlockRef
	lastCursor            *bstream.Cursor
	sentBlocks            uint64
	quickSaved            bool
	sessionInitSent       bool // ensures a single Response_Session per request; see sendSession

	blockStepMap         map[bstream.StepType]uint64
	workerPoolFactory    work.WorkerPoolFactory
	checkPendingShutdown func() bool
	outputBufferSize     int
	supportBuffering     bool
}

func New(
	ctx context.Context,
	isTier1 bool,
	execGraph *exec.Graph,
	blockType string,
	stores *Stores,
	indices map[string]map[string]*roaring64.Bitmap,
	execoutStorage *execout.Configs,
	wasmRuntime *wasm.Registry,
	execOutputCache *cache.Engine,
	stateBundleSize uint64,
	workerPoolFactory work.WorkerPoolFactory,
	respFunc substreams.ResponseFunc,
	executionTimeout time.Duration,
	checkPendingShutdown func() bool,
	foundationalEndpoints map[string]string,
	outputBufferSize int,
	supportBuffering bool,
	opts ...Option,
) *Pipeline {
	pipe := &Pipeline{
		ctx:                     ctx,
		isTier1:                 isTier1,
		gate:                    newGate(ctx),
		execOutputCache:         execOutputCache,
		stateBundleSize:         stateBundleSize,
		preexistingBlockIndices: indices,
		blockType:               blockType,
		execGraph:               execGraph,
		wasmRuntime:             wasmRuntime,
		respFunc:                respFunc,
		stores:                  stores,
		foundationalClients:     make(map[string][]pbservice.StoreClient),
		foundationalClosers:     make(map[string][]func() error),
		foundationalEndpoints:   foundationalEndpoints,
		execoutStorage:          execoutStorage,
		forkHandler:             NewForkHandler(),
		blockStepMap:            make(map[bstream.StepType]uint64),
		startTime:               time.Now(),
		executionTimeout:        executionTimeout,
		workerPoolFactory:       workerPoolFactory,
		checkPendingShutdown:    checkPendingShutdown,
		moduleNameToStage:       make(map[string]int),
		outputBufferSize:        outputBufferSize,
		supportBuffering:        supportBuffering,
		ModuleBlockIndexes:      make(map[string]*index.BlockIndex),
	}
	for _, opt := range opts {
		opt(pipe)
	}

	slm := pipe.execGraph.StagedUsedModules()
	for stage, layers := range slm {
		for _, layer := range layers {
			for _, mod := range layer {
				pipe.moduleNameToStage[mod.Name] = stage
			}
		}
	}

	return pipe
}

func (p *Pipeline) Init(ctx context.Context) (err error) {
	reqDetails := reqctx.Details(ctx)

	p.forkHandler.registerUndoHandler(func(clock *pbsubstreams.Clock, moduleOutputs []*pbssinternal.ModuleOutput) {
		for _, modOut := range moduleOutputs {
			p.stores.storesHandleUndo(modOut)
		}
	})

	p.setupProcessingModule(reqDetails)

	stagedModules := p.execGraph.StagedUsedModules()

	// truncate stages to highest scheduled stage
	if highest := p.highestStage; highest != nil {
		if len(stagedModules) < *highest+1 {
			return fmt.Errorf("invalid stage %d, there aren't that many", highest)
		}
		stagedModules = stagedModules[0 : *highest+1]
	}
	p.executionStages = stagedModules

	return nil
}

func (p *Pipeline) InitTier2Stores(ctx context.Context) (err error) {

	storeMap, err := p.setupSubrequestStores(ctx)
	if err != nil {
		return fmt.Errorf("subrequest stores setup failed: %w", err)
	}

	p.stores.SetStoreMap(storeMap)

	logger := reqctx.Logger(ctx)
	logger.Debug("stores loaded", zap.Object("stores", p.stores.StoreMap), zap.Int("stage", reqctx.Details(ctx).Tier2Stage))

	return nil
}

// openStoresFromQuickload checks whether the request can resume linearly from its
// cursor by loading store state from the temporary quicksave files, and if so opens
// (but does NOT yet stream) those files. Returning a primed store map + the cursor
// block lets the caller emit the client's session/trace-id before the slow streaming
// decode in finishStoresFromQuickload. On any miss it cleans up and returns false so
// the caller falls back to a full parallel rebuild.
func (p *Pipeline) openStoresFromQuickload(ctx context.Context, reqPlan *plan.RequestPlan) (store.Map, bstream.BlockRef, bool) {

	// no stores to init
	if len(p.stores.configs) == 0 {
		return nil, nil, false
	}

	if reqPlan.LinearPipeline == nil {
		return nil, nil, false
	}

	if reqPlan.WriteExecOut != nil || reqPlan.ReadExecOut != nil {
		return nil, nil, false
	}

	details := reqctx.Details(ctx)
	if details.ResolvedCursor == "" {
		return nil, nil, false
	}

	cursor, err := bstream.CursorFromOpaque(details.ResolvedCursor)
	if err != nil {
		reqctx.Logger(ctx).Warn("invalid cursor", zap.Error(err))
		return nil, nil, false
	}

	if !reqPlan.LinearPipeline.Contains(cursor.Block.Num() + 1) {
		return nil, nil, false
	}

	storeMap := store.NewMap()
	for _, storeConfig := range p.stores.configs {
		storeMap[storeConfig.Name()] = storeConfig.NewFullKV(p.stores.logger)
	}

	if err := storeMap.QuickLoadOpen(ctx, cursor.Block); err != nil {
		p.stores.logger.Info("no temporary store files found", zap.Error(err))
		p.closeQuickloadStores(storeMap)
		return nil, nil, false
	}

	return storeMap, cursor.Block, true
}

// finishStoresFromQuickload streams the quicksave files opened by
// openStoresFromQuickload into the store map and installs it as the active store map.
func (p *Pipeline) finishStoresFromQuickload(ctx context.Context, storeMap store.Map, atBlock bstream.BlockRef) error {
	if err := storeMap.QuickLoadFinish(ctx, atBlock); err != nil {
		return err
	}
	reqctx.Logger(ctx).Info("skipping backprocessing, reading from temporary files", zap.Strings("stores", storeMap.Names()), zap.Uint64("block_num", atBlock.Num()), zap.String("block_id", atBlock.ID()))
	p.stores.StoreMap = storeMap
	return nil
}

// closeQuickloadStores releases the KV backends (e.g. the bbolt mmap file) of a
// quickload store map that will not be used, so a failed/abandoned quickload does
// not leak open stores.
func (p *Pipeline) closeQuickloadStores(storeMap store.Map) {
	for _, st := range storeMap.All() {
		if closer, ok := st.(interface{ Close() error }); ok {
			if cerr := closer.Close(); cerr != nil {
				p.stores.logger.Warn("failed to close store after quickload failure", zap.String("store", st.Name()), zap.Error(cerr))
			}
		}
	}
}

func (p *Pipeline) InitTier1StoresAndBackprocess(ctx context.Context, reqPlan *plan.RequestPlan, noopMode bool) (bool, error) {
	if storeMap, atBlock, ok := p.openStoresFromQuickload(ctx, reqPlan); ok {
		// Emit the session (trace id) and start keepalives BEFORE streaming the
		// quicksave files: that decode can take a long time for a large store on a
		// cold/remote quicksave store, and it used to run with zero output to the
		// client, risking a gateway/client idle-timeout before the trace id ever
		// arrived. The files are already open here, so we're committed to the linear
		// resume path.
		if err := p.emitLinearSessionInit(ctx); err != nil {
			storeMap.QuickLoadClose()
			p.closeQuickloadStores(storeMap)
			return false, err
		}

		// A decode failure here (corrupt/truncated file, or canceled context) falls
		// through to a full rebuild below, exactly as a missing file would. The already
		// -emitted linear session is deduped by sendSession, so the rebuild path won't
		// emit a second, conflicting one. Release the abandoned store map first (open
		// readers + mmap db/temp file), and leave FromQuickload false so the rebuild is
		// not mistaken for a genuine quickload resume downstream (live backfiller).
		if err := p.finishStoresFromQuickload(ctx, storeMap, atBlock); err != nil {
			p.stores.logger.Warn("quicksave present but failed to decode, falling back to full processing", zap.Error(err))
			storeMap.QuickLoadClose()
			p.closeQuickloadStores(storeMap)
		} else {
			reqctx.Details(ctx).FromQuickload = true
			return true, nil
		}
	}

	if reqPlan.RequiresParallelProcessing() {
		storeMap, err := p.runParallelProcess(ctx, reqPlan, noopMode, p.getHeadBlockNum)
		if err != nil {
			return false, fmt.Errorf("run_parallel_process failed: %w", err)
		}
		p.stores.SetStoreMap(storeMap) // this is valid even if we don't have stores in the parallelProcessing but only a mapper
		return false, nil
	} else {
		if err := p.emitLinearSessionInit(ctx); err != nil {
			return false, err
		}
	}

	p.stores.SetStoreMap(p.setupEmptyStores(ctx))
	return false, nil
}

// sendSession emits the request's Response_Session, at most once. Both the linear
// (emitLinearSessionInit) and parallel (runParallelProcess) paths funnel through
// here, so a quickload that emits its linear session and then falls back to a full
// rebuild cannot produce a second, conflicting session. Called only from the request
// goroutine, so the flag needs no synchronization.
func (p *Pipeline) sendSession(session *pbsubstreamsrpc.SessionInit) {
	if p.sessionInitSent {
		return
	}
	p.sessionInitSent = true
	p.respFunc(&pbsubstreamsrpc.Response{
		Message: &pbsubstreamsrpc.Response_Session{Session: session},
	})
}

// emitLinearSessionInit sends the Response_Session message for a purely linear run
// (no parallel backprocessing): quickload resume from a cursor, or a start block
// already inside the linear pipeline. The parallel path emits its own Session message
// from runParallelProcess.
func (p *Pipeline) emitLinearSessionInit(ctx context.Context) error {
	reqDetails := reqctx.Details(ctx)
	var toProcessAfter uint64
	estimateProcessUpto := reqDetails.StopBlockNum
	if estimateProcessUpto == 0 && p.getHeadBlockNum != nil {
		headBlock, err := p.getHeadBlockNum()
		if err != nil {
			reqctx.Logger(ctx).Warn("cannot get head block for sessionInit", zap.Error(err))
		} else {
			estimateProcessUpto = headBlock
		}
	}

	if estimateProcessUpto != 0 && estimateProcessUpto > reqDetails.ResolvedStartBlockNum {
		toProcessAfter = estimateProcessUpto - reqDetails.ResolvedStartBlockNum
	}

	p.sendSession(&pbsubstreamsrpc.SessionInit{
		TraceId:                                  tracing.GetTraceID(ctx).String(),
		ResolvedStartBlock:                       reqDetails.ResolvedStartBlockNum,
		LinearHandoffBlock:                       reqDetails.LinearHandoffBlockNum,
		MaxParallelWorkers:                       reqDetails.MaxParallelJobs,
		BlocksToProcessBeforeStartBlock:          0,
		EffectiveBlocksToProcessBeforeStartBlock: 0,
		BlocksToProcessAfterStartBlock:           toProcessAfter, // only linear processing
		EffectiveBlocksToProcessAfterStartBlock:  toProcessAfter, // only linear processing
		SegmentBlockCount:                        p.stateBundleSize,
	})

	if err := reqDetails.AssertProcessedBlocksLimit(0, toProcessAfter); err != nil {
		return connect.NewError(connect.CodeFailedPrecondition, err)
	}

	return nil
}

func (p *Pipeline) GetStoreMap() store.Map {
	return p.stores.StoreMap
}

func (p *Pipeline) LastCursor() *bstream.Cursor {
	return p.lastCursor
}

func (p *Pipeline) setupProcessingModule(reqDetails *reqctx.RequestDetails) {
	for _, module := range reqDetails.Modules.Modules {
		if reqDetails.IsOutputModule(module.Name) {
			p.processingModule = &processingModule{
				name:            module.GetName(),
				initialBlockNum: reqDetails.ResolvedStartBlockNum,
			}
		}
	}
}

// setupSubrequestStores will prepare stores for all required modules up to the current stage.
func (p *Pipeline) setupSubrequestStores(ctx context.Context) (storeMap store.Map, err error) {
	ctx, span := reqctx.WithSpan(ctx, "substreams/pipeline/tier2/store_setup")
	defer span.EndWithErr(&err)

	reqDetails := reqctx.Details(ctx)
	logger := reqctx.Logger(ctx)

	storeMap = store.NewMap()
	defer func() {
		if err != nil {
			for _, st := range storeMap.All() {
				if closer, ok := st.(interface{ Close() error }); ok {
					if cerr := closer.Close(); cerr != nil {
						logger.Warn("failed to close store after setup error", zap.String("store", st.Name()), zap.Error(cerr))
					}
				}
			}
		}
	}()

	type loadable struct {
		fullKVStore *store.FullKV
		fileInfo    *store.FileInfo
	}
	var loadableStores []*loadable

	lastStage := len(p.executionStages) - 1
	for stageIdx, stage := range p.executionStages {
		if p.highestStage != nil && stageIdx > *p.highestStage {
			break // skip stores for stages that we're not running
		}
		isLastStage := stageIdx == lastStage
		layer := stage.LastLayer()
		if !layer.IsStoreLayer() {
			continue
		}
		for _, mod := range layer {
			storeConfig := p.stores.configs[mod.Name]

			if isLastStage {
				initialBlock := reqDetails.ResolvedStartBlockNum
				if storeConfig.ModuleInitialBlock() > reqDetails.ResolvedStartBlockNum {
					initialBlock = storeConfig.ModuleInitialBlock()
				}
				partialStore := storeConfig.NewPartialKV(initialBlock, logger)
				storeMap.Set(partialStore)

			} else {
				fullStore := storeConfig.NewFullKV(logger)

				if fullStore.InitialBlock() < reqDetails.ResolvedStartBlockNum {
					file := store.NewCompleteFileInfo(fullStore.Name(), fullStore.InitialBlock(), reqDetails.ResolvedStartBlockNum)
					loadableStores = append(loadableStores, &loadable{
						fullKVStore: fullStore,
						fileInfo:    file,
					})

				}
				storeMap.Set(fullStore)
			}
		}
	}

	storesMetadata := make(map[string]map[string]string)
	var neededSize uint64
	var sizeMu sync.Mutex
	egSize := llerrgroup.New(8)
	for _, loadable := range loadableStores {
		if egSize.Stop() {
			break
		}
		egSize.Go(func() error {
			compressed, uncompressed, metadata, err := loadable.fullKVStore.GetSize(ctx, loadable.fileInfo.Filename) // ignore error here
			if err != nil {
				logger.Debug("failed to get size of store", zap.String("store_name", loadable.fullKVStore.Name()), zap.Error(err))
			}
			sizeMu.Lock()
			if uncompressed == nil {
				neededSize += compressed * 4
			} else {
				neededSize += *uncompressed
			}
			storesMetadata[loadable.fullKVStore.Name()] = metadata
			sizeMu.Unlock()
			return nil
		})
	}
	if err := egSize.Wait(); err != nil {
		return nil, err
	}

	logger.Info("about to load stores", zap.String("approx_store_size", humanize.IBytes(neededSize)))

	if reqHandler := reqctx.ActiveRequestsHandler(ctx); reqHandler != nil {
		reqHandler.AllocateFullKVSizeOrForceCancelRequest(neededSize)
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
	}

	var actualRequestStoresSize uint64
	var loadMu sync.Mutex
	egLoad := llerrgroup.New(8)
	for _, loadable := range loadableStores {
		if egLoad.Stop() {
			break
		}
		egLoad.Go(func() error {
			if err := loadable.fullKVStore.Load(ctx, loadable.fileInfo); err != nil {
				// A canceled context surfaces as ErrInvalidFullKVFile because the
				// streaming unmarshal wraps the read error as a string, so only
				// treat the file as corrupt when the context is still live —
				// otherwise a canceled request would delete a valid store file.
				if errors.Is(err, store.ErrInvalidFullKVFile) && ctx.Err() == nil {
					logger.Warn("found a corrupted fullKV store, deleting it", zap.Error(err), zap.String("store_name", loadable.fullKVStore.Name()), zap.String("store_hash", loadable.fullKVStore.ModuleHash()), zap.String("filename", loadable.fileInfo.Filename))
					if err := loadable.fullKVStore.Delete(ctx, loadable.fileInfo); err != nil {
						logger.Error("cannot delete corrupted fullKV store", zap.Error(err), zap.String("store_name", loadable.fullKVStore.Name()), zap.String("store_hash", loadable.fullKVStore.ModuleHash()), zap.String("filename", loadable.fileInfo.Filename))
					}
				}
				return fmt.Errorf("load full store %s (%s): %w", loadable.fullKVStore.Name(), loadable.fullKVStore.ModuleHash(), err)
			}
			//  add loaded file size to metadata
			actualSize := loadable.fullKVStore.SizeBytes()
			loadMu.Lock()
			met := storesMetadata[loadable.fullKVStore.Name()]
			if met == nil {
				met = make(map[string]string)
			}
			if met["datasize"] == "" {
				met["datasize"] = fmt.Sprintf("%d", actualSize)
			}
			actualRequestStoresSize += actualSize
			loadMu.Unlock()
			// Detached metadata write: does not retain fullKVStore or ride the
			// request ctx (see store.SetMetadataDetached).
			store.SetMetadataDetached(loadable.fullKVStore.Store(), loadable.fullKVStore.Filename(), loadable.fullKVStore.Name(), met, logger)
			return nil
		})
	}
	if err := egLoad.Wait(); err != nil {
		return nil, err
	}

	if reqHandler := reqctx.ActiveRequestsHandler(ctx); reqHandler != nil {
		logger.Info("adjusting to stores size", zap.String("approx_store_size", humanize.IBytes(actualRequestStoresSize)))
		reqHandler.AdjustFullKVSize(actualRequestStoresSize)
	}

	return storeMap, nil
}

func (p *Pipeline) setupEmptyStores(ctx context.Context) store.Map {
	logger := reqctx.Logger(ctx)
	storeMap := store.NewMap()
	for _, storeConfig := range p.stores.configs {
		fullStore := storeConfig.NewFullKV(logger)
		storeMap.Set(fullStore)
	}
	return storeMap
}

// runParallelProcess
func (p *Pipeline) runParallelProcess(ctx context.Context, reqPlan *plan.RequestPlan, noopMode bool, getHeadBlockNum func() (uint64, error)) (storeMap store.Map, err error) {
	ctx, span := reqctx.WithSpan(ctx, "substreams/pipeline/tier1/parallel_process")
	defer span.EndWithErr(&err)

	reqDetails := reqctx.Details(ctx)
	reqStats := reqctx.ReqStats(ctx)
	logger := reqctx.Logger(ctx)

	if reqDetails.ShouldStreamCachedOutputs() && p.pendingUndoMessage != nil {
		p.respFunc(p.pendingUndoMessage)
	}

	workerPool := p.workerPoolFactory(ctx)
	parallelProcessor, err := orchestrator.BuildParallelProcessor(
		ctx,
		reqPlan,
		workerPool,
		p.execGraph,
		p.execoutStorage,
		p.respFunc,
		p.stores.configs,
		noopMode,
		p.outputBufferSize,
		p.supportBuffering,
	)
	if err != nil {
		return nil, fmt.Errorf("building parallel processor: %w", err)
	}

	var headBlockNum uint64
	if getHeadBlockNum != nil {
		headBlockNum, err = getHeadBlockNum()
		if err != nil {
			logger.Warn("cannot get head block when checking at parallel processor", zap.Error(err))
		}
	}

	blocksBefore, effectiveBlocksBefore, blocksAfter, effectiveBlocksAfter := parallelProcessor.Stages().BlocksToProcess(headBlockNum)
	p.sendSession(&pbsubstreamsrpc.SessionInit{
		TraceId:                                  tracing.GetTraceID(ctx).String(),
		ResolvedStartBlock:                       reqDetails.ResolvedStartBlockNum,
		LinearHandoffBlock:                       reqDetails.LinearHandoffBlockNum,
		MaxParallelWorkers:                       reqDetails.MaxParallelJobs,
		BlocksToProcessBeforeStartBlock:          blocksBefore,
		BlocksToProcessAfterStartBlock:           blocksAfter,
		EffectiveBlocksToProcessBeforeStartBlock: effectiveBlocksBefore,
		EffectiveBlocksToProcessAfterStartBlock:  effectiveBlocksAfter,
		SegmentBlockCount:                        p.stateBundleSize,
	})

	if err := reqDetails.AssertProcessedBlocksLimit(effectiveBlocksBefore, effectiveBlocksAfter); err != nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, err)
	}

	stats := reqctx.ReqStats(ctx)
	progressCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() {
		stream := response.New(p.respFunc)

		meter := dmetering.GetBytesMeter(ctx)

		for {
			select {
			case <-time.After(progressMessageInterval(time.Since(p.startTime), reqDetails.UpdateInterval)):
				stagesProgress := stats.Stages()
				jobs := stats.JobsStats()
				modStats := stats.AggregatedModulesStats()
				remoteBytesRead, remoteBytesWritten := stats.RemoteBytesConsumption()
				stream.SendModulesStats(modStats, stagesProgress, jobs, metering.GetTotalBytesRead(meter)+remoteBytesRead, metering.GetTotalBytesWritten(meter)+remoteBytesWritten, stats.GetBlocksProcessed())
			case <-progressCtx.Done():
				return
			}
		}
	}()

	logger.Debug("starting parallel processing")

	storeMap, err = parallelProcessor.Run(ctx, p.checkPendingShutdown)
	if err != nil {
		return nil, fmt.Errorf("parallel processing run: %w", err)
	}
	reqStats.RecordInitializationComplete()

	return storeMap, nil
}

func (p *Pipeline) isOutputModule(name string) bool {
	return p.execGraph.IsOutputModule(name)
}

func (p *Pipeline) runPostJobHooks(ctx context.Context, clock *pbsubstreams.Clock) {
	for _, hook := range p.postJobHooks {
		if err := hook(ctx, clock); err != nil {
			reqctx.Logger(ctx).Warn("post job hook failed", zap.Error(err))
		}
	}
}

func (p *Pipeline) runPreBlockHooks(ctx context.Context, clock *pbsubstreams.Clock) (err error) {
	for _, hook := range p.preBlockHooks {
		if err := hook(ctx, clock); err != nil {
			return fmt.Errorf("pre block hook: %w", err)
		}
	}
	return nil
}

// TODO: move this to `responses`
func toRPCStoreModuleOutputs(in *pbssinternal.ModuleOutput) (out *pbsubstreamsrpc.StoreModuleOutput) {
	deltas := in.GetStoreDeltas()
	if deltas == nil {
		return nil
	}

	return &pbsubstreamsrpc.StoreModuleOutput{
		Name:             in.ModuleName,
		DebugStoreDeltas: toRPCDeltas(deltas),
		DebugInfo: &pbsubstreamsrpc.OutputDebugInfo{
			Logs:          in.Logs,
			LogsTruncated: in.DebugLogsTruncated,
			Cached:        in.Cached,
		},
	}
}

func toRPCDeltas(in *pbsubstreams.StoreDeltas) (out []*pbsubstreamsrpc.StoreDelta) {
	if len(in.StoreDeltas) == 0 {
		return nil
	}

	out = make([]*pbsubstreamsrpc.StoreDelta, len(in.StoreDeltas))
	for i, d := range in.StoreDeltas {
		out[i] = &pbsubstreamsrpc.StoreDelta{
			Operation: toRPCOperation(d.Operation),
			Ordinal:   d.Ordinal,
			Key:       d.Key,
			OldValue:  d.OldValue,
			NewValue:  d.NewValue,
		}
	}
	return
}

func toRPCOperation(in pbsubstreams.StoreDelta_Operation) (out pbsubstreamsrpc.StoreDelta_Operation) {
	switch in {
	case pbsubstreams.StoreDelta_UPDATE:
		return pbsubstreamsrpc.StoreDelta_UPDATE
	case pbsubstreams.StoreDelta_CREATE:
		return pbsubstreamsrpc.StoreDelta_CREATE
	case pbsubstreams.StoreDelta_DELETE:
		return pbsubstreamsrpc.StoreDelta_DELETE
	}
	return pbsubstreamsrpc.StoreDelta_UNSET
}

func toRPCMapModuleOutputs(in *pbssinternal.ModuleOutput) (out *pbsubstreamsrpc.MapModuleOutput) {
	data := in.GetMapOutput()
	if data == nil {
		return nil
	}

	return &pbsubstreamsrpc.MapModuleOutput{
		Name:      in.ModuleName,
		MapOutput: data,
		DebugInfo: &pbsubstreamsrpc.OutputDebugInfo{
			Logs:          in.Logs,
			LogsTruncated: in.DebugLogsTruncated,
			Cached:        in.Cached,
		},
	}
}

// progressMessageInterval returns how long to wait before the next progress message is
// sent to the client. A non-zero requested interval, from Request.progress_messages_interval_ms,
// pins the cadence for the whole request. Otherwise the interval widens as the request ages:
// progress matters most while the client is still waiting for its first data, and every
// message is billed as egress.
func progressMessageInterval(elapsed, requested time.Duration) time.Duration {
	if requested > 0 {
		return requested
	}

	switch {
	case elapsed < time.Minute:
		return time.Second
	case elapsed < 5*time.Minute:
		return 10 * time.Second
	case elapsed < 10*time.Minute:
		return 30 * time.Second
	default:
		return time.Minute
	}
}

func (p *Pipeline) returnRPCModuleProgressOutputs(forceOutput bool) error {
	interval := progressMessageInterval(time.Since(p.startTime), reqctx.Details(p.ctx).UpdateInterval)
	if time.Since(p.lastProgressSent) < interval && !forceOutput {
		return nil
	}
	p.lastProgressSent = time.Now()

	stats := reqctx.ReqStats(p.ctx)
	stream := response.New(p.respFunc)
	stagesProgress := stats.Stages()
	jobs := stats.JobsStats()
	modStats := stats.AggregatedModulesStats()

	meter := dmetering.GetBytesMeter(p.ctx)
	remoteBytesRead, remoteBytesWritten := stats.RemoteBytesConsumption()
	return stream.SendModulesStats(modStats, stagesProgress, jobs, metering.GetTotalBytesRead(meter)+remoteBytesRead, metering.GetTotalBytesWritten(meter)+remoteBytesWritten, stats.GetBlocksProcessed())

}

func (p *Pipeline) toInternalUpdate(clock *pbsubstreams.Clock) *pbssinternal.Update {
	meter := dmetering.GetBytesMeter(p.ctx)

	out := &pbssinternal.Update{
		DurationMs:        uint64(time.Since(p.startTime).Milliseconds()),
		TotalBytesRead:    metering.GetTotalBytesRead(meter),
		TotalBytesWritten: metering.GetTotalBytesWritten(meter),
		ModulesStats:      reqctx.ReqStats(p.ctx).LocalModulesStats(),
	}

	if clock != nil {
		out.ProgressBlocks = clock.Number - p.processingModule.initialBlockNum
	}
	return out
}

func (p *Pipeline) returnInternalModuleComplete() error {

	out := &pbssinternal.ProcessRangeResponse{
		Type: &pbssinternal.ProcessRangeResponse_Completed{
			Completed: &pbssinternal.Completed{
				ProcessedBlocks: reqctx.ReqStats(p.ctx).GetBlocksProcessed(),
				StreamingMode:   reqctx.Details(p.ctx).IsStreamingTier2,
			},
		},
	}

	if err := p.respFunc(out); err != nil {
		return fmt.Errorf("calling return func: %w", err)
	}
	return nil
}

// SendProgressSnapshot emits an internal progress update from outside the block loop.
//
// returnInternalModuleProgressOutputs only runs once a block finished processing, so a module
// stuck inside a single block — typically blocked on an external call that is retrying against
// an unreachable endpoint — reports nothing at all for as long as it is stuck, and tier1 sees
// an idle job with no external call metrics. The stats already account for calls still in
// flight; they just need to be sent. Safe to call concurrently with the block loop: the tier2
// response function serializes sends.
func (p *Pipeline) SendProgressSnapshot() error {
	if p.respFunc == nil {
		return nil
	}

	update := p.toInternalUpdate(nil)
	// toInternalUpdate only knows how to derive the progress from a completed block, and a
	// snapshot is taken precisely when none completed. Reporting 0 here would overwrite the
	// job's progress on tier1 with every snapshot.
	if current := p.currentBlockNum.Load(); current != 0 && p.processingModule != nil && current > p.processingModule.initialBlockNum {
		update.ProgressBlocks = current - p.processingModule.initialBlockNum
	}

	return p.respFunc(&pbssinternal.ProcessRangeResponse{
		Type: &pbssinternal.ProcessRangeResponse_Update{
			Update: update,
		},
	})
}

func (p *Pipeline) returnInternalModuleProgressOutputs(clock *pbsubstreams.Clock, forceOutput bool) error {
	if time.Since(p.lastProgressSent) < internalProgressMessageInterval && !forceOutput {
		return nil
	}
	p.lastProgressSent = time.Now()

	upd := p.toInternalUpdate(clock)

	out := &pbssinternal.ProcessRangeResponse{
		Type: &pbssinternal.ProcessRangeResponse_Update{
			Update: upd,
		},
	}

	if err := p.respFunc(out); err != nil {
		return fmt.Errorf("calling return func: %w", err)
	}
	return nil
}

func getFoundationalStores(inputs []wasm.Argument) []pbservice.StoreClient {
	for _, arg := range inputs {
		if fStore, ok := arg.(*wasm.FoundationalStoreInput); ok {
			return fStore.Clients
		}
	}
	return nil
}

// BuildModuleExecutors builds the ModuleExecutors, and the loadedModules.
func (p *Pipeline) BuildModuleExecutors(ctx context.Context) error {
	if p.StagedModuleExecutors != nil {
		// Eventually, we can invalidate our catch to accommodate the PATCH
		// and rebuild all the modules, and tear down the previously loaded ones.
		return nil
	}

	logger := reqctx.Logger(ctx)
	logger = logger.Named("BuildModuleExecutors")

	reqDetails := reqctx.Details(ctx)
	reqModules := reqDetails.Modules
	tracer := otel.GetTracerProvider().Tracer("executor")

	loadedModules := make(map[moduleKey]wasm.Module)
	loadingModuleStart := time.Now()
	moduleHashDuration := time.Duration(0)
	for _, stage := range p.executionStages {
		for _, layer := range stage {
			for _, module := range layer {
				code := reqModules.Binaries[module.BinaryIndex]
				key := moduleKey{binaryIndex: module.BinaryIndex, binaryType: code.Type}

				moduleHashStart := time.Now()
				moduleHash := p.execGraph.ModuleHashes()[module.Name]
				_ = moduleHash
				moduleHashDuration += time.Since(moduleHashStart)

				if _, exists := loadedModules[key]; exists {
					continue
				}

				m, err := p.wasmRuntime.NewModule(ctx, code.Content, code.Type)
				if err != nil {
					return fmt.Errorf("new wasm module: %w", err)
				}
				if p.moduleCache != nil {
					p.moduleCache.Set(moduleHash, m)
				}
				loadedModules[key] = m
			}
		}
	}
	loadingModuleDuration := time.Since(loadingModuleStart)
	p.loadedModules = loadedModules
	modulesInitBlocks := p.execGraph.ModulesInitBlocks()

	executerCreationStart := time.Now()
	var stagedModuleExecutors [][]exec.ModuleExecutor
	for _, stage := range p.executionStages {
		for _, layer := range stage {
			var moduleExecutors []exec.ModuleExecutor
			for _, module := range layer {
				inputs, err := p.renderWasmInputs(module)
				if err != nil {
					return fmt.Errorf("module %q: get wasm inputs: %w", module.Name, err)
				}
				var moduleBlockIndex *index.BlockIndex
				if module.BlockFilter != nil {
					qs, err := module.BlockFilterQueryString()
					if err != nil {
						return err
					}
					expr, err := sqe.Parse(ctx, qs)
					if err != nil {
						return fmt.Errorf("parse block filter: %q: %w", module.BlockFilter.Query, err)
					}
					var precomputedBitmap *roaring64.Bitmap

					if indices := p.preexistingBlockIndices[module.BlockFilter.Module]; indices != nil {
						precomputedBitmap = sqe.RoaringBitmapsApply(expr, indices)
					}
					moduleBlockIndex = index.NewBlockIndex(expr, module.BlockFilter.Module, precomputedBitmap)
				}

				entrypoint := module.BinaryEntrypoint
				code := reqModules.Binaries[module.BinaryIndex]
				key := moduleKey{binaryIndex: module.BinaryIndex, binaryType: code.Type}
				mod := loadedModules[key]

				foundationalStores := getFoundationalStores(inputs)
				switch kind := module.Kind.(type) {
				case *pbsubstreams.Module_KindMap_:
					p.ModuleBlockIndexes[module.Name] = moduleBlockIndex
					outType := strings.TrimPrefix(module.Output.Type, "proto:")

					baseExecutor := exec.NewBaseExecutor(
						ctx,
						module.Name,
						p.execGraph.ModuleHashes()[module.Name],
						modulesInitBlocks[module.Name],
						mod,
						p.wasmRuntime.InstanceCacheEnabled(),
						inputs,
						moduleBlockIndex,
						entrypoint,
						tracer,
						foundationalStores,
					)
					executor := exec.NewMapperModuleExecutor(baseExecutor, outType)
					moduleExecutors = append(moduleExecutors, executor)

				case *pbsubstreams.Module_KindStore_:
					p.ModuleBlockIndexes[module.Name] = moduleBlockIndex
					updatePolicy := kind.KindStore.UpdatePolicy
					valueType := kind.KindStore.ValueType

					outputStore, found := p.stores.StoreMap.Get(module.Name)
					if !found {
						return fmt.Errorf("store %q not found", module.Name)
					}
					inputs = append(inputs, wasm.NewStoreWriterOutput(module.Name, outputStore, updatePolicy, valueType))

					baseExecutor := exec.NewBaseExecutor(
						ctx,
						module.Name,
						p.execGraph.ModuleHashes()[module.Name],
						modulesInitBlocks[module.Name],
						mod,
						p.wasmRuntime.InstanceCacheEnabled(),
						inputs,
						moduleBlockIndex,
						entrypoint,
						tracer,
						foundationalStores,
					)
					executor := exec.NewStoreModuleExecutor(baseExecutor, outputStore)
					moduleExecutors = append(moduleExecutors, executor)

				case *pbsubstreams.Module_KindBlockIndex_:
					p.ModuleBlockIndexes[module.Name] = moduleBlockIndex // this must be before the 'break': we check the index later even if we don't execute this module
					if indices := p.preexistingBlockIndices[module.Name]; indices != nil {
						break // don't execute index modules that are useless
					}
					baseExecutor := exec.NewBaseExecutor(
						ctx,
						module.Name,
						p.execGraph.ModuleHashes()[module.Name],
						modulesInitBlocks[module.Name],
						mod,
						p.wasmRuntime.InstanceCacheEnabled(),
						inputs,
						moduleBlockIndex,
						entrypoint,
						tracer,
						foundationalStores,
					)

					executor := exec.NewIndexModuleExecutor(baseExecutor)
					moduleExecutors = append(moduleExecutors, executor)

				default:
					panic(fmt.Errorf("invalid kind %q input module %q", module.Kind, module.Name))
				}
			}
			stagedModuleExecutors = append(stagedModuleExecutors, moduleExecutors)
		}
	}

	executerCreationDuratiom := time.Since(executerCreationStart)

	logger.Info("build module executers",
		zapx.HumanDuration("module hash", moduleHashDuration),
		zapx.HumanDuration("build module executers", executerCreationDuratiom),
		zapx.HumanDuration("module loading", loadingModuleDuration),
		zap.Bool("keep", false),
	)

	p.StagedModuleExecutors = stagedModuleExecutors
	return nil
}

func (p *Pipeline) cleanUpModuleExecutors(ctx context.Context, logger *zap.Logger) error {
	for _, layer := range p.StagedModuleExecutors {
		for _, executor := range layer {
			if err := executor.Close(ctx); err != nil {
				return fmt.Errorf("closing module executor %q: %w", executor.Name(), err)
			}
		}
	}
	for key, mod := range p.loadedModules {
		if err := mod.Close(ctx); err != nil {
			return fmt.Errorf("closing wasm module %+v: %w", key, err)
		}
	}

	return nil
}

// closeFoundationalResources closes the per-identifier foundational store gRPC
// clients. gRPC connections are not tied to a context lifetime, so they must
// be closed explicitly to avoid leaking sockets and background goroutines
// across requests. The control-plane registry client is process-wide (built
// once on the tier1/tier2 service) and is not closed here.
func (p *Pipeline) closeFoundationalResources(logger *zap.Logger) {
	for identifier, closers := range p.foundationalClosers {
		for _, closer := range closers {
			if err := closer(); err != nil {
				logger.Warn("closing foundational store client", zap.String("identifier", identifier), zap.Error(err))
			}
		}
	}
	p.foundationalClosers = nil
	p.foundationalClients = nil
}

// normalizedOpaqueCursor returns a opaque cursor string without some
// esoteric steps like 'NewPartial' and 'UndoPartial' which may not be supported by some clients
func normalizedOpaqueCursor(cursor bstream.Cursor) string {
	switch cursor.Step {
	case bstream.StepNewPartial:
		cursor.Step = bstream.StepNew
	case bstream.StepUndoPartial:
		cursor.Step = bstream.StepUndo
	}
	return cursor.ToOpaque()
}

func returnPartialDataOutput(
	clock *pbsubstreams.Clock,
	cursor *bstream.Cursor,
	mapModuleOutput *pbsubstreamsrpc.MapModuleOutput,
	respFunc substreams.ResponseFunc,
	partialIdx uint32,
	lastPartial bool,
	supportBuffering bool,
) error {

	out := &pbsubstreamsrpc.BlockScopedData{
		Clock:            clock,
		Cursor:           normalizedOpaqueCursor(*cursor),
		FinalBlockHeight: cursor.LIB.Num(),
		Output:           mapModuleOutput,
		IsPartial:        true,
		PartialIndex:     &partialIdx,
		IsLastPartial:    &lastPartial,
	}

	if supportBuffering { //v4 support
		bsd := &pbsubstreamsrpcv4.BlockScopedDatas{
			Items: []*pbsubstreamsrpc.BlockScopedData{
				out,
			},
		}

		if err := respFunc(substreams.NewBlockScopedDatasResponse(bsd)); err != nil {
			return fmt.Errorf("calling return func: %w", err)
		}
		return nil
	}

	if err := respFunc(substreams.NewBlockScopedDataResponse(out)); err != nil {
		return fmt.Errorf("calling response func: %w", err)
	}

	return nil
}

func returnModuleDataOutputs(
	clock *pbsubstreams.Clock,
	cursor *bstream.Cursor,
	mapModuleOutput *pbsubstreamsrpc.MapModuleOutput,
	extraMapModuleOutputs []*pbsubstreamsrpc.MapModuleOutput,
	extraStoreModuleOutputs []*pbsubstreamsrpc.StoreModuleOutput,
	respFunc substreams.ResponseFunc,
	supportBuffering bool,
	logger *zap.Logger,
) error {

	if cursor.Block.Num() < cursor.LIB.Num() {
		// safeguard for a bug that "may" have been fixed in bstream library
		logger.Warn("cursor is invalid", zap.Uint64("clock_num", clock.Number), zap.String("cursor", cursor.String()))
		return fmt.Errorf("internal error 1203")
	}
	out := &pbsubstreamsrpc.BlockScopedData{
		Clock:             clock,
		Output:            mapModuleOutput,
		DebugMapOutputs:   extraMapModuleOutputs,
		DebugStoreOutputs: extraStoreModuleOutputs,
		Cursor:            normalizedOpaqueCursor(*cursor),
		FinalBlockHeight:  cursor.LIB.Num(),
	}

	if supportBuffering { //v4 support
		bsd := &pbsubstreamsrpcv4.BlockScopedDatas{
			Items: []*pbsubstreamsrpc.BlockScopedData{
				out,
			},
		}

		if err := respFunc(substreams.NewBlockScopedDatasResponse(bsd)); err != nil {
			return fmt.Errorf("calling return func: %w", err)
		}
		return nil
	}

	if err := respFunc(substreams.NewBlockScopedDataResponse(out)); err != nil {
		return fmt.Errorf("calling return func: %w", err)
	}

	return nil
}

func returnTier2DataOutputs(
	clock *pbsubstreams.Clock,
	data *anypb.Any,
	respFunc substreams.ResponseFunc,
) error {

	out := &pbssinternal.BlockScopedData{
		Clock:  clock,
		Output: data,
	}

	if err := respFunc(substreams.NewBlockScopedDataInternResponse(out)); err != nil {
		return fmt.Errorf("calling return func: %w", err)
	}

	return nil
}

func (p *Pipeline) renderWasmInputs(module *pbsubstreams.Module) (out []wasm.Argument, err error) {
	storeAccessor := p.stores.StoreMap
	for _, input := range module.Inputs {
		switch in := input.Input.(type) {

		case *pbsubstreams.Module_Input_Params_:
			out = append(out, wasm.NewParamsInput(input.GetParams().GetValue()))

		case *pbsubstreams.Module_Input_Map_:
			out = append(out, wasm.NewMapInput(in.Map.ModuleName, p.execGraph.ModulesInitBlocks()[in.Map.ModuleName]))

		case *pbsubstreams.Module_Input_Store_:
			inputName := input.GetStore().ModuleName
			if input.GetStore().Mode == pbsubstreams.Module_Input_Store_DELTAS {
				out = append(out, wasm.NewMapInput(inputName, p.execGraph.ModulesInitBlocks()[inputName]))
			} else {
				inputStore, found := storeAccessor.Get(inputName)
				if !found {
					return nil, fmt.Errorf("store %q not found", inputName)
				}
				out = append(out, wasm.NewStoreReaderInput(inputName, inputStore, p.execGraph.ModulesInitBlocks()[inputName]))
			}

		case *pbsubstreams.Module_Input_Source_:
			// in.Source.Type checking against `blockType` is already done
			// upfront in `validateGraph`.
			out = append(out, wasm.NewSourceInput(in.Source.Type, 0))

		case *pbsubstreams.Module_Input_FoundationalStore:
			identifier := in.FoundationalStore.GetIdentifier()
			clients, ok := p.foundationalClients[identifier]
			if !ok {
				endpoint, err := foundational_store.Lookup(p.foundationalEndpoints, identifier)
				if err != nil {
					return nil, fmt.Errorf("failed to connect remotely to foundational store with identifier %q, it's either not supported by this specific operator or you have a typo in your identifier: %w", identifier, err)
				}

				client, closer, err := foundational_store.NewStoreClient(endpoint.Address, endpoint.TLS, logging.Logger(p.ctx, zap.NewNop()))
				if err != nil {
					return nil, fmt.Errorf("failed to connect remotely to foundational store with identifier %q, it's either not supported by this specific operator or you have a typo in your identifier: %w", identifier, err)
				}

				clients = append(clients, client)
				p.foundationalClients[identifier] = clients
				p.foundationalClosers[identifier] = []func() error{closer}
			}
			out = append(out, wasm.NewFoundationalStoreInput(identifier, clients))

		default:
			return nil, fmt.Errorf("invalid input struct for module %q", module.Name)
		}
	}
	return out, nil
}

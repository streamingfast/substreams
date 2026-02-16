package pipeline

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime/debug"
	"strconv"

	"connectrpc.com/connect"
	"github.com/streamingfast/bstream"
	pbbstream "github.com/streamingfast/bstream/pb/sf/bstream/v1"
	"github.com/streamingfast/dmetering"
	"github.com/streamingfast/logging"
	"github.com/streamingfast/substreams/metering"
	"github.com/streamingfast/substreams/metrics"
	pbssinternal "github.com/streamingfast/substreams/pb/sf/substreams/intern/v2"
	pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/pipeline/exec"
	"github.com/streamingfast/substreams/reqctx"
	"github.com/streamingfast/substreams/storage/execout"
	"github.com/streamingfast/substreams/storage/store"
	"github.com/streamingfast/substreams/wasm"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var PrintStack = os.Getenv("SUBSTREAMS_PRINT_STACK") == "true" || os.Getenv("SUBSTREAMS_PRINT_STACK") == "1"

var biggestPartialBlockIndex = getBiggestPartialBlockIndex()

func getBiggestPartialBlockIndex() int32 {
	if env := os.Getenv("SUBSTREAMS_BIGGEST_PARTIAL_BLOCK_INDEX"); env != "" {
		if val, err := strconv.ParseInt(env, 10, 32); err == nil {
			return int32(val)
		}
	}
	return int32(10)
}

var ErrShuttingDown = errors.New("endpoint is shutting down, please reconnect")
var minBlocksProcessedToSave = uint64(25)

func (p *Pipeline) ProcessFromExecOutput(
	ctx context.Context,
	clock *pbsubstreams.Clock,
	cursor *bstream.Cursor,
) (err error) {

	p.gate.processBlock(clock.Number, bstream.StepNewIrreversible)
	execOutput, err := p.execOutputCache.NewBuffer(nil, clock, cursor)
	if err != nil {
		return fmt.Errorf("setting up exec output: %w", err)
	}

	if err = p.processBlock(ctx, execOutput, clock, cursor, bstream.StepNewIrreversible, nil, 0, false); err != nil {
		return err
	}

	return nil
}

func (p *Pipeline) ProcessBlock(block *pbbstream.Block, obj interface{}) (err error) {
	ctx := p.ctx

	metrics.BlockBeginProcess.Inc()
	defer metrics.BlockEndProcess.Inc()

	clock := BlockToClock(block)
	cursor := obj.(bstream.Cursorable).Cursor()
	step := obj.(bstream.Stepable).Step()
	if p.finalBlocksOnly && step == bstream.StepIrreversible {
		step = bstream.StepNewIrreversible // with finalBlocksOnly, we never get NEW signals so we fake any 'irreversible' signal as both
	}

	reorgJunctionBlock := obj.(bstream.Stepable).ReorgJunctionBlock()

	p.gate.processBlock(block.Number, step)
	execOutput, err := p.execOutputCache.NewBuffer(block, clock, cursor)
	if err != nil {
		return fmt.Errorf("setting up exec output: %w", err)
	}

	if err = p.processBlock(ctx, execOutput, clock, cursor, step, reorgJunctionBlock, block.PartialIndex, block.LastPartial); err != nil {
		return err // watch out, io.EOF needs to go through undecorated
	}

	reqctx.ReqStats(ctx).RecordBlock(block.AsRef())
	return
}

func BlockToClock(block *pbbstream.Block) *pbsubstreams.Clock {
	return &pbsubstreams.Clock{
		Number:    block.Number,
		Id:        block.Id,
		Timestamp: timestamppb.New(block.Time()),
	}
}

func blockRefToPB(block bstream.BlockRef) *pbsubstreams.BlockRef {
	return &pbsubstreams.BlockRef{
		Number: block.Num(),
		Id:     block.ID(),
	}
}

func (p *Pipeline) processBlock(
	ctx context.Context,
	execOutput execout.ExecutionOutput,
	clock *pbsubstreams.Clock,
	cursor *bstream.Cursor,
	step bstream.StepType,
	reorgJunctionBlock bstream.BlockRef,
	partialIndex int32,
	isLastPartial bool,
) (err error) {
	var eof bool

	switch step {
	case bstream.StepPartial:
		p.blockStepMap[bstream.StepPartial]++
		if reqctx.PartialBlocks(ctx) {
			if err := p.handleStepPartial(ctx, clock, cursor, execOutput, partialIndex, isLastPartial); err != nil {
				return fmt.Errorf("step partial: %w", err)
			}
		}

	case bstream.StepUndoPartial:
		if reqctx.PartialBlocks(ctx) {
			p.blockStepMap[bstream.StepUndoPartial]++
			if err := p.handleStepUndoPartial(cursor); err != nil {
				return err
			}
		}

	case bstream.StepUndo:
		p.blockStepMap[bstream.StepUndo]++
		if err := p.handleStepUndo(clock, cursor, reorgJunctionBlock); err != nil {
			return fmt.Errorf("step undo: %w", err)
		}
	case bstream.StepStalled:
		p.blockStepMap[bstream.StepStalled]++
		if err := p.handleStepStalled(clock); err != nil {
			return fmt.Errorf("step stalled: %w", err)
		}
	case bstream.StepNew:
		p.blockStepMap[bstream.StepNew]++

		// legacy metering
		//todo: (deprecated)
		dmetering.GetBytesMeter(ctx).AddBytesRead(execOutput.Len())

		err = p.handleStepNew(ctx, clock, cursor, execOutput, false)
		if err != nil {
			if err == io.EOF {
				eof = true
			} else {
				return err
			}
		}
	case bstream.StepNewIrreversible:
		p.blockStepMap[bstream.StepNewIrreversible]++
		err = p.handleStepNew(ctx, clock, cursor, execOutput, true)
		if err != nil && err != io.EOF {
			return err
		}
		if err == io.EOF {
			eof = true
		}
		err = p.handleStepFinal(clock)
		if err != nil {
			return err
		}
	case bstream.StepIrreversible:
		p.blockStepMap[bstream.StepIrreversible]++
		err = p.handleStepFinal(clock)
		if err != nil {
			return err
		}
	}

	if clock.Number%500 == 0 {
		logger := reqctx.Logger(ctx)
		// log the total number of StepNew and StepNewIrreversible blocks, and the ratio of the two
		logger.Debug("block stats",
			zap.Uint64("block_num", clock.Number),
			zap.Uint64("step_new", p.blockStepMap[bstream.StepNew]),
			zap.Uint64("step_new_irreversible", p.blockStepMap[bstream.StepNewIrreversible]),
			zap.Float64("ratio", float64(p.blockStepMap[bstream.StepNewIrreversible])/float64(p.blockStepMap[bstream.StepNew])),
		)
	}

	if eof {
		return io.EOF
	}
	return nil
}

func (p *Pipeline) handleStepStalled(clock *pbsubstreams.Clock) error {
	p.execOutputCache.HandleStalled(clock)
	p.forkHandler.removeReversibleOutput(clock.Id)
	return nil
}

// undoPartialStates reverts all partial states from stores in reverse order.
// It clears the partial processing state but does NOT send any undo signal to the user.
func (p *Pipeline) undoPartialStates() error {
	if p.partialProcessingState == nil {
		return nil
	}

	for i := len(p.partialProcessingState.processedPartials) - 1; i >= 0; i-- {
		clk := p.partialProcessingState.processedPartials[i]
		if err := p.forkHandler.handleUndo(clk); err != nil {
			return fmt.Errorf("reverting outputs: %w", err)
		}
	}

	p.partialProcessingState = nil
	return nil
}

// handleUndo reverts a block's outputs and sends an undo signal to the user.
func (p *Pipeline) handleUndo(clock *pbsubstreams.Clock, cursor *bstream.Cursor, reorgJunctionBlock bstream.BlockRef) error {
	if err := p.forkHandler.handleUndo(clock); err != nil {
		return fmt.Errorf("reverting outputs: %w", err)
	}

	if bstream.EqualsBlockRefs(p.insideReorgUpTo, reorgJunctionBlock) {
		return nil
	}
	p.insideReorgUpTo = reorgJunctionBlock

	targetCursor := &bstream.Cursor{
		Step:      bstream.StepNew,
		Block:     reorgJunctionBlock,
		LIB:       cursor.LIB,
		HeadBlock: cursor.HeadBlock,
	}

	targetClock := blockRefToPB(reorgJunctionBlock)

	p.lastProcessedBlockRef = reorgJunctionBlock
	p.lastCursor = targetCursor
	return p.respFunc(
		&pbsubstreamsrpc.Response{
			Message: &pbsubstreamsrpc.Response_BlockUndoSignal{
				BlockUndoSignal: &pbsubstreamsrpc.BlockUndoSignal{
					LastValidBlock:  targetClock,
					LastValidCursor: targetCursor.ToOpaque(),
				},
			},
		})
}

// handleStepUndo handles a regular undo step: undoes any pending partial states,
// then reverts the block and sends an undo signal to the user.
func (p *Pipeline) handleStepUndo(clock *pbsubstreams.Clock, cursor *bstream.Cursor, reorgJunctionBlock bstream.BlockRef) error {

	if p.previousLastPartialBlock != nil && p.previousLastPartialBlock.Num() > reorgJunctionBlock.Num() {
		p.previousLastPartialBlock = nil // remove assumptions about an 'undone' previous partial block
	}
	if err := p.undoPartialStates(); err != nil {
		return err
	}
	return p.handleUndo(clock, cursor, reorgJunctionBlock)
}

// handleStepUndoPartial handles an undo specifically for partial blocks.
// It undoes partial states and sends an undo signal to the user.
func (p *Pipeline) handleStepUndoPartial(cursor *bstream.Cursor) error {
	if p.partialProcessingState == nil {
		return nil
	}

	clock := &pbsubstreams.Clock{
		Id:     p.partialProcessingState.lastBlockID,
		Number: p.partialProcessingState.num,
	}

	targetCursor := &bstream.Cursor{
		Step:      bstream.StepUndo,
		Block:     cursor.Block,
		HeadBlock: cursor.HeadBlock,
		LIB:       cursor.LIB,
	}

	reorgJunctionBlock := p.partialProcessingState.previousBlockRef

	if err := p.undoPartialStates(); err != nil {
		return err
	}

	return p.handleUndo(clock, targetCursor, reorgJunctionBlock)
}

func (p *Pipeline) handleStepFinal(clock *pbsubstreams.Clock) error {
	p.lastFinalClock = clock
	p.insideReorgUpTo = nil
	if err := p.execOutputCache.HandleFinal(clock); err != nil {
		return fmt.Errorf("exec output cache: handle final: %w", err)
	}
	p.forkHandler.removeReversibleOutput(clock.Id)
	return nil
}

func (p *Pipeline) handleStepPartial(ctx context.Context, clock *pbsubstreams.Clock, cursor *bstream.Cursor, execOutput execout.ExecutionOutput, idx int32, isLast bool) (err error) {
	if p.partialProcessingState != nil {
		if clock.Id == p.partialProcessingState.lastBlockID && !isLast {
			return nil // same hash: nothing new to process. We only send this 'empty' partial block if it is the last
		}
		if clock.Number == p.partialProcessingState.num {
			// new partial for same block number: normal behavior
			p.partialProcessingState.highestIndex = idx
		} else {
			reqctx.Logger(ctx).Warn("received partial after another non-last partial with different block numbers (without step_Undo) -- this should not happen unless you are running a chain with partial blocks AND skipped blocks", zap.Uint64("received", clock.Number), zap.Uint64("expected", p.partialProcessingState.num))
			if err := p.handleStepUndoPartial(cursor); err != nil {
				return err
			}
			// now that we have undone previous partials, we start a new one
			p.partialProcessingState = newPartialProcessingState(clock.Number, clock.Id, idx, cursor.HeadBlock) //on StepPartial, the cursor.headBlock is the parent of the partial
		}
	} else {
		p.partialProcessingState = newPartialProcessingState(clock.Number, clock.Id, idx, cursor.HeadBlock)
	}

	txCount, txsHash, err := splitPartialblock(execOutput, p.blockType, p.partialProcessingState.processedTransactionsCount, p.partialProcessingState.processedTransactionsHash)
	if err != nil {
		// an error occurred, partial blocks do not contain what they should...
		return err
	}

	reqDetails := reqctx.Details(ctx)
	if isBlockOverStopBlock(clock.Number, reqDetails.StopBlockNum) {
		return nil
	}
	if p.isTier1 && p.checkPendingShutdown() {
		return nil
	}
	if p.pendingUndoMessage != nil {
		return nil
	}
	if reqDetails.IsTier2Request {
		panic("tier2 requests should never receive partial block")
	}

	if err := p.executeModules(ctx, execOutput, false, false); err != nil {
		return fmt.Errorf("execute modules: %w", err)
	}

	if isLast {
		// allow an 'undo' on this 'last' partial block to undo the whole thing -- must be run AFTER the executeModules()
		// important because we will lose all information on the partialProcessingState
		p.forkHandler.joinReversibleOutputs(clock, p.partialProcessingState.processedPartials)
		p.partialProcessingState = nil
		p.previousLastPartialBlock = bstream.NewBlockRef(clock.Id, clock.Number)
	} else {
		p.partialProcessingState.processedTransactionsHash = txsHash
		p.partialProcessingState.processedTransactionsCount = txCount
		p.partialProcessingState.lastBlockID = clock.Id
		p.partialProcessingState.processedPartials = append(p.partialProcessingState.processedPartials, clock)
	}

	if !p.gate.shouldSendOutputs() {
		return nil
	}

	mapModuleOutput := normalizeModuleOutput(p.mapModuleOutput, reqDetails.OutputModule)

	if err = returnPartialDataOutput(clock, cursor, mapModuleOutput, p.respFunc, uint32(idx), isLast); err != nil {
		return fmt.Errorf("failed to return module data output: %w", err)
	}

	return nil
}

func normalizeModuleOutput(in *pbsubstreamsrpc.MapModuleOutput, outputModule string) *pbsubstreamsrpc.MapModuleOutput {
	if in == nil {
		return &pbsubstreamsrpc.MapModuleOutput{
			Name:      outputModule,
			MapOutput: &anypb.Any{},
		}
	}
	if in.Name == "" {
		in.Name = outputModule
	}
	return in
}

func (p *Pipeline) handleStepNew(ctx context.Context, clock *pbsubstreams.Clock, cursor *bstream.Cursor, execOutput execout.ExecutionOutput, isFinalBlock bool) (err error) {

	if p.isTier1 && p.checkPendingShutdown() {
		if p.stores.StoreMap != nil && p.sentBlocks > minBlocksProcessedToSave {
			p.stores.logger.Info("shutting down, quick saving stores")
			if err := p.undoPartialStates(); err != nil {
				return err
			}
			p.stores.StoreMap.QuickSave(ctx, p.lastProcessedBlockRef.ID())
		} else {
			p.stores.logger.Info("shutting down (no quick save)", zap.Bool("has_stores", p.stores.StoreMap != nil), zap.Uint64("sent_blocks", p.sentBlocks))
		}
		return ErrShuttingDown
	}

	p.insideReorgUpTo = nil
	reqDetails := reqctx.Details(ctx)

	if p.respFunc != nil {
		defer func() {
			forceSend := (clock.Number+1)%p.stateBundleSize == 0 || err != nil
			var sendError error
			if reqDetails.IsTier2Request {
				sendError = p.returnInternalModuleProgressOutputs(clock, forceSend)
			} else {
				sendError = p.returnRPCModuleProgressOutputs(forceSend)
			}
			if err == nil {
				err = sendError
			}
		}()
	}

	if isBlockOverStopBlock(clock.Number, reqDetails.StopBlockNum) {
		return io.EOF
	}

	// note: if we start on a forked cursor, the undo signal will appear BEFORE we send the snapshot
	if p.gate.shouldSendSnapshot() && !reqDetails.IsTier2Request {
		if err := p.sendSnapshots(p.stores.StoreMap, reqDetails.DebugInitialStoreSnapshotForModules); err != nil {
			return fmt.Errorf("send initial snapshots: %w", err)
		}
	}

	logger := reqctx.Logger(ctx)

	if err := p.runPreBlockHooks(ctx, clock); err != nil {
		return fmt.Errorf("pre block hook: %w", err)
	}

	metering.AddWasmInputBytes(ctx, execOutput.Len())

	if p.pendingUndoMessage != nil && p.gate.shouldSendOutputs() {
		if err := p.respFunc(p.pendingUndoMessage); err != nil {
			return fmt.Errorf("failed to run send pending undo message: %w", err)
		}
		p.pendingUndoMessage = nil
	}

	withPartialBlocks := reqctx.PartialBlocks(ctx)

	if withPartialBlocks {

		if p.previousLastPartialBlock != nil {
			if clock.Number <= p.previousLastPartialBlock.Num() {

				// extra check, but forkDB should prevent this
				if clock.Number == p.previousLastPartialBlock.Num() && p.previousLastPartialBlock.ID() != clock.Id {
					return fmt.Errorf("received new block with the same number as the previous last partial block (%d), but different ID (%q, expected %q). expected an UNDO", clock.Number, clock.Id, p.previousLastPartialBlock.ID())
				}

				// all good, this 'new' block can be ignored, it is older than our previous last partial block
				p.lastProcessedBlockRef = bstream.NewBlockRef(clock.Id, clock.Number)
				p.lastCursor = cursor
				return nil
			}
		}

		if p.partialProcessingState != nil {
			prevLastPartial := "nil"
			if p.previousLastPartialBlock != nil {
				prevLastPartial = p.previousLastPartialBlock.String()
			}
			logger.Info("receiving new block with an active partial state, but previousLastPartialBlock is not the same as the current block. Undoing partial state",
				zap.String("previous last partial", prevLastPartial), zap.Stringer("current new block", clock))
			if err := p.handleStepUndoPartial(cursor); err != nil {
				return fmt.Errorf("failed to undo partial states: %w", err)
			}
		}
	}

	if err := p.executeModules(ctx, execOutput, isFinalBlock, true); err != nil {
		return fmt.Errorf("execute modules: %w", err)
	}

	if p.gate.shouldSendOutputs() {
		if reqDetails.IsTier2Request { // the gate.shouldSendOutputs() assures us that we are a streaming tier2 in this case
			if out := p.mapModuleOutput.GetMapOutput(); out != nil {
				skippable := p.mapModuleOutputSkippable && len(out.Value) == 0
				if !skippable {
					p.sentBlocks++
					if err = returnTier2DataOutputs(clock, out, p.respFunc); err != nil {
						return fmt.Errorf("failed to return module data output: %w", err)
					}
				}
			}
		} else {
			// LIVE and DEV mode always receive module data outputs, even when they are empty
			// so they can follow progress (and dev also gets debug output...)
			//

			if p.partialProcessingState == nil {
				mapModuleOutput := normalizeModuleOutput(p.mapModuleOutput, reqDetails.OutputModule)
				p.sentBlocks++
				if err = returnModuleDataOutputs(clock, cursor, mapModuleOutput, p.extraMapModuleOutputs, p.extraStoreModuleOutputs, p.respFunc, logger); err != nil {
					return fmt.Errorf("failed to return module data output: %w", err)
				}
			}
		}
	}

	p.stores.resetStores()
	logger.Debug("block processed", zap.Uint64("block_num", clock.Number))
	p.lastProcessedBlockRef = bstream.NewBlockRef(clock.Id, clock.Number)
	p.lastCursor = cursor
	return nil
}

func (p *Pipeline) executeModules(ctx context.Context, execOutput execout.ExecutionOutput, isFinalBlock bool, cachable bool) (err error) {
	ctx, span := reqctx.WithModuleExecutionSpan(ctx, "modules_executions")
	defer span.EndWithErr(&err)

	p.mapModuleOutput = nil
	p.extraMapModuleOutputs = nil
	p.extraStoreModuleOutputs = nil
	blockNum := execOutput.Clock().Number

	// they may be already built, but we call this function every time to enable future dynamic changes
	if err := p.BuildModuleExecutors(ctx); err != nil {
		return fmt.Errorf("building wasm module tree: %w", err)
	}

	// the ctx is cached in the built moduleExecutors so we only activate timeout here
	ctx, cancel := context.WithTimeout(ctx, p.executionTimeout)
	defer func() {
		if r := recover(); r != nil {
			err = recoverExecutionPanic(ctx, err, r, execOutput.Clock().AsBlockRef())
		}
		cancel()
	}()

	maxParallelExecutor := reqctx.MaxStageLayerParallelExecutor(ctx)
	logging.Logger(ctx, p.stores.logger).Debug("executing stage's layers", zap.Int("layer_count", len(p.StagedModuleExecutors)), zap.Uint64("max_parallel_executor", maxParallelExecutor))

	executedStages := make(map[int]bool)
	for _, layer := range p.StagedModuleExecutors {
		if maxParallelExecutor <= 1 || len(layer) <= 1 {
			for _, executor := range layer {
				if !executor.RunsOnBlock(blockNum) {
					continue
				}
				res := p.execute(ctx, executor, execOutput, isFinalBlock, cachable)
				if res.output != nil && !res.skippedExecution && !res.output.Cached {
					executedStages[p.moduleNameToStage[res.output.ModuleName]] = true
				}
				if err := p.applyExecutionResult(ctx, executor, res, execOutput); err != nil {
					return fmt.Errorf("applying executor results %q on block %d (%s): %w", executor.Name(), blockNum, execOutput.Clock().Id, res.err)
				}
			}
		} else {
			results := make([]resultObj, len(layer))
			wg := errgroup.Group{}
			wg.SetLimit(int(maxParallelExecutor))

			for i, executor := range layer {
				if !executor.RunsOnBlock(execOutput.Clock().Number) {
					results[i] = resultObj{notRunnable: true}
					continue
				}

				wg.Go(func() (err error) {
					defer func() {
						// each go thread needs its own recover(), as the WASM execution can panic
						if r := recover(); r != nil {
							err = recoverExecutionPanic(ctx, err, r, execOutput.Clock().AsBlockRef())
						}
					}()

					res := p.execute(ctx, executor, execOutput, isFinalBlock, cachable)
					results[i] = res

					return
				})
			}

			if err := wg.Wait(); err != nil {
				return fmt.Errorf("running executors: %w", err)
			}

			for i, result := range results {
				if result.notRunnable {
					continue
				}
				if result.output != nil && !result.skippedExecution && !result.output.Cached {
					executedStages[p.moduleNameToStage[result.output.ModuleName]] = true
				}
				executor := layer[i]
				if result.err != nil {
					return fmt.Errorf("running executor %q: %w", executor.Name(), result.err)
				}

				if err := p.applyExecutionResult(ctx, executor, result, execOutput); err != nil {
					return fmt.Errorf("applying executor results %q on block %s: %w", executor.Name(), execOutput.Clock().AsBlockRef(), result.err)
				}
			}
		}
	}
	metering.AddProcessedBlocks(ctx, len(executedStages)) // blocks are counted on every stage for which they were executed (not skipped for indexes, not loaded from existing cache)
	reqctx.ReqStats(ctx).RecordBlocksProcessed(uint64(len(executedStages)))

	return nil
}

// recoverExecutionPanic performs the recovery logic if a `recover() != nil` condition happened. The `recover()` must always
// be called at the goroutine location, then the recovered value must be passed to this handler.
//
// The executionError is the "normal" non-recovered error that happened during execution of the execution engine, usually
// a WASM virtual machine.
func recoverExecutionPanic(ctx context.Context, executionError error, recovered any, blockRef bstream.BlockRef) error {
	var recoveredErr error
	if e, ok := recovered.(error); ok {
		recoveredErr = e
	} else {
		recoveredErr = fmt.Errorf("%v", recovered)
	}

	// If recovering from context cancel, simply return the execution error as-is
	if ctx.Err() != nil || errors.Is(recoveredErr, context.Canceled) {
		return executionError
	}

	// If the context deadline was exceeded, return a deadline exceeded error RPC error
	if ctx.Err() == context.DeadlineExceeded {
		return connect.NewError(connect.CodeDeadlineExceeded, fmt.Errorf("execution timed out at block %s: %w", blockRef, recoveredErr))
	}

	// Otherwise, log the panic and return a generic error
	if PrintStack {
		debug.PrintStack()
	}

	// Also log error with stacktrace for easier debugging, the message contains also the block, for easy logs viewing
	reqctx.Logger(ctx).Error(fmt.Sprintf("panic at block %s", blockRef), zap.Stringer("block", blockRef), zap.Error(recoveredErr), zap.Stack("stacktrace"))

	return fmt.Errorf("panic at block %s: %w", blockRef, recoveredErr)
}

type resultObj struct {
	output           *pbssinternal.ModuleOutput
	bytes            []byte
	bytesForFiles    []byte
	err              error
	notRunnable      bool
	skippedExecution bool
	skippableOutput  bool
}

func (p *Pipeline) execute(ctx context.Context, executor exec.ModuleExecutor, execOutput execout.ExecutionOutput, isFinalBlock bool, cachable bool) (out resultObj) {
	logger := reqctx.Logger(ctx)

	executorName := executor.Name()
	logger.Debug("executing", zap.Uint64("block", execOutput.Clock().Number), zap.String("module_name", executorName))

	defer func() {
		if r := recover(); r != nil {
			var recoveredErr error
			if err, ok := r.(error); ok {
				recoveredErr = err
			} else {
				recoveredErr = fmt.Errorf("%v", r)
			}

			if errors.Is(recoveredErr, wasm.ErrWasmDeterministicExec) || errors.Is(recoveredErr, store.ErrStoreAboveMaxSize) {
				p.execoutStorage.ConfigMap[executorName].WriteDeterministicError(ctx, execOutput.Clock().Number, fmt.Errorf("%w (deterministic error)", recoveredErr))
				out.err = recoveredErr
				return
			}

			// Those are not deterministic errors, just propagate them up for now, but the context is probably cancelled at this point
			if errors.Is(recoveredErr, wasm.ErrFoundationalStoreCanceled) {
				out.err = recoveredErr
				return
			}

			// Move the panic up so it's handled at a higher level by those who call execute()
			panic(fmt.Errorf("unknown error: %s", r))
		}
	}()

	moduleOutput, outputBytes, outputBytesFiles, skippedExecution, skippableOutput, runError := exec.RunModule(ctx, executor, execOutput, cachable)

	if isFinalBlock && errors.Is(runError, wasm.ErrWasmDeterministicExec) {
		p.execoutStorage.ConfigMap[executorName].WriteDeterministicError(ctx, execOutput.Clock().Number, runError)
	}

	return resultObj{
		output:           moduleOutput,
		bytes:            outputBytes,
		bytesForFiles:    outputBytesFiles,
		err:              runError,
		notRunnable:      false,
		skippedExecution: skippedExecution,
		skippableOutput:  skippableOutput,
	}
}

func (p *Pipeline) applyExecutionResult(ctx context.Context, executor exec.ModuleExecutor, res resultObj, execOutput execout.ExecutionOutput) (err error) {
	executorName := executor.Name()

	moduleOutput, outputBytes, runError := res.output, res.bytes, res.err
	if runError != nil {
		return fmt.Errorf("execute module: %w", runError)
	}

	if executor.HasValidOutput() {
		p.saveModuleOutput(moduleOutput, executor.Name(), reqctx.Details(ctx).ProductionMode, res.skippableOutput)
	}

	skip_output := res.skippableOutput

	if !skip_output && executor.HasValidOutput() {
		if err := execOutput.Set(executorName, outputBytes); err != nil {
			return fmt.Errorf("set output cache: %w", err)
		}
		if moduleOutput != nil {
			p.forkHandler.addReversibleOutput(moduleOutput, execOutput.Clock().Id)
		}
	}

	if !skip_output && executor.HasOutputForFiles() {
		if err := execOutput.SetFileOutput(executorName, res.bytesForFiles); err != nil {
			return fmt.Errorf("set output cache: %w", err)
		}
	}

	return nil
}

// this will be sent to the requestor
func (p *Pipeline) saveModuleOutput(output *pbssinternal.ModuleOutput, moduleName string, isProduction bool, skippable bool) {
	if p.isOutputModule(moduleName) {
		p.mapModuleOutput = toRPCMapModuleOutputs(output)
		p.mapModuleOutputSkippable = skippable
		return
	}
	if isProduction {
		return
	}

	if storeOutputs := toRPCStoreModuleOutputs(output); storeOutputs != nil {
		p.extraStoreModuleOutputs = append(p.extraStoreModuleOutputs, storeOutputs)
	}

	if mapOutput := toRPCMapModuleOutputs(output); mapOutput != nil {
		p.extraMapModuleOutputs = append(p.extraMapModuleOutputs, mapOutput)
	}
}

package pipeline

import (
	"context"
	"errors"
	"fmt"
	"io"
	"runtime/debug"
	"sync"

	"github.com/streamingfast/bstream"
	pbbstream "github.com/streamingfast/bstream/pb/sf/bstream/v1"
	"github.com/streamingfast/dmetering"
	"github.com/streamingfast/substreams/metering"
	"github.com/streamingfast/substreams/metrics"
	pbssinternal "github.com/streamingfast/substreams/pb/sf/substreams/intern/v2"
	pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/pipeline/exec"
	"github.com/streamingfast/substreams/reqctx"
	"github.com/streamingfast/substreams/storage/execout"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

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

	if err = p.processBlock(ctx, execOutput, clock, cursor, bstream.StepNewIrreversible, nil); err != nil {
		return err
	}

	return nil
}

func (p *Pipeline) ProcessBlock(block *pbbstream.Block, obj interface{}) (err error) {
	ctx := p.ctx

	logger := reqctx.Logger(ctx)
	defer func() {
		if r := recover(); r != nil {
			if err, ok := r.(error); ok {
				if errors.Is(err, context.Canceled) {
					return
				}
			}
			truncatedBlock := block.AsRef().String()
			err = fmt.Errorf("panic at block %d: %s [%s]", block.Number, r, truncatedBlock)
			logger.Warn("panic while process block", zap.Uint64("block_num", block.Number), zap.Error(err))
			logger.Debug(string(debug.Stack())) // there are known panic cases, we don't want them in error logs
		}
	}()

	metrics.BlockBeginProcess.Inc()
	defer metrics.BlockEndProcess.Inc()

	clock := BlockToClock(block)
	cursor := obj.(bstream.Cursorable).Cursor()
	step := obj.(bstream.Stepable).Step()
	if p.finalBlocksOnly && step == bstream.StepIrreversible {
		step = bstream.StepNewIrreversible // with finalBlocksOnly, we never get NEW signals so we fake any 'irreversible' signal as both
	}

	reorgJunctionBlock := obj.(bstream.Stepable).ReorgJunctionBlock()

	reqctx.ReqStats(ctx).RecordBlock(block.AsRef())
	p.gate.processBlock(block.Number, step)
	execOutput, err := p.execOutputCache.NewBuffer(block, clock, cursor)
	if err != nil {
		return fmt.Errorf("setting up exec output: %w", err)
	}

	if err = p.processBlock(ctx, execOutput, clock, cursor, step, reorgJunctionBlock); err != nil {
		return err // watch out, io.EOF needs to go through undecorated
	}
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
) (err error) {
	var eof bool

	switch step {
	case bstream.StepUndo:
		p.blockStepMap[bstream.StepUndo]++
		if err = p.handleStepUndo(clock, cursor, reorgJunctionBlock); err != nil {
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

		err = p.handleStepNew(ctx, clock, cursor, execOutput)
		if err != nil && err != io.EOF {
			return err
		}
		if err == io.EOF {
			eof = true
		}
	case bstream.StepNewIrreversible:
		p.blockStepMap[bstream.StepNewIrreversible]++
		err = p.handleStepNew(ctx, clock, cursor, execOutput)
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

func (p *Pipeline) handleStepUndo(clock *pbsubstreams.Clock, cursor *bstream.Cursor, reorgJunctionBlock bstream.BlockRef) error {

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

func (p *Pipeline) handleStepFinal(clock *pbsubstreams.Clock) error {
	p.lastFinalClock = clock
	p.insideReorgUpTo = nil
	if err := p.execOutputCache.HandleFinal(clock); err != nil {
		return fmt.Errorf("exec output cache: handle final: %w", err)
	}
	p.forkHandler.removeReversibleOutput(clock.Id)
	return nil
}

func (p *Pipeline) handleStepNew(ctx context.Context, clock *pbsubstreams.Clock, cursor *bstream.Cursor, execOutput execout.ExecutionOutput) (err error) {
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

	if err := p.executeModules(ctx, execOutput); err != nil {
		return fmt.Errorf("execute modules: %w", err)
	}

	if p.gate.shouldSendOutputs() {
		logger.Debug("will return module outputs")
		if p.pendingUndoMessage != nil {
			if err := p.respFunc(p.pendingUndoMessage); err != nil {
				return fmt.Errorf("failed to run send pending undo message: %w", err)
			}
		}
		p.pendingUndoMessage = nil

		// LIVE and DEV mode always receive module data outputs, even when they are empty
		// so they can follow progress (and dev also gets debug output...)
		mapModuleOutput := p.mapModuleOutput
		if mapModuleOutput == nil && !reqDetails.IsTier2Request {
			mapModuleOutput = &pbsubstreamsrpc.MapModuleOutput{
				Name:      reqDetails.OutputModule,
				MapOutput: &anypb.Any{},
			}
		}
		if err = returnModuleDataOutputs(clock, cursor, mapModuleOutput, p.extraMapModuleOutputs, p.extraStoreModuleOutputs, p.respFunc, logger); err != nil {
			return fmt.Errorf("failed to return module data output: %w", err)
		}
	}

	p.stores.resetStores()
	logger.Debug("block processed", zap.Uint64("block_num", clock.Number))
	return nil
}

func (p *Pipeline) executeModules(ctx context.Context, execOutput execout.ExecutionOutput) (err error) {
	ctx, span := reqctx.WithModuleExecutionSpan(ctx, "modules_executions")
	defer span.EndWithErr(&err)

	p.mapModuleOutput = nil
	p.extraMapModuleOutputs = nil
	p.extraStoreModuleOutputs = nil
	blockNum := execOutput.Clock().Number
	block := fmt.Sprintf("%d (%s)", blockNum, execOutput.Clock().Id)

	// they may be already built, but we call this function every time to enable future dynamic changes
	if err := p.BuildModuleExecutors(ctx); err != nil {
		return fmt.Errorf("building wasm module tree: %w", err)
	}

	// the ctx is cached in the built moduleExecutors so we only activate timeout here
	ctx, cancel := context.WithTimeout(ctx, p.executionTimeout)
	defer cancel()
	for _, stage := range p.ModuleExecutors {
		//t0 := time.Now()
		if len(stage) < 2 {
			//fmt.Println("Linear stage", len(stage))
			for _, executor := range stage {
				if !executor.RunsOnBlock(blockNum) {
					continue
				}
				res := p.execute(ctx, executor, execOutput)
				if err := p.applyExecutionResult(ctx, executor, res, execOutput); err != nil {
					return fmt.Errorf("applying executor results %q on block %s: %w", executor.Name(), block, res.err)
				}
			}
		} else {
			results := make([]resultObj, len(stage))
			wg := sync.WaitGroup{}
			//fmt.Println("Parallelized in stage", stageIdx, len(stage))
			for i, executor := range stage {
				if !executor.RunsOnBlock(execOutput.Clock().Number) {
					results[i] = resultObj{not_runnable: true}
					continue
				}
				wg.Add(1)
				i := i
				executor := executor
				go func() {
					defer wg.Done()
					res := p.execute(ctx, executor, execOutput)
					results[i] = res
				}()
			}
			wg.Wait()

			for i, result := range results {
				if result.not_runnable {
					continue
				}
				executor := stage[i]
				if result.err != nil {
					//p.returnFailureProgress(ctx, err, executor)
					return fmt.Errorf("running executor %q: %w", executor.Name(), result.err)
				}
				if err := p.applyExecutionResult(ctx, executor, result, execOutput); err != nil {
					return fmt.Errorf("applying executor results %q on block %s: %w", executor.Name(), block, result.err)
				}
			}
		}
		//blockDuration += time.Since(t0)
	}

	return nil
}

type resultObj struct {
	output         *pbssinternal.ModuleOutput
	bytes          []byte
	bytesForFiles  []byte
	err            error
	not_runnable   bool
	skipped_output bool
}

func (p *Pipeline) execute(ctx context.Context, executor exec.ModuleExecutor, execOutput execout.ExecutionOutput) resultObj {
	logger := reqctx.Logger(ctx)

	executorName := executor.Name()
	logger.Debug("executing", zap.Uint64("block", execOutput.Clock().Number), zap.String("module_name", executorName))

	moduleOutput, outputBytes, outputBytesFiles, skipped, runError := exec.RunModule(ctx, executor, execOutput)

	return resultObj{
		output:         moduleOutput,
		bytes:          outputBytes,
		bytesForFiles:  outputBytesFiles,
		err:            runError,
		not_runnable:   false,
		skipped_output: skipped,
	}
}

func (p *Pipeline) applyExecutionResult(ctx context.Context, executor exec.ModuleExecutor, res resultObj, execOutput execout.ExecutionOutput) (err error) {
	executorName := executor.Name()

	moduleOutput, outputBytes, runError := res.output, res.bytes, res.err
	if runError != nil {
		return fmt.Errorf("execute module: %w", runError)
	}

	if executor.HasValidOutput() {
		p.saveModuleOutput(moduleOutput, executor.Name(), reqctx.Details(ctx).ProductionMode)
	}

	skip_output := res.skipped_output

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
func (p *Pipeline) saveModuleOutput(output *pbssinternal.ModuleOutput, moduleName string, isProduction bool) {
	if p.isOutputModule(moduleName) {
		p.mapModuleOutput = toRPCMapModuleOutputs(output)
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

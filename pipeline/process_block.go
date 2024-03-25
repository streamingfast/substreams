package pipeline

import (
	"context"
	"errors"
	"fmt"
	"io"
	"runtime/debug"
	"sync"

	pbbstream "github.com/streamingfast/bstream/pb/sf/bstream/v1"
	"github.com/streamingfast/dmetering"

	"github.com/streamingfast/bstream"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/streamingfast/substreams/metrics"
	pbssinternal "github.com/streamingfast/substreams/pb/sf/substreams/intern/v2"
	pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/pipeline/exec"
	"github.com/streamingfast/substreams/reqctx"
	"github.com/streamingfast/substreams/storage/execout"
	"github.com/streamingfast/substreams/storage/store"
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
			err = fmt.Errorf("panic at block %s: %s", block, r)
			logger.Error("panic while process block", zap.Uint64("block_num", block.Number), zap.Error(err))
			logger.Error(string(debug.Stack()))
		}
	}()

	metrics.BlockBeginProcess.Inc()
	defer metrics.BlockEndProcess.Inc()

	clock := blockToClock(block)
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

func blockToClock(block *pbbstream.Block) *pbsubstreams.Clock {
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
		if err = p.handleStepUndo(ctx, clock, cursor, reorgJunctionBlock); err != nil {
			return fmt.Errorf("step undo: %w", err)
		}
	case bstream.StepStalled:
		p.blockStepMap[bstream.StepStalled]++
		if err := p.handleStepStalled(clock); err != nil {
			return fmt.Errorf("step stalled: %w", err)
		}
	case bstream.StepNew:
		p.blockStepMap[bstream.StepNew]++

		dmetering.GetBytesMeter(ctx).AddBytesRead(execOutput.Len())
		err = p.handleStepNew(ctx, clock, cursor, execOutput)
		if err != nil && err != io.EOF {
			return fmt.Errorf("step new: handler step new: %w", err)
		}
		if err == io.EOF {
			eof = true
		}
	case bstream.StepNewIrreversible:
		p.blockStepMap[bstream.StepNewIrreversible]++
		err = p.handleStepNew(ctx, clock, cursor, execOutput)
		if err != nil && err != io.EOF {
			return fmt.Errorf("step new irr: handler step new: %w", err)
		}
		if err == io.EOF {
			eof = true
		}
		err = p.handleStepFinal(clock)
		if err != nil {
			return fmt.Errorf("handling step irreversible: %w", err)
		}
	case bstream.StepIrreversible:
		p.blockStepMap[bstream.StepIrreversible]++
		err = p.handleStepFinal(clock)
		if err != nil {
			return fmt.Errorf("handling step irreversible: %w", err)
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

func (p *Pipeline) handleStepUndo(ctx context.Context, clock *pbsubstreams.Clock, cursor *bstream.Cursor, reorgJunctionBlock bstream.BlockRef) error {

	if err := p.forkHandler.handleUndo(clock, cursor); err != nil {
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
			forceSend := (clock.Number+1)%p.runtimeConfig.StateBundleSize == 0 || err != nil
			var sendError error
			if reqDetails.IsTier2Request {
				sendError = p.returnInternalModuleProgressOutputs(clock, forceSend)
			} else {
				sendError = p.returnRPCModuleProgressOutputs(clock, forceSend)
			}
			if err == nil {
				err = sendError
			}
		}()
	}

	if isBlockOverStopBlock(clock.Number, reqDetails.StopBlockNum) {
		return io.EOF
	}

	// FIXME: when handling the real-time segment, it's dangerous
	// to save the stores, as they might have components that get
	// reverted, and we won't go change the stores then.
	// So we _shouldn't_ save the stores unless we're in irreversible-only
	// mode. Basically, tier1 shouldn't save unless it's a StepNewIrreversible
	// (we're in a historical segment)
	// When we're in the real-time segment, we shouldn't save anything.
	if reqDetails.IsTier2Request {
		if err := p.stores.flushStores(ctx, p.executionStages, clock.Number); err != nil {
			return fmt.Errorf("step new irr: stores end of stream: %w", err)
		}
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

	dmetering.GetBytesMeter(ctx).CountInc("wasm_input_bytes", execOutput.Len())
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
		if err = returnModuleDataOutputs(clock, cursor, p.mapModuleOutput, p.extraMapModuleOutputs, p.extraStoreModuleOutputs, p.respFunc); err != nil {
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
	moduleExecutors, err := p.buildModuleExecutors(ctx)
	if err != nil {
		return fmt.Errorf("building wasm module tree: %w", err)
	}
	for _, stage := range moduleExecutors {
		//t0 := time.Now()

		if len(stage) < 2 {
			//fmt.Println("Linear stage", len(stage))
			for _, executor := range stage {
				res := p.execute(ctx, executor, execOutput)
				if err := p.applyExecutionResult(ctx, executor, res, execOutput); err != nil {
					return fmt.Errorf("applying executor results %q: %w", executor.Name(), res.err)
				}
			}
		} else {
			results := make([]resultObj, len(stage))
			wg := sync.WaitGroup{}
			//fmt.Println("Parallelized in stage", stageIdx, len(stage))
			for i, executor := range stage {
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
				executor := stage[i]
				if result.err != nil {
					//p.returnFailureProgress(ctx, err, executor)
					return fmt.Errorf("running executor %q: %w", executor.Name(), result.err)
				}
				if err := p.applyExecutionResult(ctx, executor, result, execOutput); err != nil {
					return fmt.Errorf("applying executor results %q: %w", executor.Name(), result.err)
				}
			}
		}
		//blockDuration += time.Since(t0)
	}

	return nil
}

type resultObj struct {
	output *pbssinternal.ModuleOutput
	bytes  []byte
	err    error
}

func (p *Pipeline) execute(ctx context.Context, executor exec.ModuleExecutor, execOutput execout.ExecutionOutput) resultObj {
	logger := reqctx.Logger(ctx)

	executorName := executor.Name()
	logger.Debug("executing", zap.Uint64("block", execOutput.Clock().Number), zap.String("module_name", executorName))

	moduleOutput, outputBytes, runError := exec.RunModule(ctx, executor, execOutput)
	return resultObj{moduleOutput, outputBytes, runError}
}

func (p *Pipeline) applyExecutionResult(ctx context.Context, executor exec.ModuleExecutor, res resultObj, execOutput execout.ExecutionOutput) (err error) {
	executorName := executor.Name()
	hasValidOutput := executor.HasValidOutput()

	moduleOutput, outputBytes, runError := res.output, res.bytes, res.err
	if runError != nil {
		if hasValidOutput {
			p.saveModuleOutput(moduleOutput, executor.Name(), reqctx.Details(ctx).ProductionMode)
		}
		return fmt.Errorf("execute module: %w", runError)
	}

	if hasValidOutput {
		p.saveModuleOutput(moduleOutput, executor.Name(), reqctx.Details(ctx).ProductionMode)
		if err := execOutput.Set(executorName, outputBytes); err != nil {
			return fmt.Errorf("set output cache: %w", err)
		}
		if moduleOutput != nil {
			p.forkHandler.addReversibleOutput(moduleOutput, execOutput.Clock().Id)
		}
	} else { // we are in a partial store
		if stor, ok := p.GetStoreMap().Get(executorName); ok {
			if pkvs, ok := stor.(*store.PartialKV); ok {
				if err := execOutput.Set(executorName, pkvs.ReadOps()); err != nil {
					return fmt.Errorf("set output cache: %w", err)
				}
			}

		}
	}
	return nil
}

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

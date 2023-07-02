package pipeline

import (
	"context"
	"errors"
	"fmt"
	"io"
	"runtime/debug"
	"sync"

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
)

func (p *Pipeline) ProcessBlock(block *bstream.Block, obj interface{}) (err error) {
	ctx := p.ctx

	reqStats := reqctx.ReqStats(ctx)
	logger := reqctx.Logger(ctx)
	defer func() {
		if r := recover(); r != nil {
			if err, ok := r.(error); ok {
				if errors.Is(err, context.Canceled) {
					logger.Info("context canceled")
					return
				}
			}
			err = fmt.Errorf("panic at block %s: %s", block, r)
			logger.Error("panic while process block", zap.Uint64("block_num", block.Num()), zap.Error(err))
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

	finalBlockHeight := obj.(bstream.Stepable).FinalBlockHeight()
	reorgJunctionBlock := obj.(bstream.Stepable).ReorgJunctionBlock()

	reqStats.RecordBlock(block.AsRef())
	p.gate.processBlock(block.Number, step)
	if err = p.processBlock(ctx, block, clock, cursor, step, finalBlockHeight, reorgJunctionBlock); err != nil {
		return err // watch out, io.EOF needs to go through undecorated
	}
	return
}

func blockToClock(block *bstream.Block) *pbsubstreams.Clock {
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
	block *bstream.Block,
	clock *pbsubstreams.Clock,
	cursor *bstream.Cursor,
	step bstream.StepType,
	finalBlockHeight uint64,
	reorgJunctionBlock bstream.BlockRef,
) (err error) {
	var eof bool
	switch step {
	case bstream.StepUndo:
		if err = p.handleStepUndo(ctx, clock, cursor, reorgJunctionBlock); err != nil {
			return fmt.Errorf("step undo: %w", err)
		}

	case bstream.StepStalled:
		if err := p.handleStepStalled(clock); err != nil {
			return fmt.Errorf("step stalled: %w", err)
		}

	case bstream.StepNew:
		err := p.handleStepNew(ctx, block, clock, cursor)
		if err != nil && err != io.EOF {
			return fmt.Errorf("step new: handler step new: %w", err)
		}
		if err == io.EOF {
			eof = true
		}
	case bstream.StepNewIrreversible:
		err := p.handleStepNew(ctx, block, clock, cursor)
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
		err = p.handleStepFinal(clock)
		if err != nil {
			return fmt.Errorf("handling step irreversible: %w", err)
		}
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

func (p *Pipeline) handleStepNew(ctx context.Context, block *bstream.Block, clock *pbsubstreams.Clock, cursor *bstream.Cursor) error {
	p.insideReorgUpTo = nil
	reqDetails := reqctx.Details(ctx)
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
	execOutput, err := p.execOutputCache.NewBuffer(block, clock, cursor)
	if err != nil {
		return fmt.Errorf("setting up exec output: %w", err)
	}

	if err := p.runPreBlockHooks(ctx, clock); err != nil {
		return fmt.Errorf("pre block hook: %w", err)
	}

	//blockDuration = 0
	//exec.Timer = 0
	if err := p.executeModules(ctx, execOutput); err != nil {
		return fmt.Errorf("execute modules: %w", err)
	}
	//sumCount++
	//sumDuration += exec.Timer
	//fmt.Println("accumulated time for all modules", exec.Timer, "avg", sumDuration/time.Duration(sumCount))

	if reqDetails.ShouldReturnProgressMessages() {
		if reqDetails.IsTier2Request {
			forceSend := (clock.Number+1)%p.runtimeConfig.CacheSaveInterval == 0

			if err = p.returnInternalModuleProgressOutputs(clock, forceSend); err != nil {
				return fmt.Errorf("failed to return modules progress %w", err)
			}
		} else {
			if err = p.returnRPCModuleProgressOutputs(clock); err != nil {
				return fmt.Errorf("failed to return modules progress %w", err)
			}
		}
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
	logger.Debug("block processed", zap.Uint64("block_num", block.Number))
	return nil
}

// var blockDuration time.Duration
// var sumDuration time.Duration
// var sumCount int
func (p *Pipeline) executeModules(ctx context.Context, execOutput execout.ExecutionOutput) (err error) {
	ctx, span := reqctx.WithModuleExecutionSpan(ctx, "modules_executions")
	defer span.EndWithErr(&err)

	// TODO(abourget): get the module executors lazily from the OutputModulesGraph
	//  this way we skip the buildWASM() in `Init()`.
	//  Would pave the way towards PATCH'd modules too.

	p.mapModuleOutput = nil
	p.extraMapModuleOutputs = nil
	p.extraStoreModuleOutputs = nil
	for _, stage := range p.moduleExecutors {
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

	if !hasValidOutput {
		return nil
	}
	p.saveModuleOutput(moduleOutput, executor.Name(), reqctx.Details(ctx).ProductionMode)
	if err := execOutput.Set(executorName, outputBytes); err != nil {
		return fmt.Errorf("set output cache: %w", err)
	}
	if moduleOutput != nil {
		p.forkHandler.addReversibleOutput(moduleOutput, execOutput.Clock().Id)
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

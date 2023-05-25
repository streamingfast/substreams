package pipeline

import (
	"context"
	"errors"
	"fmt"
	"io"
	"runtime/debug"

	"github.com/streamingfast/substreams/storage/execout"

	"github.com/streamingfast/substreams/reqctx"

	"github.com/streamingfast/bstream"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/streamingfast/substreams/metrics"
	pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
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

	if err := p.stores.flushStores(ctx, clock.Number); err != nil {
		return fmt.Errorf("step new irr: stores end of stream: %w", err)
	}

	// note: if we start on a forked cursor, the undo signal will appear BEFORE we send the snapshot
	if p.gate.shouldSendSnapshot() {
		if err := p.sendSnapshots(ctx, p.stores.StoreMap); err != nil {
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

	if err := p.executeModules(ctx, execOutput); err != nil {
		return fmt.Errorf("execute modules: %w", err)
	}

	if reqDetails.ShouldReturnProgressMessages() {
		if reqDetails.IsSubRequest {
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

func (p *Pipeline) executeModules(ctx context.Context, execOutput execout.ExecutionOutput) (err error) {
	ctx, span := reqctx.WithModuleExecutionSpan(ctx, "modules_executions")
	defer span.EndWithErr(&err)

	// TODO(abourget): get the module executors lazily from the OutputModulesGraph
	//  this way we skip the buildWASM() in `Init()`.
	//  Would pave the way towards PATCH'd modules too.

	p.mapModuleOutput = nil
	p.extraMapModuleOutputs = nil
	p.extraStoreModuleOutputs = nil
	for _, executor := range p.moduleExecutors {
		if err := p.execute(ctx, executor, execOutput); err != nil {
			//p.returnFailureProgress(ctx, err, executor)
			return fmt.Errorf("running executor %q: %w", executor.Name(), err)
		}
	}

	return nil
}

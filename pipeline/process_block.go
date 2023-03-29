package pipeline

import (
	"context"
	"errors"
	"fmt"
	"io"
	"runtime/debug"

	"github.com/streamingfast/substreams/storage/execout"

	"github.com/streamingfast/substreams/reqctx"
	"go.opentelemetry.io/otel/attribute"

	"github.com/streamingfast/bstream"
	"github.com/streamingfast/substreams/metrics"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func (p *Pipeline) ProcessBlock(block *bstream.Block, obj interface{}) (err error) {
	ctx, span := reqctx.WithSpan(p.ctx, "process_block")
	defer span.EndWithErr(&err)

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

	clock := &pbsubstreams.Clock{
		Number:    block.Num(),
		Id:        block.Id,
		Timestamp: timestamppb.New(block.Time()),
	}
	cursor := obj.(bstream.Cursorable).Cursor()
	step := obj.(bstream.Stepable).Step()

	span.SetAttributes(
		attribute.String("block.id", block.Id),
		attribute.Int64("block.num", int64(block.Number)),
		attribute.Stringer("block.step", step),
	)

	reqStats.RecordBlock(block.AsRef())
	p.gate.processBlock(block.Number, step)
	if err = p.processBlock(ctx, block, clock, cursor, step); err != nil {
		p.runPostJobHooks(ctx, clock)
		return err // watch out, io.EOF needs to go through undecorated
	}
	return
}

func (p *Pipeline) processBlock(ctx context.Context, block *bstream.Block, clock *pbsubstreams.Clock, cursor *bstream.Cursor, step bstream.StepType) (err error) {
	var eof bool
	switch step {
	case bstream.StepUndo:
		if err = p.handleStepUndo(ctx, clock, cursor); err != nil {
			return fmt.Errorf("step undo: %w", err)
		}

	case bstream.StepStalled:
		if err := p.handleStepStalled(clock); err != nil {
			return fmt.Errorf("step stalled: %w", err)
		}

	case bstream.StepNew:
		err := p.handlerStepNew(ctx, block, clock, cursor)
		if err != nil && err != io.EOF {
			return fmt.Errorf("step new: handler step new: %w", err)
		}
		if err == io.EOF {
			eof = true
		}
	case bstream.StepNewIrreversible:
		err := p.handlerStepNew(ctx, block, clock, cursor)
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

func (p *Pipeline) handleStepUndo(ctx context.Context, clock *pbsubstreams.Clock, cursor *bstream.Cursor) error {
	if p.gate.shouldSendSnapshot() {
		if err := p.sendSnapshots(ctx, p.stores.StoreMap); err != nil {
			return fmt.Errorf("send initial snapshots: %w", err)
		}
	}
	p.execOutputCache.HandleUndo(clock)

	var outputableModules []*pbsubstreams.Module
	if reqctx.Details(ctx).Request.GetProductionMode() {
		outputableModules = []*pbsubstreams.Module{p.outputGraph.OutputModule()}
	} else {
		outputableModules = p.outputGraph.AllModules()
	}

	if err := p.forkHandler.handleUndo(clock, cursor, p.respFunc, p.gate.shouldSendOutputs(), outputableModules); err != nil {
		return fmt.Errorf("reverting outputs: %w", err)
	}
	return nil
}

func (p *Pipeline) handleStepFinal(clock *pbsubstreams.Clock) error {
	p.lastFinalClock = clock
	if err := p.execOutputCache.HandleFinal(clock); err != nil {
		return fmt.Errorf("exec output cache: handle final: %w", err)
	}
	p.forkHandler.removeReversibleOutput(clock.Id)
	return nil
}

func (p *Pipeline) handlerStepNew(ctx context.Context, block *bstream.Block, clock *pbsubstreams.Clock, cursor *bstream.Cursor) error {
	reqDetails := reqctx.Details(ctx)
	if isBlockOverStopBlock(clock.Number, reqDetails.Request.StopBlockNum) {
		return io.EOF
	}

	if err := p.stores.flushStores(ctx, block.Number); err != nil {
		return fmt.Errorf("step new irr: stores end of stream: %w", err)
	}

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
		if err = p.returnModuleProgressOutputs(clock); err != nil {
			return fmt.Errorf("failed to return modules progress %w", err)
		}
	}

	if p.gate.shouldSendOutputs() {
		logger.Debug("will return module outputs")
		if err = returnModuleDataOutputs(clock, bstream.StepNew, cursor, p.moduleOutputs, p.respFunc); err != nil {
			return fmt.Errorf("failed to return module data output: %w", err)
		}
	}

	p.stores.resetStores()
	logger.Debug("block processed", zap.Uint64("block_num", block.Number))
	return nil
}

func (p *Pipeline) executeModules(ctx context.Context, execOutput execout.ExecutionOutput) (err error) {
	ctx, span := reqctx.WithSpan(ctx, "modules_executions")
	defer span.EndWithErr(&err)

	// TODO(abourget): get the module executors lazily from the OutputModulesGraph
	//  this way we skip the buildWASM() in `Init()`.
	//  Would pave the way towards PATCH'd modules too.

	p.moduleOutputs = nil
	for _, executor := range p.moduleExecutors {
		if err := p.execute(ctx, executor, execOutput); err != nil {
			return fmt.Errorf("running executor %q: %w", executor.Name(), err)
		}
	}

	return nil
}

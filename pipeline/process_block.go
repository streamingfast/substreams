package pipeline

import (
	"context"
	"fmt"
	"io"
	"runtime/debug"

	"github.com/streamingfast/substreams/reqctx"
	"go.opentelemetry.io/otel/attribute"

	"github.com/streamingfast/bstream"
	"github.com/streamingfast/substreams/metrics"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/pipeline/execout"
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
			err = fmt.Errorf("panic at block %s: %s", block, r)
			logger.Error("panic while process block", zap.Uint64("block_num", block.Num()), zap.Error(err))
			logger.Error(string(debug.Stack()))
		}
	}()

	metrics.BlockProcess.Inc()
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
	if err = p.processBlock(ctx, block, clock, cursor, step); err != nil {
		p.runPostJobHooks(ctx, clock)
		return err
	}

	return
}

func (p *Pipeline) processBlock(ctx context.Context, block *bstream.Block, clock *pbsubstreams.Clock, cursor *bstream.Cursor, step bstream.StepType) (err error) {
	switch step {
	case bstream.StepUndo:
		if err = p.handleStepUndo(ctx, clock, cursor); err != nil {
			return fmt.Errorf("step undo: %w", err)
		}

	case bstream.StepStalled:
		if err := p.handleStepStalled(ctx, clock); err != nil {
			return fmt.Errorf("step stalled: %w", err)
		}

	case bstream.StepNew:
		err := p.handlerStepNew(ctx, block, clock, cursor, step)
		if err != nil && err != io.EOF {
			return fmt.Errorf("step new: handler step new: %w", err)
		}
	case bstream.StepNewIrreversible:
		err := p.handlerStepNew(ctx, block, clock, cursor, step)
		if err != nil {
			return fmt.Errorf("step new irr: handler step new: %w", err)
		}

		p.handleStepIrreversible(block.Number)
	case bstream.StepIrreversible:

		p.handleStepIrreversible(block.Number)
	}

	return nil
}

func (p *Pipeline) handleStepStalled(_ context.Context, clock *pbsubstreams.Clock) error {
	p.forkHandler.removeReversibleOutput(clock.Number)
	return nil
}

func (p *Pipeline) handleStepUndo(ctx context.Context, clock *pbsubstreams.Clock, cursor *bstream.Cursor) error {
	reqctx.Span(ctx).AddEvent("handling_step_undo")
	if err := p.forkHandler.handleUndo(clock, cursor, p.respFunc); err != nil {
		return fmt.Errorf("reverting outputs: %w", err)
	}
	return nil
}

func (p *Pipeline) handleStepIrreversible(blockNum uint64) {
	p.forkHandler.removeReversibleOutput(blockNum)
}

func (p *Pipeline) handlerStepNew(ctx context.Context, block *bstream.Block, clock *pbsubstreams.Clock, cursor *bstream.Cursor, step bstream.StepType) error {
	reqDetails := reqctx.Details(ctx)
	if isBlockOverStopBlock(clock.Number, reqDetails.Request.StopBlockNum) {
		return io.EOF
	}

	if err := p.flushStores(ctx, block.Number); err != nil {
		return fmt.Errorf("step new irr: stores end of stream: %w", err)
	}

	logger := reqctx.Logger(ctx)
	execOutput, err := p.execOutputCache.NewExecOutput(p.blockType, block, clock, cursor)
	if err != nil {
		return fmt.Errorf("setting up exec output: %w", err)
	}

	if err := p.runPreBlockHooks(ctx, clock); err != nil {
		return fmt.Errorf("pre block hook: %w", err)
	}

	if err := p.executeModules(ctx, execOutput); err != nil {
		return fmt.Errorf("execute modules: %w", err)
	}

	if shouldReturnProgress(reqDetails.IsSubRequest) {
		if err = p.returnModuleProgressOutputs(clock); err != nil {
			return fmt.Errorf("failed to return modules progress %w", err)
		}
	}

	if shouldReturnDataOutputs(clock.Number, reqDetails.EffectiveStartBlockNum, reqDetails.IsSubRequest) {
		logger.Debug("will return module outputs")

		if err = returnModuleDataOutputs(clock, step, cursor, p.moduleOutputs, p.respFunc); err != nil {
			return fmt.Errorf("failed to return module data output: %w", err)
		}
	}

	p.resetStores()
	p.moduleOutputs = nil
	logger.Debug("block processed", zap.Uint64("block_num", block.Number))
	return nil
}

func (p *Pipeline) executeModules(ctx context.Context, execOutput execout.ExecutionOutput) (err error) {
	ctx, span := reqctx.WithSpan(ctx, "modules_executions")
	defer span.EndWithErr(&err)

	for _, executor := range p.moduleExecutors {
		if err := p.execute(ctx, executor, execOutput); err != nil {
			return fmt.Errorf("running executor %q: %w", executor.Name(), err)
		}
	}

	return nil
}

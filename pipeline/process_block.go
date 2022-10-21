package pipeline

import (
	"context"
	"fmt"
	"github.com/streamingfast/substreams/reqctx"
	"go.opentelemetry.io/otel/attribute"
	"io"
	"runtime/debug"

	"github.com/streamingfast/bstream"
	"github.com/streamingfast/substreams/metrics"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/pipeline/execout"
	"github.com/streamingfast/substreams/store"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func (p *Pipeline) ProcessBlock(block *bstream.Block, obj interface{}) (err error) {
	ctx, span := reqctx.WithSpan(p.ctx, "process_block")
	defer span.EndWithErr(&err)

	metrics.BlockBeginProcess.Inc()
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
	span.AddEvent("fooevenet")

	if err = p.processBlock(ctx, block, clock, cursor, step, span); err != nil {
		// TODO should we check th error here
		p.runPostJobHooks(ctx, clock)
	}
	return
}

func (p *Pipeline) processBlock(ctx context.Context, block *bstream.Block, clock *pbsubstreams.Clock, cursor *bstream.Cursor, step bstream.StepType, span trace.Span) (err error) {
	logger := reqctx.Logger(ctx)
	// TODO: should this move to the step new
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic at block %s: %s", block, r)
			logger.Error("panic while process block", zap.Uint64("block_num", block.Num()), zap.Error(err))
			logger.Error(string(debug.Stack()))
		}
	}()

	switch {
	case step.Matches(bstream.StepUndo):
		if err = p.handleStepUndo(clock, cursor, span); err != nil {
			return fmt.Errorf("step undo: %w", err)
		}

	case step.Matches(bstream.StepStalled):
		p.forkHandler.removeReversibleOutput(block.Num())

	case step.Matches(bstream.StepNew):
		if err := p.handleStepMatchesNew(ctx, block, clock, cursor, step, span); err != nil {
			return fmt.Errorf("step new: %w", err)
		}
	}

	if step.Matches(bstream.StepIrreversible) {
		p.forkHandler.removeReversibleOutput(block.Num())
	}

	if err := p.execOutputCache.NewBlock(block.AsRef(), step); err != nil {
		return fmt.Errorf("caching engine new block %s: %w", block.AsRef().String(), err)
	}

	return nil
}

func (p *Pipeline) handleStepUndo(clock *pbsubstreams.Clock, cursor *bstream.Cursor, span trace.Span) error {
	span.AddEvent("handling_step_undo")
	if err := p.forkHandler.handleUndo(clock, cursor, p.StoreMap, p.respFunc); err != nil {
		return fmt.Errorf("reverting outputs: %w", err)
	}
	return nil
}

func (p *Pipeline) handleStepMatchesNew(ctx context.Context, block *bstream.Block, clock *pbsubstreams.Clock, cursor *bstream.Cursor, step bstream.StepType, span trace.Span) error {
	reqdetails := reqctx.Details(ctx)
	logger := reqctx.Logger(ctx)
	execOutput, err := p.execOutputCache.NewExecOutput(p.blockType, block, clock, cursor)
	if err != nil {
		return fmt.Errorf("setting up exec output: %w", err)
	}

	if err := p.runPreBlockHooks(ctx, clock); err != nil {
		return fmt.Errorf("pre block hook: %w", err)
	}

	// TODO: will this happen twice? blockstream also calls this at stopBluckNum
	if err = p.flushStores(ctx, block.Num()); err != nil {
		return fmt.Errorf("failed to flush stores: %w", err)
	}

	if isStopBlockReached(clock.Number, reqdetails.Request.StopBlockNum) {
		// TODO: should we not flush the cache only in IRR
		//	p.ctx.logger.Debug("about to save cache output",
		//		zap.Uint64("clock", clock.Number),
		//		zap.Uint64("stop_block", p.ctx.StopBlockNum()),
		//	)
		//	if err = p.execOutputCache.Flush(p.ctx.Context()); err != nil {
		//		return fmt.Errorf("failed to flush cache engines: %w", err)
		//	}
		return io.EOF
	}

	if err := p.executeModules(ctx, execOutput); err != nil {
		return fmt.Errorf("execute modules: %w", err)
	}

	if shouldReturnProgress(reqdetails.IsSubRequest) {
		if err = p.returnModuleProgressOutputs(clock); err != nil {
			return fmt.Errorf("failed to return modules progress %w", err)
		}
	}

	if shouldReturnDataOutputs(clock.Number, reqdetails.EffectiveStartBlockNum, reqdetails.IsSubRequest) {
		logger.Debug("will return module outputs")

		if err = returnModuleDataOutputs(clock, step, cursor, p.moduleOutputs, p.respFunc); err != nil {
			return fmt.Errorf("failed to return module data output: %w", err)
		}
	}

	for _, s := range p.StoreMap.All() {
		if resetableStore, ok := s.(store.Resetable); ok {
			resetableStore.Reset()
		}
	}

	p.moduleOutputs = nil
	logger.Debug("block processed", zap.Uint64("block_num", block.Number))
	return nil
}

func (p *Pipeline) executeModules(ctx context.Context, execOutput execout.ExecutionOutput) (err error) {
	ctx, span := reqctx.WithSpan(ctx, "modules_executions")
	defer span.EndWithErr(&err)

	for _, executor := range p.moduleExecutors {
		if err := p.runExecutor(ctx, executor, execOutput); err != nil {
			return fmt.Errorf("running executor %q: %w", executor.Name(), err)
		}
	}

	metrics.BlockEndProcess.Inc()
	return nil
}

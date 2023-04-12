package cache

import (
	"context"
	"fmt"

	"github.com/streamingfast/substreams/reqctx"

	"github.com/streamingfast/bstream"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/service/config"
	"github.com/streamingfast/substreams/storage/execout"
	"go.uber.org/zap"
)

// TODO(abourget): this is now something like an execout.LifecycleWriter ?
//
//	pipeline.Lifecycle ? to hold also the *pbsubstreams.ModuleOutput
//	so that `ForkHandler` disappears in the end?
type Engine struct {
	ctx               context.Context
	blockType         string
	reversibleBuffers map[uint64]*execout.Buffer // block num to modules' outputs for that given block
	writableFiles     *execout.Writer            // moduleName => irreversible File
	runtimeConfig     config.RuntimeConfig
	logger            *zap.Logger
}

func NewEngine(ctx context.Context, runtimeConfig config.RuntimeConfig, execOutWriter *execout.Writer, blockType string) (*Engine, error) {
	e := &Engine{
		ctx:               ctx,
		runtimeConfig:     runtimeConfig,
		reversibleBuffers: map[uint64]*execout.Buffer{},
		writableFiles:     execOutWriter,
		logger:            reqctx.Logger(ctx),
		blockType:         blockType,
	}
	return e, nil
}

func (e *Engine) NewBuffer(block *bstream.Block, clock *pbsubstreams.Clock, cursor *bstream.Cursor) (execout.ExecutionOutput, error) {
	execOutBuf, err := execout.NewBuffer(e.blockType, block, clock)
	if err != nil {
		return nil, fmt.Errorf("setting up map: %w", err)
	}

	e.reversibleBuffers[clock.Number] = execOutBuf

	return execOutBuf, nil
}

func (e *Engine) HandleUndo(clock *pbsubstreams.Clock) {
	delete(e.reversibleBuffers, clock.Number)
}

func (e *Engine) HandleFinal(clock *pbsubstreams.Clock) error {
	execOutBuf := e.reversibleBuffers[clock.Number]
	if execOutBuf == nil {
		// TODO(abourget): cross check here, do we want to defer the MaybeRotate
		//  at after?
		return nil
	}

	if e.writableFiles != nil {
		// TODO(abourget): clarify what we send to `MaybeRotate`, perhaps we do the checking
		// flushing conditions here? We pass a few conditions down?
		// the File down there will know if it should flush its subrequest or not?
		if err := e.writableFiles.MaybeRotate(e.ctx, clock.Number); err != nil {
			return fmt.Errorf("rotating writable files: %w", err)
		}

		e.writableFiles.Write(clock, execOutBuf)
	}

	delete(e.reversibleBuffers, clock.Number)

	return nil
}

func (e *Engine) HandleStalled(clock *pbsubstreams.Clock) error {
	delete(e.reversibleBuffers, clock.Number)
	return nil
}

func (e *Engine) EndOfStream(lastFinalClock *pbsubstreams.Clock) error {
	if e.writableFiles != nil {
		// We're adding +1 here for the case where we triggered the `stopBlock` using the
		// >= clause, in which case +1 will make it go over that boundary and save/rotate the files.
		// In the cases where we skipped huge number of blocks, and we get a large clock jump
		// then +1 is not necessary but won't harm either.
		if err := e.writableFiles.MaybeRotate(e.ctx, lastFinalClock.Number+1); err != nil {
			return fmt.Errorf("rotating writable files: %w", err)
		}
	}

	return nil
}

func (e *Engine) Close() {
	if e.writableFiles != nil {
		e.writableFiles.Close()
	}
}

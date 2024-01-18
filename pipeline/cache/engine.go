package cache

import (
	"context"
	"fmt"

	pbbstream "github.com/streamingfast/bstream/pb/sf/bstream/v1"

	"github.com/streamingfast/substreams/reqctx"

	"github.com/streamingfast/bstream"
	"go.uber.org/zap"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/service/config"
	"github.com/streamingfast/substreams/storage/execout"
)

// Engine manages the reversible segments and keeps track of
// the execution output between each module's.
//
// Upon Finality, it writes it to some output cache files.
type Engine struct {
	// FIXME: Rename to pipeline.Lifecycle ? to hold also the *pbsubstreams.ModuleOutput
	//  so that `ForkHandler` disappears in the end?
	ctx               context.Context
	blockType         string
	reversibleBuffers map[uint64]*execout.Buffer // block num to modules' outputs for that given block
	execOutputWriter  *execout.Writer            // moduleName => irreversible File
	runtimeConfig     config.RuntimeConfig       // TODO(abourget): Deprecated: remove this as it's not used
	logger            *zap.Logger
}

func NewEngine(ctx context.Context, runtimeConfig config.RuntimeConfig, execOutWriter *execout.Writer, blockType string) (*Engine, error) {
	e := &Engine{
		ctx:               ctx,
		runtimeConfig:     runtimeConfig,
		reversibleBuffers: map[uint64]*execout.Buffer{},
		execOutputWriter:  execOutWriter,
		logger:            reqctx.Logger(ctx),
		blockType:         blockType,
	}
	return e, nil
}

func (e *Engine) NewBuffer(block *pbbstream.Block, clock *pbsubstreams.Clock, cursor *bstream.Cursor) (execout.ExecutionOutput, error) {
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

	if e.execOutputWriter != nil {
		e.execOutputWriter.Write(clock, execOutBuf)
	}

	delete(e.reversibleBuffers, clock.Number)

	return nil
}

func (e *Engine) HandleStalled(clock *pbsubstreams.Clock) error {
	delete(e.reversibleBuffers, clock.Number)
	return nil
}

func (e *Engine) EndOfStream(lastFinalClock *pbsubstreams.Clock) error {
	if e.execOutputWriter != nil {
		e.execOutputWriter.Close(context.Background())
	}
	return nil
}

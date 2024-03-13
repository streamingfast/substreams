package integration

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"go.uber.org/atomic"
	"go.uber.org/zap"

	"github.com/streamingfast/substreams/block"
	"github.com/streamingfast/substreams/orchestrator/loop"
	"github.com/streamingfast/substreams/orchestrator/response"
	"github.com/streamingfast/substreams/orchestrator/stage"
	"github.com/streamingfast/substreams/orchestrator/work"
	"github.com/streamingfast/substreams/reqctx"
)

type TestWorker struct {
	t                      *testing.T
	responseCollector      *responseCollector
	newBlockGenerator      BlockGeneratorFactory
	blockProcessedCallBack blockProcessedCallBack
	testTempDir            string
	id                     uint64
}

var workerID atomic.Uint64

func (w *TestWorker) ID() string {
	return fmt.Sprintf("%d", w.id)
}

func (w *TestWorker) Work(ctx context.Context, unit stage.Unit, workRange *block.Range, moduleNames []string, upstream *response.Stream) loop.Cmd {
	w.t.Helper()

	ctx = reqctx.WithTier2RequestParameters(ctx, reqctx.Tier2RequestParameters{
		BlockType:            "sf.substreams.v1.test.Block",
		StateBundleSize:      10,
		StateStoreURL:        filepath.Join(w.testTempDir, "test.store"),
		StateStoreDefaultTag: "tag",
	})
	request := work.NewRequest(ctx, reqctx.Details(ctx), unit.Stage, workRange)

	logger := reqctx.Logger(ctx)
	logger = logger.With(zap.Uint64("workerId", w.id))
	ctx = reqctx.WithLogger(ctx, logger)

	logger.Info("worker running test job",
		zap.Strings("stage_modules", moduleNames),
		zap.Int("stage", unit.Stage),
		zap.Uint64("start_block_num", request.StartBlockNum),
		zap.Uint64("stop_block_num", request.StopBlockNum),
	)

	return func() loop.Msg {
		if err := processInternalRequest(w.t, ctx, request, nil, w.newBlockGenerator, w.responseCollector, w.blockProcessedCallBack, w.testTempDir); err != nil {
			return work.MsgJobFailed{Unit: unit, Error: fmt.Errorf("processing test tier2 request: %w", err)}
		}
		logger.Info("worker done running job",
			zap.String("output_module", request.OutputModule),
			zap.Uint64("start_block_num", request.StartBlockNum),
			zap.Uint64("stop_block_num", request.StopBlockNum),
			zap.Int("stage", unit.Stage),
		)

		return work.MsgJobSucceeded{Unit: unit, Worker: w}
	}
}

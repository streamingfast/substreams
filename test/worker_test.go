package integration

import (
	"context"
	"fmt"
	"github.com/streamingfast/substreams"
	"github.com/streamingfast/substreams/block"
	"github.com/streamingfast/substreams/manifest"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/reqctx"
	"go.uber.org/zap"
	"testing"
)

type TestWorker struct {
	t                      *testing.T
	moduleGraph            *manifest.ModuleGraph
	responseCollector      *responseCollector
	newBlockGenerator      NewTestBlockGenerator
	blockProcessedCallBack blockProcessedCallBack
	testTempDir            string
}

func (w *TestWorker) Run(ctx context.Context, request *pbsubstreams.Request, _ substreams.ResponseFunc) (brange []*block.Range, err error) {
	w.t.Helper()

	ctx, span := reqctx.WithSpan(ctx, "running_job_test")
	defer span.EndWithErr(&err)

	logger := reqctx.Logger(ctx)

	logger.Info("worker running job",
		zap.Strings("output_modules", request.OutputModules),
		zap.Int64("start_block_num", request.StartBlockNum),
		zap.Uint64("stop_block_num", request.StopBlockNum),
	)
	if err := processRequest(w.t, ctx, request, w.moduleGraph, nil, w.newBlockGenerator, w.responseCollector, true, w.blockProcessedCallBack, w.testTempDir); err != nil {
		return nil, fmt.Errorf("processing sub request: %w", err)
	}

	return block.Ranges{
		&block.Range{
			StartBlock:        uint64(request.StartBlockNum),
			ExclusiveEndBlock: request.StopBlockNum,
		},
	}, nil
}

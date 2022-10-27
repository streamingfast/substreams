package integration

import (
	"context"
	"fmt"
	"math"
	"testing"

	"go.uber.org/atomic"

	"github.com/streamingfast/substreams"
	"github.com/streamingfast/substreams/block"
	"github.com/streamingfast/substreams/manifest"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/reqctx"
	"go.uber.org/zap"
)

type TestWorker struct {
	t                      *testing.T
	moduleGraph            *manifest.ModuleGraph
	responseCollector      *responseCollector
	newBlockGenerator      NewTestBlockGenerator
	blockProcessedCallBack blockProcessedCallBack
	testTempDir            string
}

var workerID atomic.Uint64

func (w *TestWorker) Run(ctx context.Context, request *pbsubstreams.Request, _ substreams.ResponseFunc) (brange []*block.Range, err error) {
	w.t.Helper()

	ctx, span := reqctx.WithSpan(ctx, "running_job_test")
	defer span.EndWithErr(&err)

	logger := reqctx.Logger(ctx)
	wid := workerID.Inc()
	logger = logger.With(zap.Uint64("workerId", wid))
	ctx = reqctx.WithLogger(ctx, logger)

	logger.Info("worker running job",
		zap.Strings("output_modules", request.OutputModules),
		zap.Int64("start_block_num", request.StartBlockNum),
		zap.Uint64("stop_block_num", request.StopBlockNum),
	)
	subrequestsSplitSize := uint64(10)
	if err := processRequest(w.t, ctx, request, w.moduleGraph, nil, w.newBlockGenerator, w.responseCollector, true, w.blockProcessedCallBack, w.testTempDir, subrequestsSplitSize); err != nil {
		return nil, fmt.Errorf("processing sub request: %w", err)
	}

	var blockRanges []*block.Range
	if request.StopBlockNum-uint64(request.StartBlockNum) > subrequestsSplitSize {
		blockRanges = splitBlockRanges(request, subrequestsSplitSize)
	} else {
		blockRanges = []*block.Range{
			{
				StartBlock:        uint64(request.StartBlockNum),
				ExclusiveEndBlock: request.StopBlockNum,
			},
		}
	}
	return blockRanges, nil
}

// splitBlockRanges for example: called when subrequestsSplitSize is 10 and request
// has a start block of 1 and stop block of 20 -> splits to [[1, 10), [10, 20)]
func splitBlockRanges(request *pbsubstreams.Request, subrequestsSplitSize uint64) []*block.Range {
	var blockRanges block.Ranges
	nbSplitRequests := int(math.Ceil(float64(request.StopBlockNum / subrequestsSplitSize)))

	for i := 0; i < nbSplitRequests; i++ {
		blockRange := &block.Range{
			StartBlock:        uint64(i) * subrequestsSplitSize,
			ExclusiveEndBlock: uint64(i+1) * subrequestsSplitSize,
		}
		if i == 0 {
			blockRange.StartBlock = 1
		}
		if i == nbSplitRequests-1 {
			blockRange.ExclusiveEndBlock = request.StopBlockNum
		}
		blockRanges = append(blockRanges, blockRange)
	}
	return blockRanges
}

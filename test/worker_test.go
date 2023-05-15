package integration

import (
	"context"
	"fmt"
	"math"
	"testing"

	"github.com/streamingfast/substreams"
	"github.com/streamingfast/substreams/block"
	"github.com/streamingfast/substreams/orchestrator/work"
	pbssinternal "github.com/streamingfast/substreams/pb/sf/substreams/intern/v2"
	"github.com/streamingfast/substreams/reqctx"
	"github.com/streamingfast/substreams/service"
	"github.com/streamingfast/substreams/storage/store"
	"go.uber.org/atomic"
	"go.uber.org/zap"
)

type TestWorker struct {
	t                      *testing.T
	responseCollector      *responseCollector
	newBlockGenerator      BlockGeneratorFactory
	blockProcessedCallBack blockProcessedCallBack
	testTempDir            string
	id                     uint64
	traceID                *string
}

var workerID atomic.Uint64

func (w *TestWorker) ID() string {
	return fmt.Sprintf("%d", w.id)
}

func (w *TestWorker) Work(ctx context.Context, request *pbssinternal.ProcessRangeRequest, _ substreams.ResponseFunc) *work.Result {
	w.t.Helper()
	var err error

	ctx, span := reqctx.WithSpan(ctx, "running_job_test")
	defer span.EndWithErr(&err)

	logger := reqctx.Logger(ctx)
	logger = logger.With(zap.Uint64("workerId", w.id))
	ctx = reqctx.WithLogger(ctx, logger)

	logger.Info("worker running job",
		zap.String("output_module", request.OutputModule),
		zap.Uint64("start_block_num", request.StartBlockNum),
		zap.Uint64("stop_block_num", request.StopBlockNum),
	)
	subrequestsSplitSize := uint64(10)
	if err := processInternalRequest(w.t, ctx, request, nil, w.newBlockGenerator, w.responseCollector, true, w.blockProcessedCallBack, w.testTempDir, subrequestsSplitSize, 1, 0, w.traceID); err != nil {
		return &work.Result{
			Error: fmt.Errorf("processing sub request: %w", err),
		}
	}
	logger.Info("worker done running job",
		zap.String("output_module", request.OutputModule),
		zap.Uint64("start_block_num", request.StartBlockNum),
		zap.Uint64("stop_block_num", request.StopBlockNum),
	)

	var partialFiles store.FileInfos
	if request.StopBlockNum-uint64(request.StartBlockNum) > subrequestsSplitSize {
		partialFiles = splitFileRanges(request.StopBlockNum, subrequestsSplitSize)
	} else {
		traceID := service.TestTraceID
		if w.traceID != nil {
			traceID = *w.traceID
		}

		partialFiles = store.FileInfos{
			store.NewPartialFileInfo(uint64(request.StartBlockNum), request.StopBlockNum, traceID),
		}
	}

	return &work.Result{
		PartialFilesWritten: partialFiles,
		Error:               nil,
	}
}

// splitFileRanges for example: called when subrequestsSplitSize is 10 and request
// has a start block of 1 and stop block of 20 -> splits to [[1, 10), [10, 20)]
func splitFileRanges(stopBlockNum, subrequestsSplitSize uint64) store.FileInfos {
	var fileRanges store.FileInfos
	nbSplitRequests := int(math.Ceil(float64(stopBlockNum) / float64(subrequestsSplitSize)))

	for i := 0; i < nbSplitRequests; i++ {
		blockRange := &block.Range{
			StartBlock:        uint64(i) * subrequestsSplitSize,
			ExclusiveEndBlock: uint64(i+1) * subrequestsSplitSize,
		}
		if i == 0 {
			blockRange.StartBlock = 1
		}
		if i == nbSplitRequests-1 {
			blockRange.ExclusiveEndBlock = stopBlockNum
		}

		fileRanges = append(fileRanges, store.NewPartialFileInfo(blockRange.StartBlock, blockRange.ExclusiveEndBlock, ""))
	}
	return fileRanges
}

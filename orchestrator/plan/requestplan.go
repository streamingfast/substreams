package plan

import (
	"fmt"

	"github.com/streamingfast/substreams/block"
	"github.com/streamingfast/substreams/utils"
)

// RequestPlan lays out the configuration of the components to accomplish
// the work of the ParallelProcessor. Different conditions put different
// constraints on the output of the parallel processor.
type RequestPlan struct {
	// This is simply the ranges that exist and are
	// considered in the request. Further process will deal with the
	// existence or non-existence of the current partials and full stores.
	// And they will decide whether to schedule work or not.
	BuildStores *block.Range

	// Whether to process the last map stage.
	//
	// In development mode,
	// we only care about processing the stores up to the handoff block,
	// which then kicks in the linear mode, which will then output its
	// results.
	// In production mode, we will want that mapper to be produced
	// to generate the ExecOut files, and kick off the ExecOutWalker
	// here to output the results.
	//
	// WriteExecOut will always have a start block on the boundary,
	// so the reading process needs to take into account the _start block_
	// at which it wants to send the data over. Production of map output
	// requires stores to be aligned, so needs to start from previous
	// store snapshots.
	WriteExecOut *block.Range // Can be nil

	// Range that will be produced by the linear pipeline. Could have no end.
	LinearPipeline *block.Range
	// ref: /docs/assets/range_planning.png

	// Whether to save a Full Store snapshot to the storage when the
	// last segment is not on standard boundaries.
	//
	// This will be useful in development mode, where the user wants
	// to iterate multiple times at the same start block, without
	// needing to sync from say 1000 to 1565, wasting 565 blocks
	// of processing each time.
	// We would not save those in production mode, because the chances
	// of being reused are very low. You don't iterate in production mode.
	SnapshotFullStoresAtHandoff bool // to speed up iterations in dev mode
}

func BuildRequestPlan(productionMode bool, segmenter *block.Segmenter, graphInitBlock, resolvedStartBlock, linearHandoffBlock, exclusiveEndBlock uint64) *RequestPlan {
	plan := &RequestPlan{}
	plan.SnapshotFullStoresAtHandoff = !productionMode
	if linearHandoffBlock != exclusiveEndBlock {
		plan.LinearPipeline = block.NewRange(linearHandoffBlock, exclusiveEndBlock)
	}
	if resolvedStartBlock < graphInitBlock {
		panic(fmt.Errorf("start block cannot be prior to the lowest init block in the requested module graph (%d)", graphInitBlock))
	}
	if productionMode {
		storesStopOnBound := plan.LinearPipeline == nil
		endStoreBound := linearHandoffBlock
		if storesStopOnBound {
			segmentIdx := segmenter.IndexForBlock(linearHandoffBlock)
			endStoreBound = segmenter.Range(segmentIdx).StartBlock
		}
		plan.BuildStores = block.NewRange(graphInitBlock, endStoreBound)

		startExecOutAtBlock := utils.MaxOf(resolvedStartBlock, graphInitBlock)
		startExecOutAtSegment := segmenter.IndexForBlock(startExecOutAtBlock)
		execOutStartBlock := segmenter.Range(startExecOutAtSegment).StartBlock
		plan.WriteExecOut = block.NewRange(execOutStartBlock, linearHandoffBlock)
	} else { /* dev mode */
		plan.BuildStores = block.NewRange(graphInitBlock, linearHandoffBlock)
		plan.WriteExecOut = nil
		plan.LinearPipeline = block.NewRange(linearHandoffBlock, exclusiveEndBlock)
	}
	return plan
}

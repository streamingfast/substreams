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

	segmentInterval uint64
}

func BuildTier1RequestPlan(productionMode bool, segmentInterval uint64, graphInitBlock, resolvedStartBlock, linearHandoffBlock, exclusiveEndBlock uint64) *RequestPlan {
	segmenter := block.NewSegmenter(segmentInterval, graphInitBlock, exclusiveEndBlock)
	plan := &RequestPlan{
		segmentInterval: segmentInterval,
	}
	if linearHandoffBlock != exclusiveEndBlock {
		// assumes exclusiveEndBlock isn't 0, because linearHandoffBlock cannot be 0
		plan.LinearPipeline = block.NewRange(linearHandoffBlock, exclusiveEndBlock)
	}
	if resolvedStartBlock < graphInitBlock {
		panic(fmt.Errorf("start block cannot be prior to the lowest init block in the requested module graph (%d)", graphInitBlock))
	}
	if resolvedStartBlock == linearHandoffBlock && graphInitBlock == resolvedStartBlock {
		return plan
	}
	if productionMode {
		storesStopOnBound := plan.LinearPipeline == nil
		endStoreBound := linearHandoffBlock
		if storesStopOnBound {
			segmentIdx := segmenter.IndexForEndBlock(linearHandoffBlock)
			endStoreBound = segmenter.Range(segmentIdx).StartBlock
		}
		plan.BuildStores = block.NewRange(graphInitBlock, endStoreBound)

		startExecOutAtBlock := utils.MaxOf(resolvedStartBlock, graphInitBlock)
		startExecOutAtSegment := segmenter.IndexForStartBlock(startExecOutAtBlock)
		execOutStartBlock := segmenter.Range(startExecOutAtSegment).StartBlock
		plan.WriteExecOut = block.NewRange(execOutStartBlock, linearHandoffBlock)
	} else { /* dev mode */
		plan.BuildStores = block.NewRange(graphInitBlock, linearHandoffBlock)
		plan.WriteExecOut = nil
	}
	return plan
}

func (p *RequestPlan) StoresSegmenter() *block.Segmenter {
	return block.NewSegmenter(p.segmentInterval, p.BuildStores.StartBlock, p.BuildStores.ExclusiveEndBlock)
}

func (p *RequestPlan) ModuleSegmenter(modInitBlock uint64) *block.Segmenter {
	return block.NewSegmenter(p.segmentInterval, modInitBlock, p.BuildStores.ExclusiveEndBlock)
}

func (p *RequestPlan) WriteOutSegmenter() *block.Segmenter {
	return block.NewSegmenter(p.segmentInterval, p.WriteExecOut.StartBlock, p.WriteExecOut.ExclusiveEndBlock)
}

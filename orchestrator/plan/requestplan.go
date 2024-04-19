package plan

import (
	"fmt"

	"github.com/streamingfast/substreams/block"
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

	// When WriteExecOut produces files, we might want to start reading
	// blocks only a bit further down a file (if boundary is at 20,
	// the request's start block might be 25). This will instruct
	// the output stream to start at that block number.
	ReadExecOut *block.Range

	// Range that will be produced by the linear pipeline. Could have no end.
	LinearPipeline *block.Range
	// ref: /docs/assets/range_planning.png

	segmentInterval uint64
}

func (p *RequestPlan) RequiresParallelProcessing() bool {
	return p.WriteExecOut != nil || p.BuildStores != nil
}

func BuildTier1RequestPlan(productionMode bool, segmentInterval uint64, graphInitBlock, resolvedStartBlock, linearHandoffBlock, exclusiveEndBlock uint64, scheduleStores bool) (*RequestPlan, error) {
	if exclusiveEndBlock != 0 && linearHandoffBlock > exclusiveEndBlock {
		return nil, fmt.Errorf("end block %d cannot be prior to the linear handoff block %d", exclusiveEndBlock, linearHandoffBlock)
	}
	if resolvedStartBlock < graphInitBlock {
		return nil, fmt.Errorf("start block cannot be prior to the lowest init block in the requested module graph (%d)", graphInitBlock)
	}

	segmenter := block.NewSegmenter(segmentInterval, graphInitBlock, exclusiveEndBlock)
	plan := &RequestPlan{
		segmentInterval: segmentInterval,
	}
	if linearHandoffBlock != exclusiveEndBlock ||
		linearHandoffBlock == 0 { // ex: unbound dev mode
		plan.LinearPipeline = block.NewRange(linearHandoffBlock, exclusiveEndBlock)
	}
	if resolvedStartBlock == linearHandoffBlock && graphInitBlock == resolvedStartBlock {
		return plan, nil
	}
	if productionMode {
		storesStopOnBound := plan.LinearPipeline == nil
		endStoreBound := linearHandoffBlock
		if storesStopOnBound {
			segmentIdx := segmenter.IndexForEndBlock(linearHandoffBlock)
			endStoreBoundRange := segmenter.Range(segmentIdx)
			if endStoreBoundRange == nil {
				return nil, fmt.Errorf("store bound range: invalid start block %d for segment interval %d", linearHandoffBlock, segmentInterval)
			}
			endStoreBound = endStoreBoundRange.ExclusiveEndBlock
		}
		if scheduleStores {
			plan.BuildStores = block.NewRange(graphInitBlock, endStoreBound)
		}

		if resolvedStartBlock <= linearHandoffBlock {
			startExecOutAtBlock := max(resolvedStartBlock, graphInitBlock)
			startExecOutAtSegment := segmenter.IndexForStartBlock(startExecOutAtBlock)
			writeExecOutStartBlockRange := segmenter.Range(startExecOutAtSegment)
			if writeExecOutStartBlockRange == nil {
				return nil, fmt.Errorf("write execout range: invalid start block %d for segment interval %d", startExecOutAtBlock, segmentInterval)
			}
			writeExecOutStartBlock := writeExecOutStartBlockRange.StartBlock
			plan.WriteExecOut = block.NewRange(writeExecOutStartBlock, linearHandoffBlock)
			plan.ReadExecOut = block.NewRange(resolvedStartBlock, linearHandoffBlock)
		}
	} else { /* dev mode */
		if scheduleStores {
			plan.BuildStores = block.NewRange(graphInitBlock, linearHandoffBlock)
		}
		plan.WriteExecOut = nil
	}
	return plan, nil
}

func (p *RequestPlan) StoresSegmenter() *block.Segmenter {
	return block.NewSegmenter(p.segmentInterval, p.BuildStores.StartBlock, p.BuildStores.ExclusiveEndBlock)
}

func (p *RequestPlan) BackprocessSegmenter() *block.Segmenter {
	if p.BuildStores == nil {
		return p.WriteOutSegmenter()
	} else if p.WriteExecOut == nil {
		return p.StoresSegmenter()
	}
	return block.NewSegmenter(
		p.segmentInterval,
		min(p.BuildStores.StartBlock, p.WriteExecOut.StartBlock),
		max(p.BuildStores.ExclusiveEndBlock, p.WriteExecOut.ExclusiveEndBlock),
	)
}

func (p *RequestPlan) ModuleSegmenter(modInitBlock uint64) *block.Segmenter {
	return block.NewSegmenter(p.segmentInterval, modInitBlock, p.BuildStores.ExclusiveEndBlock)
}

func (p *RequestPlan) WriteOutSegmenter() *block.Segmenter {
	return block.NewSegmenter(p.segmentInterval, p.WriteExecOut.StartBlock, p.WriteExecOut.ExclusiveEndBlock)
}

func (p *RequestPlan) String() string {
	return fmt.Sprintf("interval=%d, stores=%s, map_write=%s, map_read=%s, linear=%s", p.segmentInterval, p.BuildStores, p.WriteExecOut, p.ReadExecOut, p.LinearPipeline)
}

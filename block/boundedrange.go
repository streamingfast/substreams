package block

import "github.com/streamingfast/substreams/utils"

// BoundedRange is used to track corresponding ranges in chunks, respecting
// the boundaries
type BoundedRange struct {
	moduleInitBlock          uint64
	interval                 uint64
	requestStartBlock        uint64
	requestExclusiveEndBlock uint64

	*Range
}

func NewBoundedRange(moduleInitBlock, boundInterval, requestStartBlock, requestExclusiveEndBlock uint64) *BoundedRange {
	r := &BoundedRange{
		moduleInitBlock:          moduleInitBlock,
		interval:                 boundInterval,
		requestStartBlock:        requestStartBlock,
		requestExclusiveEndBlock: requestExclusiveEndBlock,
	}
	r.Range = r.computeInitialBounds()
	return r
}

func (r *BoundedRange) NextBoundary() *BoundedRange {
	newBoundedRange := *r
	newBoundedRange.Range = r.computeNextBounds()
	return &newBoundedRange
}

// Whether both sides of the range are aligned with interval boundaries.
func (r *BoundedRange) AlignsWithBoundaries() bool {
	return r.AlignsWithLowerBound() && r.AlignsWithUpperBound()
}

func (r *BoundedRange) IsPartial() bool {
	return !r.AlignsWithUpperBound()
}

func (r *BoundedRange) AlignsWithLowerBound() bool {
	return r.Range.StartBlock%r.interval == 0
}

func (r *BoundedRange) AlignsWithUpperBound() bool {
	return r.Range.ExclusiveEndBlock%r.interval == 0
}

func (r *BoundedRange) computeInitialBounds() *Range {
	if r.requestExclusiveEndBlock < r.moduleInitBlock {
		return nil
	}
	// fixme: simple solution for the production-mode issue
	flooredRequestStartBlock := r.requestStartBlock
	if r.requestStartBlock%r.interval != 0 {
		flooredRequestStartBlock = r.requestStartBlock - r.requestStartBlock%r.interval
		if flooredRequestStartBlock < r.moduleInitBlock {
			flooredRequestStartBlock = r.moduleInitBlock
		}
	}
	lowerBound := utils.MaxOf(
		flooredRequestStartBlock,
		r.moduleInitBlock,
	)
	upperBound := utils.MinOf(
		r.requestStartBlock-r.requestStartBlock%r.interval+r.interval,
		r.requestExclusiveEndBlock,
	)

	return NewRange(lowerBound, upperBound)
}

func (r *BoundedRange) computeNextBounds() *Range {
	lowerBound := r.Range.ExclusiveEndBlock
	upperBound := utils.MinOf(
		r.Range.ExclusiveEndBlock+r.interval,
		r.requestExclusiveEndBlock,
	)
	return NewRange(lowerBound, upperBound)
}

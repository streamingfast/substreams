package block

// BoundedRange is used to track corresponding ranges in chunks, respecting
// the boundaries
type BoundedRange struct {
	segmenter *Segmenter
	segment   int
	*Range
}

func NewBoundedRange(moduleInitBlock, boundInterval, requestStartBlock, requestExclusiveEndBlock uint64) *BoundedRange {
	seg := NewSegmenter(boundInterval, moduleInitBlock, requestExclusiveEndBlock)
	firstSegment := seg.IndexForBlock(requestStartBlock)
	r := &BoundedRange{
		segmenter: seg,
		segment:   firstSegment,
		Range:     seg.Range(firstSegment),
	}
	return r
}

func (r *BoundedRange) NextBoundary() *BoundedRange {
	// FIXME: no need to clone this when we use a Segmenter everywhere
	// and the caller manages the Index
	newBoundedRange := *r
	newBoundedRange.segment++
	newBoundedRange.Range = r.segmenter.Range(r.segment)
	return &newBoundedRange
}

func (r *BoundedRange) IsPartial() bool {
	return r.segmenter.IsPartial(r.segment)
}

//
//func (r *BoundedRange) computeInitialBounds() *Range {
//	if r.requestExclusiveEndBlock < r.moduleInitBlock {
//		return nil
//	}
//
//	floorRequestStartBlock := r.requestStartBlock - r.requestStartBlock%r.interval
//
//	lowerBound := utils.MaxOf(
//		floorRequestStartBlock,
//		r.moduleInitBlock,
//	)
//	upperBound := utils.MinOf(
//		r.requestStartBlock-r.requestStartBlock%r.interval+r.interval,
//		r.requestExclusiveEndBlock,
//	)
//
//	return NewRange(lowerBound, upperBound)
//}
//
//func (r *BoundedRange) computeNextBounds() *Range {
//	lowerBound := r.Range.ExclusiveEndBlock
//	upperBound := utils.MinOf(
//		r.Range.ExclusiveEndBlock+r.interval,
//		r.requestExclusiveEndBlock,
//	)
//	return NewRange(lowerBound, upperBound)
//}

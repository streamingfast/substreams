package block

// SegmentedRange is used to track corresponding ranges in chunks, respecting
// the boundaries
type SegmentedRange struct {
	segmenter *Segmenter
	segment   int
	*Range
}

func NewRawBoundedRange(moduleInitBlock, boundInterval, requestStartBlock, requestExclusiveEndBlock uint64) *SegmentedRange {
	seg := NewSegmenter(boundInterval, moduleInitBlock, requestExclusiveEndBlock)
	return NewBoundedRange(seg, requestStartBlock)
}

func NewBoundedRange(seg *Segmenter, requestStartBlock uint64) *SegmentedRange {
	firstSegment := seg.IndexForBlock(requestStartBlock)
	r := &SegmentedRange{
		segmenter: seg,
		segment:   firstSegment,
		Range:     seg.Range(firstSegment),
	}
	return r
}

func (r *SegmentedRange) NextRange() *SegmentedRange {
	// FIXME: no need to clone this when we use a Segmenter everywhere
	// and the caller manages the Index
	newBoundedRange := *r
	newBoundedRange.segment++
	newBoundedRange.Range = r.segmenter.Range(r.segment)
	return &newBoundedRange
}

func (r *SegmentedRange) IsPartial() bool {
	return r.segmenter.IsPartial(r.segment)
}

package block

// TODO(abourget): The Segmenter is a new SegmentedRange system, that takes an index so
// the caller can always keep track of just one number, and we can obtain the corresponding
// Range for the segment. We can obtain info on the Segment too (if it's Partial, Complete, etc..)

type Segmenter struct {
	interval          uint64
	initialBlock      uint64
	exclusiveEndBlock uint64
}

func NewSegmenter(interval uint64, initialBlock uint64, exclusiveEndBlock uint64) *Segmenter {
	s := &Segmenter{
		interval:          interval,
		initialBlock:      initialBlock,
		exclusiveEndBlock: exclusiveEndBlock,
	}
	return s
}

func (s *Segmenter) InitialBlock() uint64 {
	return s.initialBlock
}

func (s *Segmenter) ExclusiveEndBlock() uint64 {
	return s.exclusiveEndBlock
}

func (s *Segmenter) WithInitialBlock(newInitialBlock uint64) *Segmenter {
	return NewSegmenter(s.interval, newInitialBlock, s.exclusiveEndBlock)
}

func (s *Segmenter) WithExclusiveEndBlock(newExclusiveEndBlock uint64) *Segmenter {
	return NewSegmenter(s.interval, s.initialBlock, newExclusiveEndBlock)
}

// Count returns the number of valid segments for the internal range.
// Use LastIndex to know about the highest index.
func (s *Segmenter) Count() int {
	return int(s.LastIndex() - s.FirstIndex() + 1)
}

func (s *Segmenter) FirstIndex() int {
	initSegment := s.initialBlock / s.interval
	return int(initSegment)
}

func (s *Segmenter) LastIndex() int {
	lastSegment := (s.exclusiveEndBlock - 1) / s.interval
	return int(lastSegment)
}

func (s *Segmenter) Range(idx int) *Range {
	first := s.FirstIndex()
	if idx < first {
		return nil
	}
	if idx == first {
		return s.firstRange()
	}
	return s.followingRange(idx)
}

func (s *Segmenter) firstRange() *Range {
	if s.exclusiveEndBlock != 0 && s.exclusiveEndBlock < s.initialBlock {
		return nil
	}
	floorLowerBound := s.initialBlock - s.initialBlock%s.interval
	upperBound := floorLowerBound + s.interval
	return NewRange(s.initialBlock, min(upperBound, s.exclusiveEndBlock))
}

func (s *Segmenter) followingRange(idx int) *Range {
	if idx > s.LastIndex() {
		return nil
	}
	baseBlock := uint64(idx) * s.interval
	upperBound := baseBlock + s.interval
	return NewRange(baseBlock, min(upperBound, s.exclusiveEndBlock))
}

func (s *Segmenter) IndexForStartBlock(blockNum uint64) int {
	return int(blockNum / s.interval)
}

func (s *Segmenter) IndexForEndBlock(blockNum uint64) int {
	return int((blockNum - 1) / s.interval) /* exclusive of the given blockNum */
}

func (s *Segmenter) EndsOnInterval(segmentIndex int) bool {
	if segmentIndex > s.LastIndex() {
		panic("segment index out of range")
	}
	return s.Range(segmentIndex).ExclusiveEndBlock%s.interval == 0
}

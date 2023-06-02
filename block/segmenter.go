package block

// TODO(abourget): The Segmenter is a new BoundedRange system, that takes an index so
// the caller can always keep track of just one number, and we can obtain the corresponding
// Range for the segment. We can obtain info on the Segment too (if it's Partial, Complete, etc..)

type Segmenter struct {
	interval           uint64
	graphInitBlock     uint64 // Lowest module init block across the requested module graph.
	moduleInitBlock    uint64
	linearHandoffBlock uint64

	countFromBegin      int
	countFromModuleInit int
	firstModuleSegment  int
}

func NewSegmenter(interval uint64, moduleTreeInitBlock uint64, moduleInitBlock uint64, linearHandoffBlock uint64) *Segmenter {
	s := &Segmenter{
		interval:           interval,
		graphInitBlock:     moduleTreeInitBlock,
		moduleInitBlock:    moduleInitBlock,
		linearHandoffBlock: linearHandoffBlock,
	}
	s.countFromBegin = s.computeCountFromBegin()
	s.countFromModuleInit = s.computeCountFromModuleInit()
	s.firstModuleSegment = s.computeFirstModuleSegment()
	return s
}

func (s *Segmenter) CountFromBegin() int      { return s.countFromBegin }
func (s *Segmenter) CountFromModuleInit() int { return s.countFromModuleInit }
func (s *Segmenter) FirstModuleSegment() int  { return s.firstModuleSegment }

func (s *Segmenter) computeCountFromBegin() int {
	graphInitSegment := s.graphInitBlock / s.interval
	handoffSegment := s.linearHandoffBlock / s.interval
	return int(handoffSegment - graphInitSegment + 1)
}

func (s *Segmenter) computeFirstModuleSegment() int {
	graphInitSegment := s.graphInitBlock / s.interval
	modInitSegment := s.moduleInitBlock / s.interval
	return int(modInitSegment - graphInitSegment)
}

func (s *Segmenter) computeCountFromModuleInit() int {
	modInitSegment := s.moduleInitBlock / s.interval
	handoffSegment := s.linearHandoffBlock / s.interval
	return int(handoffSegment - modInitSegment + 1)
}

func (s *Segmenter) Range(idx int) *Range {
	// TODO: handle here the boundary checks, and we want a version with Range()
	// and AlignedRange() which aligns on either one or both sides
	// LowerAlignedRange(), UpperAlignedRange()

	// PORT from boundedrange.go
	return nil
}
func (s *Segmenter) Index(r *Range) int {
	// TODO: implement

	// PORT from boundedrange.go
	return 0
}
func (s *Segmenter) IndexWithBlock(blockNum uint64) int {
	blockSegment := blockNum / s.interval
	treeInitSegment := s.graphInitBlock / s.interval
	return int(blockSegment - treeInitSegment)
}
func (s *Segmenter) IsPartial(segmentIndex int) bool {
	handoffSegment := s.linearHandoffBlock / s.interval
	//if handoffSegment == segmentIndex {
	//return false // FI
	//}
	// TODO: borrow from the BoundedRange's IsPartial functions and boundary checks, etc..

	return false
}

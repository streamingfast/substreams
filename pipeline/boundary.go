package pipeline

type StoreBoundary struct {
	nextBoundary     uint64
	interval         uint64
	isSubRequest     bool
	requestStopBlock uint64
	stopBlockReached bool
}

// TODO(abourget): this constructor ought to take a `config.RuntimeConfig` as a parameter
// so it can decide which field to take.
// Passing one down in the Pipeline::New() seems overkill, as we have all the data to
// create new such objects.  Having it being created so high doesn't properly isolate it.
func NewStoreBoundary(
	interval uint64,
//isSubRequest bool,
	requestStopBlock uint64,
) *StoreBoundary {
	return &StoreBoundary{
		interval: interval,
		//isSubRequest: isSubRequest,
		requestStopBlock: requestStopBlock,
	}
}

func (r *StoreBoundary) OverBoundary(blockNUm uint64) bool {
	return blockNUm >= r.nextBoundary
}

func (r *StoreBoundary) Boundary() uint64 {
	return r.nextBoundary
}

func (r *StoreBoundary) BumpBoundary() {
	if r.stopBlockReached {
		panic("should not be calling bump when stop block has been reached")
	}
	r.nextBoundary = r.computeBoundaryBlock(r.nextBoundary)
}

func (r *StoreBoundary) InitBoundary(blockNum uint64) {
	r.nextBoundary = r.computeBoundaryBlock(blockNum)
}

func (r *StoreBoundary) StopBlockReached() bool {
	return r.stopBlockReached
}

func (r *StoreBoundary) computeBoundaryBlock(atBlockNum uint64) uint64 {
	return atBlockNum - atBlockNum%r.interval + r.interval
	//if r.isSubRequest && r.requestStopBlock != 0 && nextBlock >= r.requestStopBlock {
	//	return r.requestStopBlock
	//}
	//return nextBlock
}

func (r *StoreBoundary) GetStoreFlushRanges(isSubrequest bool, reqStopBlockNum uint64, blockNum uint64) []uint64 {
	out := []uint64{}
	for r.OverBoundary(blockNum) {
		out = append(out, r.nextBoundary)
		r.BumpBoundary()
	}

	if isSubrequest && isStopBlockReached(blockNum, reqStopBlockNum) {
		out = append(out, reqStopBlockNum)
	}

	return out
}

func isStopBlockReached(currentBlock uint64, stopBlock uint64) bool {
	return stopBlock != 0 && currentBlock >= stopBlock
}


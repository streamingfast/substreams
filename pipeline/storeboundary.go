package pipeline

import "sort"

type storeBoundary struct {
	nextBoundary     uint64
	interval         uint64
	isSubRequest     bool
	requestStopBlock uint64
	stopBlockReached bool
}

func NewStoreBoundary(
	interval uint64,
	requestStartBlockNum uint64,
	requestStopBlock uint64,
) *storeBoundary {
	b := &storeBoundary{
		interval:         interval,
		requestStopBlock: requestStopBlock,
	}
	b.InitNextBoundary(requestStartBlockNum)
	return b
}

func (r *storeBoundary) OverBoundary(blockNum uint64) bool {
	return blockNum >= r.nextBoundary
}

func (r *storeBoundary) BumpBoundary() {
	if r.stopBlockReached {
		panic("should not be calling bump when stop block has been reached")
	}
	r.nextBoundary = r.computeBoundaryBlock(r.nextBoundary)
}

func (r *storeBoundary) computeBoundaryBlock(atBlockNum uint64) uint64 {
	return atBlockNum - atBlockNum%r.interval + r.interval
}

func (r *storeBoundary) InitNextBoundary(blockNum uint64) {
	r.nextBoundary = r.computeBoundaryBlock(blockNum)
}

func (r *storeBoundary) GetStoreFlushRanges(isSubRequest bool, reqStopBlockNum uint64, blockNum uint64) []uint64 {
	boundaries := map[uint64]bool{}

	for r.OverBoundary(blockNum) {
		boundaries[r.nextBoundary] = true
		r.BumpBoundary()
		if isBlockOverStopBlock(r.nextBoundary, reqStopBlockNum) {
			break
		}
	}

	if isSubRequest && isBlockOverStopBlock(blockNum, reqStopBlockNum) {
		boundaries[reqStopBlockNum] = true
	}

	out := []uint64{}
	for v, _ := range boundaries {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i] < out[j]
	})
	return out
}

func isBlockOverStopBlock(currentBlock uint64, stopBlock uint64) bool {
	return stopBlock != 0 && currentBlock >= stopBlock
}

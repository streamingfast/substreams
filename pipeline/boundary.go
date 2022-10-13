package pipeline

type StoreBoundary struct {
	nextBoundary uint64
	interval     uint64
}

func NewStoreBoundary(interval uint64) *StoreBoundary {
	return &StoreBoundary{
		interval: interval,
	}
}

func (r *StoreBoundary) PassedBoundary(blockNUm uint64) bool {
	return r.nextBoundary <= blockNUm
}

func (r *StoreBoundary) Boundary() uint64 {
	return r.nextBoundary
}

func (r *StoreBoundary) BumpBoundary() {
	r.nextBoundary = r.computeBoundaryBlock(r.nextBoundary)
}

func (r *StoreBoundary) InitBoundary(blockNum uint64) {
	r.nextBoundary = r.computeBoundaryBlock(blockNum)
}

func (r *StoreBoundary) computeBoundaryBlock(atBlockNum uint64) uint64 {
	return atBlockNum - atBlockNum%r.interval + r.interval
}

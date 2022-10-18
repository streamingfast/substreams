package pipeline

type StoreBoundary struct {
	nextBoundary uint64
	interval     uint64
}

// TODO(abourget): this constructor ought to take a `config.RuntimeConfig` as a parameter
// so it can decide which field to take.
// Passing one down in the Pipeline::New() seems overkill, as we have all the data to
// create new such objects.  Having it being created so high doesn't properly isolate it.
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

package orchestrator

import (
	"github.com/streamingfast/substreams/block"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

type WorkPlan map[string]*WorkUnit

func (p WorkPlan) ProgressMessages() (out []*pbsubstreams.ModuleProgress) {
	for storeName, unit := range p {
		if unit.completedRange == nil {
			continue
		}
		// TODO(abourget): also send the `partialsPresent` messages, along with the loadInitialStore
		out = append(out, &pbsubstreams.ModuleProgress{
			Name: storeName,
			Type: &pbsubstreams.ModuleProgress_ProcessedRanges{
				ProcessedRanges: &pbsubstreams.ModuleProgress_ProcessedRange{
					ProcessedRanges: []*pbsubstreams.BlockRange{
						{
							StartBlock: unit.completedRange.StartBlock,
							EndBlock:   unit.completedRange.ExclusiveEndBlock,
						},
					},
				},
			},
		})
	}
	return
}

type WorkUnit struct {
	modName string

	// TODO(abourget): re-rename this to `loadInitialStore`,
	// `partialsPresent` alongside `loadInitialStore` provide all the
	// info for sending progress notifications. loadInitialStore has a
	// single purpose: compute what's necessary to initialize the
	// store to get started.
	completedRange *block.Range // Send a Progress message, saying the store is already processed for this range

	partialsMissing block.Ranges

	// TODO(abourget): To be fed into the Squasher, primed with those
	// partials that already exist, also can be Merged() and sent to
	// the end user so they know those segments have been processed
	// already.  Please don't remove the comment until it is
	// implemented (!)
	partialsPresent block.Ranges

	subRequestSplitSize uint64
	RequestRanges       block.Ranges
}

func (w *WorkUnit) InitialProcessedPartials() block.Ranges {
	//TODO(abourget): make sure we call this when the time comes to send Progress messages initially.
	return w.partialsPresent.Merged()
}

func SplitWork(modName string, subRequestSlipSize, modInitBlock, incomingReqStartBlock uint64, snapshots *Snapshots) (work *WorkUnit) {
	work = &WorkUnit{modName: modName, subRequestSplitSize: subRequestSlipSize}

	if incomingReqStartBlock <= modInitBlock {
		return work
	}

	storeLastComplete := snapshots.LastCompletedBlockBefore(incomingReqStartBlock)

	if storeLastComplete != 0 && storeLastComplete <= modInitBlock {
		panic("cannot have saved last store before module's init block") // 0 has special meaning
	}

	backProcessStartBlock := modInitBlock
	if storeLastComplete != 0 {
		backProcessStartBlock = storeLastComplete
		work.completedRange = block.NewRange(modInitBlock, storeLastComplete)
	}

	if storeLastComplete == incomingReqStartBlock {
		return
	}

	// TODO(abourget): move out again
	minOf := func(a, b uint64) uint64 {
		if a < b {
			return a
		}
		return b
	}

	for ptr := backProcessStartBlock; ptr < incomingReqStartBlock; {
		// FIXME(abourget): this ultra-simplified line, based on the storeSplit solved two issues:
		// * always lining on a store boundary, or
		// * on the incoming request boundary
		// It is then queried against the SNAPSHOTS, those snapshots that are lined up against
		// the STORE split size, and not the request's.
		end := minOf(ptr-ptr%subRequestSlipSize+subRequestSlipSize, incomingReqStartBlock)
		newPartial := block.NewRange(ptr, end)
		if !snapshots.ContainsPartial(newPartial) {
			work.partialsMissing = append(work.partialsMissing, newPartial)
		} else {
			work.partialsPresent = append(work.partialsPresent, newPartial)
		}
		ptr = end
	}

	// FIXME(abourget): this is why this was in a separate function, because the unit tests can
	// figure out what stores are to be expected to be produced everywhere, without regards to how
	// they would be batched for execution, or split at execution.
	//
	// Then, a SEPARATE function could batch the partial stores production into requests,
	// and that ended up being a simple MergedBins() call, and that was already well tested
	//
	// The only concern of the Work Planner, was therefore to align
	// individual _stores_, and not the requests really. It is even
	// possible to think of an orchestrator that doesn't even have the
	// same store split configuration as its backprocessing nodes, and
	// provided the backprocess node respects the boundaries, and
	// produces stuff, it will return the material needed by the
	// orchestrator to satisfy its upstream request. This makes things
	// much more reliable: you can restart and change the split sizes
	// in the different backends without worries.
	work.RequestRanges = work.partialsMissing.MergeRanges(work.subRequestSplitSize)
	return work
}

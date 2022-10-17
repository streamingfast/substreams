package orchestrator

import "github.com/streamingfast/substreams/block"

type WorkUnit struct {
	modName string

	initialCompleteRange *block.Range // Points to a complete .kv file, to initialize the store upon getting started.
	partialsMissing      block.Ranges
	partialsPresent      block.Ranges
}

func (w *WorkUnit) initialProcessedPartials() block.Ranges {
	return w.partialsPresent.Merged()
}

func SplitWork(modName string, storeSaveInterval, modInitBlock, reqEffectiveStartBlock uint64, snapshots *Snapshots) *WorkUnit {
	work := &WorkUnit{modName: modName}

	if reqEffectiveStartBlock <= modInitBlock {
		return work
	}

	completeSnapshot := snapshots.LastCompleteSnapshotBefore(reqEffectiveStartBlock)

	if completeSnapshot != nil && completeSnapshot.ExclusiveEndBlock <= modInitBlock {
		panic("cannot have saved last store before module's init block") // 0 has special meaning
	}

	backProcessStartBlock := modInitBlock
	if completeSnapshot != nil {
		backProcessStartBlock = completeSnapshot.ExclusiveEndBlock
		work.initialCompleteRange = block.NewRange(modInitBlock, completeSnapshot.ExclusiveEndBlock)

		if completeSnapshot.ExclusiveEndBlock == reqEffectiveStartBlock {
			return work
		}
	}

	for ptr := backProcessStartBlock; ptr < reqEffectiveStartBlock; {
		end := minOf(ptr-ptr%storeSaveInterval+storeSaveInterval, reqEffectiveStartBlock)
		newPartial := block.NewRange(ptr, end)
		if !snapshots.ContainsPartial(newPartial) {
			work.partialsMissing = append(work.partialsMissing, newPartial)
		} else {
			work.partialsPresent = append(work.partialsPresent, newPartial)
		}
		ptr = end
	}

	return work

}
func (w *WorkUnit) batchRequests(subreqSplitSize uint64) block.Ranges {
	ranges := w.partialsMissing.MergedBuckets(subreqSplitSize)
	return ranges

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
}

func minOf(a, b uint64) uint64 {
	if a < b {
		return a
	}
	return b
}

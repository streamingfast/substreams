package orchestrator

import (
	"fmt"
	"github.com/streamingfast/substreams/block"
)

// WorkUnits contains all the ranges of things we'll want to plan work for, and things that are already available.
type WorkUnits struct {
	modName string

	initialCompleteRange *block.Range // Points to a complete .kv file, to initialize the store upon getting started.
	partialsMissing      block.Ranges
	partialsPresent      block.Ranges
}

func (w *WorkUnits) init(storeSaveInterval, modInitBlock, workUpToBlockNum uint64, snapshots *Snapshots) error {
	if workUpToBlockNum <= modInitBlock {
		return nil
	}

	completeSnapshot := snapshots.LastCompleteSnapshotBefore(workUpToBlockNum)

	if completeSnapshot != nil && completeSnapshot.ExclusiveEndBlock <= modInitBlock {
		return fmt.Errorf("cannot have saved last store before module's init block")
	}

	backProcessStartBlock := modInitBlock
	if completeSnapshot != nil {
		backProcessStartBlock = completeSnapshot.ExclusiveEndBlock
		w.initialCompleteRange = block.NewRange(modInitBlock, completeSnapshot.ExclusiveEndBlock)

		if completeSnapshot.ExclusiveEndBlock == workUpToBlockNum {
			return nil
		}
	}

	for ptr := backProcessStartBlock; ptr < workUpToBlockNum; {
		end := minOf(ptr-ptr%storeSaveInterval+storeSaveInterval, workUpToBlockNum)
		newPartial := block.NewRange(ptr, end)
		if !snapshots.ContainsPartial(newPartial) {
			w.partialsMissing = append(w.partialsMissing, newPartial)
		} else {
			w.partialsPresent = append(w.partialsPresent, newPartial)
		}
		ptr = end
	}
	return nil
}

func (w *WorkUnits) initialProcessedPartials() block.Ranges {
	return w.partialsPresent.Merged()
}

func (w *WorkUnits) batchRequests(subreqSplitSize uint64) block.Ranges {
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

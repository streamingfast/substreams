package orchestrator

import (
	"context"
	"fmt"

	"github.com/streamingfast/substreams/block"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

type WorkPlan map[string]*WorkUnit

func (p WorkPlan) SquashPartialsPresent(ctx context.Context, squasher *Squasher) error {
	for _, w := range p {
		err := squasher.Squash(ctx, w.modName, w.partialsPresent)
		if err != nil {
			return fmt.Errorf("squash partials present for module %s: %w", w.modName, err)
		}
	}
	return nil
}

func (p WorkPlan) ProgressMessages() (out []*pbsubstreams.ModuleProgress) {
	for storeName, unit := range p {
		if unit.initialStoreFile == nil {
			continue
		}

		var more []*pbsubstreams.BlockRange
		if unit.initialStoreFile != nil {
			more = append(more, &pbsubstreams.BlockRange{
				StartBlock: unit.initialStoreFile.StartBlock,
				EndBlock:   unit.initialStoreFile.ExclusiveEndBlock,
			})
		}

		for _, rng := range unit.initialProcessedPartials() {
			more = append(more, &pbsubstreams.BlockRange{
				StartBlock: rng.StartBlock,
				EndBlock:   rng.ExclusiveEndBlock,
			})
		}

		out = append(out, &pbsubstreams.ModuleProgress{
			Name: storeName,
			Type: &pbsubstreams.ModuleProgress_ProcessedRanges{
				ProcessedRanges: &pbsubstreams.ModuleProgress_ProcessedRange{
					ProcessedRanges: more,
				},
			},
		})
	}
	return
}

type WorkUnit struct {
	modName string

	initialStoreFile *block.Range // Points to a complete .kv file, to initialize the store upon getting started.
	partialsMissing  block.Ranges
	partialsPresent  block.Ranges
}

func (w *WorkUnit) initialProcessedPartials() block.Ranges {
	return w.partialsPresent.Merged()
}

func SplitWork(modName string, storeSaveInterval, modInitBlock, incomingReqStartBlock uint64, snapshots *Snapshots) (work *WorkUnit) {
	work = &WorkUnit{modName: modName}

	if incomingReqStartBlock <= modInitBlock {
		return work
	}

	completeSnapshot := snapshots.LastCompleteSnapshotBefore(incomingReqStartBlock)

	if completeSnapshot != nil && completeSnapshot.ExclusiveEndBlock <= modInitBlock {
		panic("cannot have saved last store before module's init block") // 0 has special meaning
	}

	backProcessStartBlock := modInitBlock
	if completeSnapshot != nil {
		backProcessStartBlock = completeSnapshot.ExclusiveEndBlock
		work.initialStoreFile = block.NewRange(modInitBlock, completeSnapshot.ExclusiveEndBlock)
	}

	if completeSnapshot != nil && completeSnapshot.ExclusiveEndBlock == incomingReqStartBlock {
		return
	}

	for ptr := backProcessStartBlock; ptr < incomingReqStartBlock; {
		end := minOf(ptr-ptr%storeSaveInterval+storeSaveInterval, incomingReqStartBlock)
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
func (w *WorkUnit) batchRequests(subreqSplit uint64) block.Ranges {
	return w.partialsMissing.MergedBuckets(subreqSplit)

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

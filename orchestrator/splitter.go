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

	completedRange *block.Range // Send a Progress message, saying the store is already processed for this range

	partialsMissing block.Ranges
	partialsPresent block.Ranges

	subRequestSplitSize uint64
	RequestRanges       block.Ranges
}

func (w *WorkUnit) InitialProcessedPartials() block.Ranges {
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

	minOf := func(a, b uint64) uint64 {
		if a < b {
			return a
		}
		return b
	}

	for ptr := backProcessStartBlock; ptr < incomingReqStartBlock; {
		end := minOf(ptr-ptr%subRequestSlipSize+subRequestSlipSize, incomingReqStartBlock)
		newPartial := block.NewRange(ptr, end)
		if !snapshots.ContainsPartial(newPartial) {
			work.partialsMissing = append(work.partialsMissing, newPartial)
		} else {
			work.partialsPresent = append(work.partialsPresent, newPartial)
		}
		ptr = end
	}
	work.RequestRanges = work.partialsMissing.MergeRanges(work.subRequestSplitSize)
	return work
}

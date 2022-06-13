package orchestrator

import (
	"fmt"
	"strings"

	"github.com/streamingfast/substreams/block"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

// FIXME(abourget): WorkPlan ?

type SplitWorkModules map[string]*SplitWork

func (mods SplitWorkModules) ProgressMessages() (out []*pbsubstreams.ModuleProgress) {
	for storeName, work := range mods {
		if work.loadInitialStore == nil {
			continue
		}
		out = append(out, &pbsubstreams.ModuleProgress{
			Name: storeName,
			Type: &pbsubstreams.ModuleProgress_ProcessedRanges{
				ProcessedRanges: &pbsubstreams.ModuleProgress_ProcessedRange{
					ProcessedRanges: []*pbsubstreams.BlockRange{
						{
							StartBlock: work.loadInitialStore.StartBlock,
							EndBlock:   work.loadInitialStore.ExclusiveEndBlock,
						},
					},
				},
			},
		})
	}
	return
}

// FIXME(abourget): StoreWorkUnit ?
type SplitWork struct {
	modName          string
	loadInitialStore *block.Range // Send a Progress message, saying the store is already processed for this range
	partialsMissing  block.Ranges // Used to prep the reqChunks
	partialsPresent  block.Ranges // To be fed into the Squasher, primed with those partials that already exist, also can be Merged() and sent to the end user so they know those segments have been processed already.

	reqChunks reqChunks // All jobs that needs to be scheduled
}

func SplitSomeWork(modName string, subReqSplit, modInitBlock, incomingReqStartBlock uint64, snapshots *Snapshots) (work *SplitWork) {
	work = &SplitWork{modName: modName}

	if incomingReqStartBlock <= modInitBlock {
		return work
	}

	storeLastComplete := snapshots.LastCompleteBefore(incomingReqStartBlock)

	if storeLastComplete != 0 && storeLastComplete <= modInitBlock {
		panic("cannot have saved last store before module's init block") // 0 has special meaning
	}

	backprocessStartBlock := modInitBlock
	if storeLastComplete != 0 {
		backprocessStartBlock = storeLastComplete
		work.loadInitialStore = block.NewRange(modInitBlock, storeLastComplete)
	}

	if storeLastComplete == incomingReqStartBlock {
		return
	}

	for ptr := backprocessStartBlock; ptr < incomingReqStartBlock; {
		end := minOf(ptr-ptr%subReqSplit+subReqSplit, incomingReqStartBlock)
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

// What to send to end-users to inform them of already processed segments
func (work *SplitWork) InitialProcessedPartials() block.Ranges {
	return work.partialsPresent.Merged()
}

//
//func (work *SplitWork) computeRequests(storesSaveInterval, subReqSplit uint64) {
//
//	requestRanges := work.partialsMissing.MergeRange(subReqSplit)
//	for _, r := range requestRanges {
//		expectedPartials := r.Split(storesSaveInterval)
//		var reqChunks []*chunk
//
//		for _, p := range expectedPartials {
//			reqChunks = append(reqChunks, &chunk{
//				start:       p.StartBlock,
//				end:         p.ExclusiveEndBlock,
//				tempPartial: p.ExclusiveEndBlock%storesSaveInterval != 0,
//			})
//		}
//
//		work.reqChunks = append(work.reqChunks, &reqChunk{
//			start:  r.StartBlock,
//			end:    r.ExclusiveEndBlock,
//			reqChunks: reqChunks,
//		})
//	}
//}

// 	// subReqStartBlock := computeStoreExclusiveEndBlock(storeLastComplete, incomingReqStartBlock, storeSplit, modInitBlock)
// 	// if storeLastComplete != 0 && storeLastComplete != modInitBlock && subReqStartBlock != 0 {
// 	// 	work.loadInitialStore = block.NewRange(modInitBlock, subReqStartBlock)
// 	// }

// 	// if subReqStartBlock == incomingReqStartBlock {
// 	// 	return
// 	// }

// 	requestRanges := block.NewRange(subReqStartBlock, incomingReqStartBlock).Split(subReqSplit)

// 	for _, reqRange := range requestRanges {
// 		reqChunk := &reqChunk{start: reqRange.StartBlock, end: reqRange.ExclusiveEndBlock}
// 		// Now do the SECOND split, for reqChunks for `storeSplit`
// 		storeSplitRanges := reqRange.Split(storeSplit)
// 		for _, storeSplitRange := range storeSplitRanges {
// 			if storeSplitRange.StartBlock < modInitBlock {
// 				panic(fmt.Sprintf("module %q: received a squash request for a start block %d prior to the module's initial block %d", modName, storeSplitRange.StartBlock, modInitBlock))
// 			}

// 			if snapshots.ContainsPartial(storeSplitRange) {
// 				continue
// 			}

// 			// FIXME(abourget): check this one again
// 			// if reqRange.ExclusiveEndBlock < s.store.StoreInitialBlock {
// 			// 	// Otherwise, risks stalling the merging (as ranges are
// 			// 	// sorted, and only the first is checked for contiguousness)
// 			// 	continue
// 			// }
// 			addStoreChunk := &chunk{
// 				start:       storeSplitRange.StartBlock,
// 				end:         storeSplitRange.ExclusiveEndBlock,
// 				tempPartial: storeSplitRange.ExclusiveEndBlock%storeSplit != 0,
// 			}
// 			reqChunk.reqChunks = append(reqChunk.reqChunks, addStoreChunk)
// 		}
// 		work.reqChunks = append(work.reqChunks, reqChunk)
// 	}

// 	return
// }

func minOf(a, b uint64) uint64 {
	if a < b {
		return a
	}
	return b
}

// // computeStoreExclusiveEndBlock tells us WHERE we have a snapshot ready that can be queried in the conditions of this query.
// func computeStoreExclusiveEndBlock(lastSavedBlock, reqStartBlock, saveInterval, moduleInitialBlock uint64) uint64 {
// 	previousBoundary := reqStartBlock - reqStartBlock%saveInterval
// 	startBlockOnBoundary := reqStartBlock%saveInterval == 0

// 	if reqStartBlock >= lastSavedBlock {
// 		return lastSavedBlock
// 	} else if previousBoundary < moduleInitialBlock {
// 		return 0
// 	} else if startBlockOnBoundary {
// 		return reqStartBlock
// 	}
// 	return previousBoundary
// }
type reqChunks []*reqChunk

type reqChunk struct {
	start uint64
	end   uint64 // exclusive end

}

func (c reqChunk) String() string {
	out := fmt.Sprintf("%d-%d", c.start, c.end)
	return out
}

type chunks []*chunk

func (c chunks) String() string {
	var sc []string
	for _, s := range c {
		var add string
		if s.tempPartial {
			add = "TMP:"
		}
		sc = append(sc, fmt.Sprintf("%s%d-%d", add, s.start, s.end))
	}
	return strings.Join(sc, ", ")
}

type chunk struct {
	start       uint64
	end         uint64 // exclusive end
	tempPartial bool   // for off-of-bound stores (like ending in 1123, and not on 1000)
}

func (s chunk) String() string {
	var add string
	if s.tempPartial {
		add = "TMP:"
	}
	return fmt.Sprintf("%s%d-%d", add, s.start, s.end)
}

/////////////////////////////////////////////////////////////////////////////////

type Splitter struct {
	chunkSize uint64
}

func NewSplitter(chunkSize uint64) *Splitter {
	// The splitter should accomodate and produce the outgoing subrequests necessary for the
	// the incoming request to be satisfied, and for a squasher to know what to expect and do its
	// squashing job, knowing what the subrequests should produce
	return &Splitter{
		chunkSize: chunkSize,
	}
}

func (s *Splitter) Split(moduleInitialBlock uint64, lastSavedBlock uint64, blockRange *block.Range) []*block.Range {
	if moduleInitialBlock > blockRange.StartBlock {
		blockRange.StartBlock = moduleInitialBlock
	}

	if lastSavedBlock > blockRange.StartBlock {
		blockRange.StartBlock = lastSavedBlock
	}

	return blockRange.Split(s.chunkSize)
}

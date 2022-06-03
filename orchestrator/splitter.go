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
	reqChunks        []*reqChunk  // All jobs that needs to be scheduled
}

func SplitSomeWork(modName string, storeSplit, subreqSplit, modInitBlock, storeLastBlock, incomingReqStartBlock uint64) (out *SplitWork) {
	// FIXME: Make sure `storeSplit` and `subreqSplit` are a multiple of one another.
	// storeSplit must actually be a factor of subreqSplit
	// panic otherwise, and bring that check higher up the chain

	out = &SplitWork{modName: modName}

	if storeLastBlock != 0 && storeLastBlock < modInitBlock {
		panic("cannot have saved last store before module's init block") // 0 has special meaning
	}

	if incomingReqStartBlock <= modInitBlock {
		return out
	}

	subreqStartBlock := computeStoreExclusiveEndBlock(storeLastBlock, incomingReqStartBlock, storeSplit, modInitBlock)
	if storeLastBlock != 0 && storeLastBlock != modInitBlock && subreqStartBlock != 0 {
		out.loadInitialStore = block.NewRange(modInitBlock, subreqStartBlock)
	}

	if subreqStartBlock == incomingReqStartBlock {
		return
	}
	if subreqStartBlock == 0 {
		subreqStartBlock = modInitBlock
	}

	requestRanges := block.NewRange(subreqStartBlock, incomingReqStartBlock).Split(subreqSplit)

	for _, reqRange := range requestRanges {
		addReqChunk := &reqChunk{start: reqRange.StartBlock, end: reqRange.ExclusiveEndBlock}
		// Now do the SECOND split, for chunks for `storeSplit`
		storeSplitRanges := reqRange.Split(storeSplit)
		for _, storeSplitRange := range storeSplitRanges {
			if storeSplitRange.StartBlock < modInitBlock {
				panic(fmt.Sprintf("module %q: received a squash request for a start block %d prior to the module's initial block %d", modName, storeSplitRange.StartBlock, modInitBlock))
			}
			// FIXME(abourget): check this one again
			// if reqRange.ExclusiveEndBlock < s.store.StoreInitialBlock {
			// 	// Otherwise, risks stalling the merging (as ranges are
			// 	// sorted, and only the first is checked for contiguousness)
			// 	continue
			// }
			addStoreChunk := &storeChunk{
				start:       storeSplitRange.StartBlock,
				end:         storeSplitRange.ExclusiveEndBlock,
				tempPartial: storeSplitRange.ExclusiveEndBlock%storeSplit != 0,
			}
			addReqChunk.storeChunks = append(addReqChunk.storeChunks, addStoreChunk)
		}
		out.reqChunks = append(out.reqChunks, addReqChunk)
	}

	return
}

// computeStoreExclusiveEndBlock tells us WHERE we have a snapshot ready that can be queried in the conditions of this query.
func computeStoreExclusiveEndBlock(lastSavedBlock, reqStartBlock, saveInterval, moduleInitialBlock uint64) uint64 {
	previousBoundary := reqStartBlock - reqStartBlock%saveInterval
	startBlockOnBoundary := reqStartBlock%saveInterval == 0

	if reqStartBlock >= lastSavedBlock {
		return lastSavedBlock
	} else if previousBoundary < moduleInitialBlock {
		return 0
	} else if startBlockOnBoundary {
		return reqStartBlock
	}
	return previousBoundary
}

type reqChunk struct {
	start uint64
	end   uint64 // exclusive end

	storeChunks []*storeChunk // All partial stores that are expected to be produced by this subreq
}

func (c reqChunk) String() string {
	var sc []string
	for _, s := range c.storeChunks {
		var add string
		if s.tempPartial {
			add = "TMP:"
		}
		sc = append(sc, fmt.Sprintf("%s%d-%d", add, s.start, s.end))
	}
	out := fmt.Sprintf("%d-%d", c.start, c.end)
	if len(sc) != 0 {
		out += " (" + strings.Join(sc, ", ") + ")"
	}
	out = strings.Replace(out, fmt.Sprintf(" (%d-%d)", c.start, c.end), "", 1)
	return out
}

type storeChunk struct {
	start       uint64
	end         uint64 // exclusive end
	tempPartial bool   // for off-of-bound stores (like ending in 1123, and not on 1000)
}

func (s storeChunk) String() string {
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

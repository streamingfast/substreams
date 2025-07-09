package sink

import (
	"fmt"

	"github.com/streamingfast/bstream"
	pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

// blockDataBuffer is expected to be used synchronously from a single
// goroutine, concurrent access is **not** implemented.
//
// When you see "final" in the context of this implementation, we talk
// about our internal "fake" finality which happens once enough block
// has "passed" which is the size of the undo block size
type blockDataBuffer struct {
	// data is kept striclty ordered and its ordering must be respected
	// throughout the implementation.
	//
	// The array is always fully allocated, but the `dataEmptyAt` determines
	// where we have actual unallocated space.
	data []*pbsubstreamsrpc.BlockScopedData

	// We manage manually the offset at which we have space to add new block.
	dataEmptyAt int

	lastEmittedBlock bstream.BlockRef
}

func newBlockDataBuffer(size int) *blockDataBuffer {
	return &blockDataBuffer{
		data:             make([]*pbsubstreamsrpc.BlockScopedData, size),
		dataEmptyAt:      0,
		lastEmittedBlock: nil,
	}
}

func (b *blockDataBuffer) HandleBlockScopedData(blockData *pbsubstreamsrpc.BlockScopedData) (finalBlocks []*pbsubstreamsrpc.BlockScopedData, err error) {
	// We have one element already in, validate that received block is strictly ordered
	if b.dataEmptyAt != 0 {
		highestBlock := b.data[b.dataEmptyAt-1]
		if blockData.Clock.Number <= highestBlock.Clock.Number {
			return nil, fmt.Errorf("received new block scoped data (Block %s) whose height is lower or equal than our most recent block (Block %s)", blockToRef(blockData), blockToRef(highestBlock))
		}
	}

	lastFinalBlockAt := b.findOldestFinalBlockIndex(blockData.FinalBlockHeight)

	dataContainsFinalBlocks := lastFinalBlockAt != -1
	isFullCapacity := b.dataEmptyAt == len(b.data)

	if isFullCapacity || dataContainsFinalBlocks {
		// We are at full capacity and no block is final now, assume oldest block is now final
		if lastFinalBlockAt == -1 {
			lastFinalBlockAt = 0
		}

		// We perform a copy of the array slice because we are about to shift it, modifying it in-place, we add plus 1 to allow received blockData itself to be added if needed
		finalBlockCount := lastFinalBlockAt + 1
		finalBlocks = make([]*pbsubstreamsrpc.BlockScopedData, 0, finalBlockCount+1)
		finalBlocks = append(finalBlocks, b.data[0:finalBlockCount]...)

		// Shifts non-final blocks at the beginning of the array
		for i := lastFinalBlockAt + 1; i < b.dataEmptyAt; i++ {
			b.data[i-finalBlockCount] = b.data[i]
		}

		b.dataEmptyAt = b.dataEmptyAt - finalBlockCount
		b.lastEmittedBlock = blockToRef(finalBlocks[len(finalBlocks)-1])
	}

	// If the block received is itself final, we add it to the final blocks array, otherwise, we process as normal
	isReceivedBlockFinal := blockData.Clock.Number <= blockData.FinalBlockHeight

	if isReceivedBlockFinal {
		finalBlocks = append(finalBlocks, blockData)
	} else {
		b.data[b.dataEmptyAt] = blockData
		b.dataEmptyAt += 1
	}

	return finalBlocks, nil
}

func (b *blockDataBuffer) HandleBlockUndoSignal(undoSignal *pbsubstreamsrpc.BlockUndoSignal) error {
	lastValidBlock := asBlockRef(undoSignal.LastValidBlock)

	if b.lastEmittedBlock != nil && b.lastEmittedBlock.Num() >= lastValidBlock.Num() {
		// We might have actually sent exactly the last valid block, in which case no error should occur since the chain
		// ordering is respected
		if !bstream.EqualsBlockRefs(b.lastEmittedBlock, lastValidBlock) {
			return fmt.Errorf("cannot undo down to last valid Block %s because we already sent you Block %s which is after last valid block", lastValidBlock, b.lastEmittedBlock)
		}
	}

	// There is nothing to do, we are already emptied due to a previous undo signal
	if b.dataEmptyAt == 0 {
		return nil
	}

	// If last valid block is not found, `dataEmptyAt` will become 0 (-1 + 1) which "clears" all our block
	b.dataEmptyAt = b.findNewestValidBlockIndex(lastValidBlock.Num()) + 1
	return nil
}

func (b *blockDataBuffer) findNewestValidBlockIndex(lastValidBlockHeight uint64) int {
	newestValidBlockAt := -1
	for i := b.dataEmptyAt - 1; i >= 0; i-- {
		if b.data[i].Clock.Number <= lastValidBlockHeight {
			newestValidBlockAt = i
			break
		}
	}

	return newestValidBlockAt
}

func (b *blockDataBuffer) findOldestFinalBlockIndex(finalHeight uint64) int {
	oldestFinalBlockAt := -1
	for i := 0; i < b.dataEmptyAt; i++ {
		if finalHeight < b.data[i].Clock.Number {
			break
		}

		oldestFinalBlockAt = i
	}

	return oldestFinalBlockAt
}

func (b *blockDataBuffer) Capacity() int {
	return len(b.data)
}

func (b *blockDataBuffer) String() string {
	if b == nil {
		return "None"
	}

	return fmt.Sprintf("Buffering (%d blocks)", len(b.data))
}

func blockToRef(blockScopedData *pbsubstreamsrpc.BlockScopedData) bstream.BlockRef {
	return clockToBlockRef(blockScopedData.Clock)
}

func asBlockRef(blockRef *pbsubstreams.BlockRef) bstream.BlockRef {
	return bstream.NewBlockRef(blockRef.Id, blockRef.Number)
}

func clockToBlockRef(clock *pbsubstreams.Clock) bstream.BlockRef {
	return bstream.NewBlockRef(clock.Id, clock.Number)
}

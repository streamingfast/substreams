package state

import (
	"github.com/streamingfast/substreams/block"
	"github.com/streamingfast/substreams/storage/execout"
	"github.com/streamingfast/substreams/utils"
)

// WARN: this was never used? We didn't do anything with it, and the new scheduler
// will simply walk the files. In the future, we might pluck those files
// and move on.. but we'll see them. It won't look like this at all.
type ExecOutputStorageState struct {
	ModuleName         string
	ModuleInitialBlock uint64

	SegmentsPresent block.Ranges
	SegmentsMissing block.Ranges
}

func (m ExecOutputStorageState) Name() string { return m.ModuleName }
func (m ExecOutputStorageState) InitialProgressRanges() block.Ranges {
	return m.SegmentsPresent.Merged()
}
func (m ExecOutputStorageState) ReadyUpToBlock() uint64 {
	if len(m.SegmentsMissing) != 0 {
		return m.SegmentsMissing[0].StartBlock
	}
	if l := len(m.SegmentsPresent); l != 0 {
		return m.SegmentsPresent[l-1].ExclusiveEndBlock
	}
	return m.ModuleInitialBlock
}

func (m ExecOutputStorageState) BatchRequests(subRequestSplitSize uint64) block.Ranges {
	return m.SegmentsMissing.MergedBuckets(subRequestSplitSize)
}

func NewExecOutputStorageState(config *execout.Config, saveInterval, requestStartBlock, linearHandoffBlock uint64, snapshots block.Ranges) (out *ExecOutputStorageState, err error) {
	modInitBlock := config.ModuleInitialBlock()
	out = &ExecOutputStorageState{
		ModuleName:         config.Name(),
		ModuleInitialBlock: modInitBlock,
	}

	if linearHandoffBlock <= modInitBlock {
		return
	}
	// fixme: simple solution for the production-mode issue
	if requestStartBlock%saveInterval != 0 {
		requestStartBlock = requestStartBlock - requestStartBlock%saveInterval
		if requestStartBlock < modInitBlock {
			requestStartBlock = modInitBlock
		}
	}
	// TODO(abourget): this is the logic of the Segmenter, building off segments.
	// we're probably doing that multiple places.
	for ptr := requestStartBlock; ptr < linearHandoffBlock; {
		end := utils.MinOf(ptr-ptr%saveInterval+saveInterval, linearHandoffBlock)
		blockRange := block.NewRange(ptr, end)
		if !snapshots.Contains(blockRange) {
			out.SegmentsMissing = append(out.SegmentsMissing, blockRange)
		} else {
			out.SegmentsPresent = append(out.SegmentsPresent, blockRange)
		}
		ptr = end
	}

	return
}

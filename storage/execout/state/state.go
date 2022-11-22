package state

import (
	"github.com/streamingfast/substreams/block"
	"github.com/streamingfast/substreams/storage/execout"
	"github.com/streamingfast/substreams/utils"
)

type ExecOutputStorageState struct {
	ModuleName string

	SegmentsPresent block.Ranges
	SegmentsMissing block.Ranges
}

func (m ExecOutputStorageState) Name() string                        { return m.ModuleName }
func (m ExecOutputStorageState) InitialProgressRanges() block.Ranges { return nil }
func (m ExecOutputStorageState) ReadyUpToBlock() uint64              { return 0 }

func (m ExecOutputStorageState) BatchRequests(subRequestSplitSize uint64) block.Ranges {
	return m.SegmentsMissing.MergedBuckets(subRequestSplitSize)
}

func NewExecOutputStorageState(config *execout.Config, saveInterval, startBlock, workUpToBlockNum uint64, snapshots block.Ranges) (out *ExecOutputStorageState, err error) {
	modInitBlock := config.ModuleInitialBlock()
	out = &ExecOutputStorageState{ModuleName: config.Name()}

	if workUpToBlockNum <= modInitBlock {
		return
	}
	for ptr := startBlock; ptr < workUpToBlockNum; {
		end := utils.MinOf(ptr-ptr%saveInterval+saveInterval, workUpToBlockNum)
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

package state

import (
	"fmt"

	"github.com/streamingfast/substreams/utils"

	"github.com/streamingfast/substreams/block"

	"github.com/streamingfast/substreams/storage/execout"
)

type ExecOutputStorageState struct {
	ModuleName string

	InitialCompleteRange *block.Range

	SegmentsPresent block.Ranges
	SegmentsMissing block.Ranges
}

func (m ExecOutputStorageState) Name() string                        { return m.ModuleName }
func (m ExecOutputStorageState) InitialProgressRanges() block.Ranges { return nil }
func (m ExecOutputStorageState) ReadyUpToBlock() uint64              { return 0 }

func (m ExecOutputStorageState) BatchRequests(subRequestSplitSize uint64) block.Ranges {
	return m.SegmentsMissing.MergedBuckets(subRequestSplitSize)
}

func NewExecOutputStorageState(config *execout.Config, saveInterval, workUpToBlockNum uint64, snapshots *Snapshots) (out *ExecOutputStorageState, err error) {
	// TODO: base the content of Mapper on the `snapshots` in here..
	modInitBlock := config.ModuleInitialBlock
	out = &ExecOutputStorageState{ModuleName: config.Name()}

	if workUpToBlockNum <= modInitBlock {
		return
	}

	completeSnapshot := snapshots.LastCompleteSnapshotBefore(workUpToBlockNum)
	if completeSnapshot != nil && completeSnapshot.ExclusiveEndBlock <= modInitBlock {
		return nil, fmt.Errorf("cannot have saved last store before module's init block")
	}

	backProcessStartBlock := modInitBlock
	if completeSnapshot != nil {
		backProcessStartBlock = completeSnapshot.ExclusiveEndBlock
		out.InitialCompleteRange = block.NewRange(modInitBlock, completeSnapshot.ExclusiveEndBlock)

		if completeSnapshot.ExclusiveEndBlock == workUpToBlockNum {
			return
		}
	}

	for ptr := backProcessStartBlock; ptr < workUpToBlockNum; {
		end := utils.MinOf(ptr-ptr%saveInterval+saveInterval, workUpToBlockNum)
		blockRange := block.NewRange(ptr, end)
		if !snapshots.Contains(blockRange) {
			out.SegmentsMissing = append(out.SegmentsMissing, blockRange)
		} else {
			out.SegmentsPresent = append(out.SegmentsPresent, blockRange)
		}
		ptr = end
	}

	return &ExecOutputStorageState{
		ModuleName: config.Name(),
	}, nil
}

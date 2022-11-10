package execoutput

import "github.com/streamingfast/substreams/block"

type ExecOutputStorageState struct {
	ModuleName string

	SegmentsPresent block.Ranges
	SegmentsMissing block.Ranges
}

func (m ExecOutputStorageState) Name() string                        { return m.ModuleName }
func (m ExecOutputStorageState) InitialProgressRanges() block.Ranges { return nil }
func (m ExecOutputStorageState) ReadyUpToBlock() uint64              { return 0 }

func (m ExecOutputStorageState) BatchRequests(subreqSplitSize uint64) block.Ranges {
	return m.SegmentsMissing.MergedBuckets(subreqSplitSize)
}

func NewExecOutputStorageState(modName string, modInitBlock, workUpToBlockNum uint64, snapshots string) (out *ExecOutputStorageState, err error) {
	// TODO: base the content of Mapper on the `snapshots` in here..
	return &ExecOutputStorageState{
		ModuleName: modName,
	}, nil
}

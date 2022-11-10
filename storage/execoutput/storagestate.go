package execoutput

import "github.com/streamingfast/substreams/block"

type MapperStorageState struct {
	ModuleName string

	SegmentsPresent block.Ranges
	SegmentsMissing block.Ranges
}

func (m MapperStorageState) Name() string                        { return m.ModuleName }
func (m MapperStorageState) InitialProgressRanges() block.Ranges { return nil }
func (m MapperStorageState) ReadyUpToBlock() uint64              { return 0 }

func (m MapperStorageState) BatchRequests(subreqSplitSize uint64) block.Ranges {
	return m.SegmentsMissing.MergedBuckets(subreqSplitSize)
}

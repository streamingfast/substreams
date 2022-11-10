package storage

import (
	"sort"

	"github.com/streamingfast/substreams/block"
)

type ModuleStorageState interface {
	Name() string
	InitialProgressRanges() block.Ranges
	ReadyUpToBlock() uint64
	BatchRequests(subrequestSplitSize uint64) block.Ranges
}

type ModuleStorageStateMap map[string]ModuleStorageState

func (m ModuleStorageStateMap) Names() (out []string) {
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return
}

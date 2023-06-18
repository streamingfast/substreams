package stage

import (
	"github.com/streamingfast/substreams/block"
	"github.com/streamingfast/substreams/storage/store"
)

// An individual module's progress towards synchronizing its `store`
type ModuleState struct {
	name string
	//state MergeState

	segmenter *block.Segmenter

	store *store.FullKV
}

func NewModuleState(name string, segmenter *block.Segmenter) *ModuleState {
	return &ModuleState{
		name:      name,
		segmenter: segmenter,
		//state:     MergeIdle,
	}
}

//type MergeState int
//
//const (
//	MergeIdle MergeState = iota
//	MergeMerging
//	MergeCompleted // All merging operations were completed for the provided Segmenter
//)

package stage

import (
	"github.com/abourget/llerrgroup"

	"github.com/streamingfast/substreams/block"
)

type Stage struct {
	idx  int
	kind Kind

	segmenter *block.Segmenter
	// The module's stores have been sync'd up to this segment, and are complete.
	segmentCompleted int

	storeModuleStates []*StoreModuleState

	// allExecutedModules is all the store+mapper executed specifically for this stage
	allExecutedModules []string

	// syncWork keeps tab of the parallel goroutines that do the merge work,
	// and need to be waited on before marking the Unit as properly merged.
	syncWork *llerrgroup.Group

	// asyncWork keeps tab of goroutines that were spun out and need to be
	// waited on only when the Scheduler is shutting down and everything
	// was completed.
	asyncWork *llerrgroup.Group
}

func NewStage(idx int, kind Kind, segmenter *block.Segmenter, moduleStates []*StoreModuleState, allExecutedModules []string) *Stage {
	return &Stage{
		idx:                idx,
		kind:               kind,
		allExecutedModules: allExecutedModules,
		segmenter:          segmenter,
		segmentCompleted:   segmenter.FirstIndex() - 1,
		storeModuleStates:  moduleStates,
		syncWork:           llerrgroup.New(250),
		asyncWork:          llerrgroup.New(250),
	}
}

func (s *Stage) nextUnit() Unit {
	return Unit{
		Stage:   s.idx,
		Segment: s.segmentCompleted + 1,
	}
}

type Kind int

const (
	KindMap = Kind(iota)
	KindStore
)

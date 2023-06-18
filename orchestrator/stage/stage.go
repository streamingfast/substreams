package stage

import (
	"github.com/abourget/llerrgroup"

	"github.com/streamingfast/substreams/block"
	"github.com/streamingfast/substreams/orchestrator/loop"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

type Stage struct {
	idx  int
	kind Kind

	segmenter *block.Segmenter
	// The module's stores have been sync'd up to this segment, and are complete.
	segmentCompleted int

	moduleStates []*ModuleState

	// writerErrGroup keeps tab of the goroutines that are writing the stores
	// and deleting the partials, so that we can properly wait for them to
	// shutdown before exiting the Scheduler.
	writerErrGroup *llerrgroup.Group
}

func NewStage(idx int, kind Kind, segmenter *block.Segmenter, moduleStates []*ModuleState) *Stage {
	return &Stage{
		idx:              idx,
		kind:             kind,
		segmenter:        segmenter,
		segmentCompleted: segmenter.FirstIndex() - 1,
		moduleStates:     moduleStates,
		writerErrGroup:   llerrgroup.New(250),
	}
}

func (s *Stage) DoMerge() loop.Cmd {
	mergeUnit := Unit{}
	return func() loop.Msg {
		if err := s.multiSquash(moduleStates); err != nil {
			return MsgMergeFailed{}
		}
		return MsgMerge{}
	}
}

func (s *Stage) MarkMergeFinished(u Unit) {

}

func stageKind(mods []*pbsubstreams.Module) Kind {
	if mods[0].GetKindStore() != nil {
		return KindStore
	}
	return KindMap
}

type Kind int

const (
	KindMap = iota
	KindStore
)

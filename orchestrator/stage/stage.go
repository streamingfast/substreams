package stage

import (
	"github.com/streamingfast/substreams/block"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

type Stage struct {
	kind Kind

	segmenter *block.Segmenter

	moduleStates []*ModuleState
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

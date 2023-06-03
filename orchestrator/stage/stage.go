package stage

import (
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

type Stage struct {
	kind Kind

	firstSegment int

	modules []*ModuleState
}

func stageKind(stage []*pbsubstreams.Module) Kind {
	if stage[0].GetKindStore() != nil {
		return KindStore
	}
	return KindMap
}

type Kind int

const (
	KindMap = iota
	KindStore
)

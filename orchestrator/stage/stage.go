package stage

import (
	"github.com/streamingfast/substreams/block"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

type Stage struct {
	*block.Segmenter

	kind Kind

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

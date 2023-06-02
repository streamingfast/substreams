package stage

import (
	"github.com/streamingfast/substreams/block"
)

type ModuleState struct {
	name string
	*block.Segmenter

	completedSegments int
	scheduledSegments map[int]bool

	readyUpToBlock     uint64
	mergedUpToBlock    uint64
	scheduledUpToBlock uint64
}

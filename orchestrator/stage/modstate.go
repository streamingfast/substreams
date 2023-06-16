package stage

import (
	"github.com/streamingfast/substreams/block"
	"github.com/streamingfast/substreams/storage/store"
)

type ModuleState struct {
	name string

	segmenter *block.Segmenter

	store *store.FullKV

	// The corresponding store has been sync'd up to this segment, and is complete
	segmentCompleted int
}

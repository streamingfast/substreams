package squasher

import (
	"github.com/streamingfast/substreams/block"
	"github.com/streamingfast/substreams/state"
)

type Squasher struct {
	builders map[string]*state.Builder
}

func (s *Squasher) Squash(moduleName string, blockRange *block.Range) error {
	return nil
}

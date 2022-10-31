package execout

import (
	"fmt"
	"github.com/streamingfast/bstream"
	"github.com/streamingfast/substreams/manifest"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

type NoOpCache struct {
}

func NewNoOpCache() CacheEngine {
	return &NoOpCache{}
}

func (n *NoOpCache) Init(modules *manifest.ModuleHashes) error {
	return nil
}

func (n *NoOpCache) NewBlock(blockRef bstream.BlockRef, step bstream.StepType) error {
	return nil
}

func (n *NoOpCache) EndOfStream(blockNum uint64) error {
	return nil
}

func (n *NoOpCache) HandleFinal(clock *pbsubstreams.Clock) error {
	return nil
}

func (n *NoOpCache) HandleUndo(clock *pbsubstreams.Clock, moduleName string) {
	return
}

func (n *NoOpCache) NewExecOutput(blockType string, block *bstream.Block, clock *pbsubstreams.Clock, cursor *bstream.Cursor) (ExecutionOutput, error) {
	execOutMap, err := NewExecOutputMap(blockType, block, clock)
	if err != nil {
		return nil, fmt.Errorf("setting up map: %w", err)
	}
	return execOutMap, nil
}

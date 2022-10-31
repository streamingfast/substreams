package execout

import (
	"fmt"
	"github.com/streamingfast/bstream"
	"github.com/streamingfast/substreams/manifest"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

type NoOpCache struct {
	blockType string
}

func NewNoOpCache(blockType string) CacheEngine {
	return &NoOpCache{blockType: blockType}
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

func (n *NoOpCache) NewExecOutput(block *bstream.Block, clock *pbsubstreams.Clock, cursor *bstream.Cursor) (ExecutionOutput, error) {
	execOutMap, err := NewExecOutputMap(n.blockType, block, clock)
	if err != nil {
		return nil, fmt.Errorf("setting up map: %w", err)
	}
	return execOutMap, nil
}

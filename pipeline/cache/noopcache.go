package cache

import (
	"fmt"

	execout2 "github.com/streamingfast/substreams/storage/execout"

	"github.com/streamingfast/bstream"
	"github.com/streamingfast/substreams/manifest"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

type NoOpCache struct {
	blockType string
}

func (n *NoOpCache) Init(_ *manifest.ModuleHashes) error {
	return nil
}

func (n *NoOpCache) NewBlock(_ bstream.BlockRef, _ bstream.StepType) error {
	return nil
}

func (n *NoOpCache) EndOfStream(_ bool, _ map[string]bool) error {
	return nil
}

func (n *NoOpCache) HandleFinal(_ *pbsubstreams.Clock) error {
	return nil
}

func (n *NoOpCache) HandleUndo(_ *pbsubstreams.Clock, _ string) {
	return
}

func (n *NoOpCache) NewExecOutput(block *bstream.Block, clock *pbsubstreams.Clock, cursor *bstream.Cursor) (execout2.ExecutionOutput, error) {
	execOutMap, err := execout2.NewExecOutputMap(n.blockType, block, clock)
	if err != nil {
		return nil, fmt.Errorf("setting up map: %w", err)
	}
	return execOutMap, nil
}

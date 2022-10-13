package execout

import (
	"fmt"
	"github.com/streamingfast/bstream"
	"github.com/streamingfast/substreams/manifest"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"google.golang.org/protobuf/proto"
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

func (n *NoOpCache) NewExecOutput(blockType string, block *bstream.Block, clock *pbsubstreams.Clock, cursor *bstream.Cursor) (ExecutionOutput, error) {
	execOutMap, err := NewExecOutputMap(blockType, block, clock)
	if err != nil {
		return nil, fmt.Errorf("setting up map: %w", err)
	}
	return execOutMap, nil
}

func SetupValues(blockType string, block *bstream.Block, clock *pbsubstreams.Clock) (map[string][]byte, error) {
	blkBytes, err := block.Payload.Get()
	if err != nil {
		return nil, fmt.Errorf("getting block %d %q: %w", block.Number, block.Id, err)
	}
	clockBytes, err := proto.Marshal(clock)
	if err != nil {
		return nil, fmt.Errorf("getting block %d %q: %w", block.Number, block.Id, err)
	}
	return map[string][]byte{
		blockType:                blkBytes,
		"sf.substreams.v1.Clock": clockBytes,
	}, nil
}

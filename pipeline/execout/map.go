package execout

import (
	"fmt"

	"github.com/streamingfast/bstream"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/wasm"
	"google.golang.org/protobuf/proto"
)

type ExecOutputMap struct {
	values map[string][]byte
	clock  *pbsubstreams.Clock
}

func NewExecOutputMap(blockType string, block *bstream.Block, clock *pbsubstreams.Clock) (*ExecOutputMap, error) {
	blkBytes, err := block.Payload.Get()
	if err != nil {
		return nil, fmt.Errorf("getting block %d %q: %w", block.Number, block.Id, err)
	}
	clockBytes, err := proto.Marshal(clock)
	if err != nil {
		return nil, fmt.Errorf("getting block %d %q: %w", block.Number, block.Id, err)
	}

	return &ExecOutputMap{
		clock: clock,
		values: map[string][]byte{
			blockType:      blkBytes,
			wasm.ClockType: clockBytes,
		},
	}, nil
}

func (i *ExecOutputMap) Clock() *pbsubstreams.Clock {
	return i.clock
}

func (i *ExecOutputMap) Get(moduleName string) (value []byte, cached bool, err error) {
	val, found := i.values[moduleName]
	if !found {
		return nil, false, NotFound
	}
	return val, false, nil
}

func (i *ExecOutputMap) Set(moduleName string, value []byte) (err error) {
	i.values[moduleName] = value
	return nil
}

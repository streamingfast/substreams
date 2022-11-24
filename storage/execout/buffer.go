package execout

import (
	"fmt"

	"github.com/streamingfast/bstream"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/wasm"
	"google.golang.org/protobuf/proto"
)

// ExecOutputBuffer holds the values produced by modules and exchanged between them
// as a sort of buffer.
type ExecOutputBuffer struct {
	values map[string][]byte
	clock  *pbsubstreams.Clock
} // TODO(abourget): rename to `Buffer`

func NewExecOutputBuffer(blockType string, block *bstream.Block, clock *pbsubstreams.Clock) (*ExecOutputBuffer, error) {
	blkBytes, err := block.Payload.Get()
	if err != nil {
		return nil, fmt.Errorf("getting block %d %q: %w", block.Number, block.Id, err)
	}
	clockBytes, err := proto.Marshal(clock)
	if err != nil {
		return nil, fmt.Errorf("marshalling clock %d %q: %w", clock.Number, clock.Id, err)
	}

	return &ExecOutputBuffer{
		clock: clock,
		values: map[string][]byte{
			blockType:      blkBytes,
			wasm.ClockType: clockBytes,
		},
	}, nil
}

func (i *ExecOutputBuffer) Clock() *pbsubstreams.Clock {
	return i.clock
}

func (i *ExecOutputBuffer) Get(moduleName string) (value []byte, cached bool, err error) {
	val, found := i.values[moduleName]
	if !found {
		return nil, false, NotFound
	}
	return val, false, nil
}

func (i *ExecOutputBuffer) Set(moduleName string, value []byte) (err error) {
	i.values[moduleName] = value
	return nil
}

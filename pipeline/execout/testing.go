package execout

import (
	"testing"

	"github.com/streamingfast/bstream"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

type ExecOutputTesting struct {
	Values map[string][]byte
	clock  *pbsubstreams.Clock
}

func NewExecOutputTesting(t *testing.T, block *bstream.Block, clock *pbsubstreams.Clock) *ExecOutputTesting {
	blkBytes, err := block.Payload.Get()
	require.NoError(t, err)

	clockBytes, err := proto.Marshal(clock)
	require.NoError(t, err)

	return &ExecOutputTesting{
		clock: clock,
		Values: map[string][]byte{
			"sf.substreams.v1.test.Block": blkBytes,
			"sf.substreams.v1.Clock":      clockBytes,
		},
	}
}

func (i *ExecOutputTesting) Get(moduleName string) (value []byte, cached bool, err error) {
	val, found := i.Values[moduleName]
	if !found {
		return nil, false, NotFound
	}
	return val, false, nil
}

func (i *ExecOutputTesting) Set(moduleName string, value []byte) (err error) {
	i.Values[moduleName] = value
	return nil
}

func (i *ExecOutputTesting) Clock() *pbsubstreams.Clock {
	return i.clock
}

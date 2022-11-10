package pipeline

import (
	"encoding/json"
	"testing"

	"github.com/streamingfast/substreams/storage/execout"

	"github.com/streamingfast/bstream"
	"github.com/streamingfast/logging"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zapcore"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

func init() {
	logging.InstantiateLoggers(logging.WithDefaultLevel(zapcore.DebugLevel))
}
func assertProtoEqual(t *testing.T, expected proto.Message, actual proto.Message) {
	t.Helper()

	if !proto.Equal(expected, actual) {
		expectedAsJSON, err := protojson.Marshal(expected)
		require.NoError(t, err)

		actualAsJSON, err := protojson.Marshal(actual)
		require.NoError(t, err)

		expectedAsMap := map[string]interface{}{}
		err = json.Unmarshal(expectedAsJSON, &expectedAsMap)
		require.NoError(t, err)

		actualAsMap := map[string]interface{}{}
		err = json.Unmarshal(actualAsJSON, &actualAsMap)
		require.NoError(t, err)

		// We use equal is not equal above so we get a good diff, if the first condition failed, the second will also always
		// fail which is what we want here
		assert.Equal(t, expectedAsMap, actualAsMap)
	}
}

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
		return nil, false, execout.NotFound
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

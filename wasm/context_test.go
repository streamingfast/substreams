package wasm

import (
	"testing"
	"time"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestCall_DoClock(t *testing.T) {
	clock := &pbsubstreams.Clock{
		Id:        "block-10",
		Number:    10,
		Timestamp: timestamppb.New(time.Unix(1000, 0).UTC()),
	}
	call := &Call{ModuleName: "mod", Clock: clock}

	out := &pbsubstreams.Clock{}
	require.NoError(t, proto.Unmarshal(call.DoClock(), out))
	assert.True(t, proto.Equal(clock, out), "expected %s, got %s", clock, out)
}

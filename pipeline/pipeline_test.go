package pipeline

import (
	"context"
	"encoding/hex"
	"github.com/streamingfast/bstream"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	pbsubstreamstest "github.com/streamingfast/substreams/pb/sf/substreams/v1/test"
	"github.com/streamingfast/substreams/pipeline/execout"
	"github.com/streamingfast/substreams/wasm"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
	"io/ioutil"
	"testing"
	"time"
)

func TestPipeline_runExecutor(t *testing.T) {
	tests := []struct {
		name       string
		moduleName string
		block      *pbsubstreamstest.Block
		request    *RequestContext
		executor   ModuleExecutor
		testFunc   func(t *testing.T, data []byte)
	}{
		{
			name:       "golden path",
			moduleName: "map_test",
			block:      &pbsubstreamstest.Block{Id: "block-10", Number: 10, Step: int32(bstream.StepNewIrreversible)},
			executor:   mapTestExecutor(t),
			testFunc: func(t *testing.T, data []byte) {
				out := &pbsubstreamstest.MapResult{}
				err := proto.Unmarshal(data, out)
				require.NoError(t, err)
				assertProtoEqual(t, &pbsubstreamstest.MapResult{
					BlockNumber: 10,
					BlockHash:   "block-10",
				}, out)
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			pipe := &Pipeline{
				reqCtx: testRequestContext(context.Background()),
			}
			clock := &pbsubstreams.Clock{Id: test.block.Id, Number: test.block.Number}
			execOutput := execout.NewExecOutputTesting(t, bstreamBlk(t, test.block), clock)
			err := pipe.runExecutor(test.executor, execOutput)
			require.NoError(t, err)
			output, found := execOutput.Values[test.moduleName]
			require.Equal(t, true, found)
			test.testFunc(t, output)
		})
	}
}

func testRequestContext(ctx context.Context) *RequestContext {
	return &RequestContext{
		Context: ctx,
		logger:  zap.NewNop(),
	}
}

func mapTestExecutor(t *testing.T) *MapperModuleExecutor {
	cnt, err := ioutil.ReadFile("./testdata/map_test.code.hex")
	require.NoError(t, err)

	code, err := hex.DecodeString(string(cnt))
	require.NoError(t, err)

	wasmModule, err := wasm.NewRuntime(nil).NewModule(
		context.Background(),
		nil,
		code,
		"map_test",
		"map_test",
	)
	require.NoError(t, err)

	return &MapperModuleExecutor{
		BaseExecutor: BaseExecutor{
			moduleName: "map_test",
			wasmModule: wasmModule,
			wasmArguments: []wasm.Argument{
				wasm.NewBlockInput("sf.substreams.v1.test.Block"),
			},
			entrypoint: "map_test",
			tracer:     otel.GetTracerProvider().Tracer("test"),
		},
		outputType: "",
	}
}

type Obj struct {
	cursor *bstream.Cursor
	step   bstream.StepType
}

func (o *Obj) Cursor() *bstream.Cursor {
	return o.cursor
}

func (o *Obj) Step() bstream.StepType {
	return o.step
}

func bstreamBlk(t *testing.T, blk *pbsubstreamstest.Block) *bstream.Block {
	payload, err := proto.Marshal(blk)
	require.NoError(t, err)

	bb := &bstream.Block{
		Id:             blk.Id,
		Number:         blk.Number,
		PreviousId:     "",
		Timestamp:      time.Time{},
		LibNum:         0,
		PayloadKind:    0,
		PayloadVersion: 0,
	}
	_, err = bstream.MemoryBlockPayloadSetter(bb, payload)
	require.NoError(t, err)

	return bb
}

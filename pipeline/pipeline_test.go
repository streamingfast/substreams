package pipeline

import (
	"context"
	"testing"
	"time"

	"github.com/streamingfast/bstream"
	"github.com/streamingfast/substreams/manifest"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	pbsubstreamstest "github.com/streamingfast/substreams/pb/sf/substreams/v1/test"
	"github.com/streamingfast/substreams/wasm"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"google.golang.org/protobuf/proto"
)

func TestPipeline_runExecutor(t *testing.T) {
	tests := []struct {
		name       string
		moduleName string
		execOutput *MapperModuleExecutor
		block      *pbsubstreamstest.Block
		testFunc   func(t *testing.T, data []byte)
	}{
		{
			name:       "executes map",
			moduleName: "test_map",
			block:      &pbsubstreamstest.Block{Id: "block-10", Number: 10, Step: int32(bstream.StepNewIrreversible)},
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
			ctx := context.Background()
			pipe := &Pipeline{}
			clock := &pbsubstreams.Clock{Id: test.block.Id, Number: test.block.Number}
			execOutput := NewExecOutputTesting(t, bstreamBlk(t, test.block), clock)
			executor := mapTestExecutor(t, test.moduleName)
			err := pipe.runExecutor(ctx, executor, execOutput)
			require.NoError(t, err)
			output, found := execOutput.Values[test.moduleName]
			require.Equal(t, true, found)
			test.testFunc(t, output)
		})
	}
}

func mapTestExecutor(t *testing.T, name string) *MapperModuleExecutor {
	pkg, _ := processManifest(t, "../test/testdata/substreams-test-v0.1.0.spkg")

	binayrIndex := uint32(0)
	for _, module := range pkg.Modules.Modules {
		if module.Name == name {
			binayrIndex = module.BinaryIndex
		}
	}
	binary := pkg.Modules.Binaries[binayrIndex]
	require.Greater(t, len(binary.Content), 1)

	wasmModule, err := wasm.NewRuntime(nil).NewModule(
		context.Background(),
		nil,
		binary.Content,
		name,
		name,
	)
	require.NoError(t, err)

	return &MapperModuleExecutor{
		BaseExecutor: BaseExecutor{
			moduleName: name,
			wasmModule: wasmModule,
			wasmArguments: []wasm.Argument{
				wasm.NewSourceInput("sf.substreams.v1.test.Block"),
			},
			entrypoint: name,
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

func processManifest(t *testing.T, manifestPath string) (*pbsubstreams.Package, *manifest.ModuleGraph) {
	t.Helper()

	manifestReader := manifest.NewReader(manifestPath)
	pkg, err := manifestReader.Read()
	require.NoError(t, err)

	moduleGraph, err := manifest.NewModuleGraph(pkg.Modules.Modules)
	require.NoError(t, err)

	return pkg, moduleGraph
}

func TestSetupSubrequestStores(t *testing.T) {
	t.Skip("these need to be written")
	// TODO(abourget):
	// We need to test: setupSubrequestStores

	// with stores: [A(init=10), B(init=10), C(init=20)], startBlock=20, outputModules=['B']
	// assert: storeMap[A] is FullKV, storeMap[B] is PartialKV

	// with stores: [A(init=10), B(init=10), C(init=20)], startBlock=20, outputModules=['D']
	// assert: storeMap[A] is FullKV, storeMap[B] is PartialKV

	// This will need to work for both a mapper and a store.
	// If we ask for a store to be processed, we expect it to be a PartialKV
	// Otherwise, if the output module we want is a mapper, everything needs to be FullKV
}

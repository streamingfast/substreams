package pipeline

import (
	"context"
	"testing"
	"time"

	"github.com/streamingfast/substreams/pipeline/outputmodules"

	store2 "github.com/streamingfast/substreams/storage/store"
	"go.uber.org/zap"

	"github.com/streamingfast/bstream"
	"github.com/streamingfast/dstore"
	"github.com/streamingfast/substreams/manifest"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	pbsubstreamstest "github.com/streamingfast/substreams/pb/sf/substreams/v1/test"
	"github.com/streamingfast/substreams/pipeline/exec"
	"github.com/streamingfast/substreams/reqctx"
	"github.com/streamingfast/substreams/wasm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"google.golang.org/protobuf/proto"
)

func TestPipeline_runExecutor(t *testing.T) {
	tests := []struct {
		name       string
		moduleName string
		execOutput *exec.MapperModuleExecutor
		block      *pbsubstreamstest.Block
		testFunc   func(t *testing.T, data []byte)
	}{
		{
			name:       "executes map",
			moduleName: "test_map",
			block:      &pbsubstreamstest.Block{Id: "block-10", Number: 10},
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
			ctx := reqctx.WithRequest(context.Background(), &reqctx.RequestDetails{})
			pipe := &Pipeline{
				forkHandler: NewForkHandler(),
				outputGraph: outputmodules.TestNew(),
			}
			clock := &pbsubstreams.Clock{Id: test.block.Id, Number: test.block.Number}
			execOutput := NewExecOutputTesting(t, bstreamBlk(t, test.block), clock)
			executor := mapTestExecutor(t, test.moduleName)
			err := pipe.execute(ctx, executor, execOutput)
			require.NoError(t, err)
			output, found := execOutput.Values[test.moduleName]
			require.Equal(t, true, found)
			test.testFunc(t, output)
		})
	}
}

func mapTestExecutor(t *testing.T, name string) *exec.MapperModuleExecutor {
	pkg := manifest.TestReadManifest(t, "../test/testdata/substreams-test-v0.1.0.spkg")

	binaryIndex := uint32(0)
	for _, module := range pkg.Modules.Modules {
		if module.Name == name {
			binaryIndex = module.BinaryIndex
		}
	}
	binary := pkg.Modules.Binaries[binaryIndex]
	require.Greater(t, len(binary.Content), 1)

	wasmModule, err := wasm.NewRuntime(nil, 0).NewModule(
		context.Background(),
		nil,
		binary.Content,
		name,
		name,
	)
	require.NoError(t, err)

	return exec.NewMapperModuleExecutor(
		exec.NewBaseExecutor(
			name,
			wasmModule,
			[]wasm.Argument{
				wasm.NewParamsInput("my test params"),
				wasm.NewSourceInput("sf.substreams.v1.test.Block"),
			},
			name,
			otel.GetTracerProvider().Tracer("test"),
		),
		"",
	)
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

func TestSetupSubrequestStores(t *testing.T) {
	t.Run("test store types depending on input", func(t *testing.T) {

		confMap := testConfigMap(t, []testStoreConfig{
			{name: "mod1", initBlock: 10, writtenUpTo: 0},
			{name: "mod2", initBlock: 1, writtenUpTo: 10},
			{name: "mod3", initBlock: 5, writtenUpTo: 0},
		})
		p := Pipeline{stores: &Stores{configs: confMap}}
		ctx := withTestRequest(t, "mod3", 10)

		storeMap, err := p.setupSubrequestStores(ctx)

		require.NoError(t, err)
		assert.Len(t, storeMap, 3)

		fullKV := storeMap["mod1"].(*store2.FullKV)
		assert.Equal(t, 10, int(fullKV.ModuleInitialBlock()))
		val, _ := storeMap["mod2"].(*store2.FullKV).GetLast("k")
		assert.Equal(t, []byte("v"), val)
		partialKV := storeMap["mod3"].(*store2.PartialKV)
		assert.Equal(t, 10, int(partialKV.InitialBlock()))
	})

	//t.Run("fail with multiple output modules", func(t *testing.T) {
	//	ctx := withTestRequest(t, "mod1", 10)
	//	p := Pipeline{stores: &Stores{configs: nil}}
	//
	//	_, err := p.setupSubrequestStores(ctx)
	//
	//	assert.Equal(t, "invalid number of backprocess leaf store: 2", err.Error())
	//})
}

func testConfigMap(t *testing.T, configs []testStoreConfig) store2.ConfigMap {
	t.Helper()
	confMap := make(store2.ConfigMap)
	objStore := dstore.NewMockStore(nil)

	for _, conf := range configs {
		newStore, err := store2.NewConfig(conf.name, conf.initBlock, conf.name, pbsubstreams.Module_KindStore_UPDATE_POLICY_SET, "string", objStore)
		require.NoError(t, err)
		confMap[newStore.Name()] = newStore

		if conf.writtenUpTo != 0 {
			fullKV := newStore.NewFullKV(zap.NewNop())
			fullKV.Set(0, "k", "v")
			_, writer, err := fullKV.Save(conf.writtenUpTo)
			require.NoError(t, err)
			require.NoError(t, writer.Write(context.Background()))
		}
	}
	return confMap
}

type testStoreConfig struct {
	name        string
	initBlock   uint64
	writtenUpTo uint64
}

func withTestRequest(t *testing.T, outputModule string, startBlock uint64) context.Context {
	t.Helper()
	req, err := BuildRequestDetails(&pbsubstreams.Request{
		OutputModule:  outputModule,
		StartBlockNum: int64(startBlock),
	}, false, func(name string) bool {
		return name == outputModule
	}, func() (uint64, error) { return 0, nil })
	require.NoError(t, err)
	return reqctx.WithRequest(context.Background(), req)
}

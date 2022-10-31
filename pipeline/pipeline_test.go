package pipeline

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/streamingfast/bstream"
	"github.com/streamingfast/dstore"
	"github.com/streamingfast/substreams/manifest"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	pbsubstreamstest "github.com/streamingfast/substreams/pb/sf/substreams/v1/test"
	"github.com/streamingfast/substreams/pipeline/exec"
	"github.com/streamingfast/substreams/reqctx"
	"github.com/streamingfast/substreams/store"
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
			ctx := context.Background()
			pipe := &Pipeline{
				forkHandler: NewForkHandler(),
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

	return exec.NewMapperModuleExecutor(
		exec.NewBaseExecutor(
			name,
			wasmModule,
			[]wasm.Argument{
				wasm.NewSourceInput("sf.substreams.v1.test.Block"),
			},
			name,
			otel.GetTracerProvider().Tracer("test"),
		),
		"",
	)
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
	t.Skip("file format change")
	p := Pipeline{}

	t.Run("test store types depending on input", func(t *testing.T) {
		confMap := testConfigMap(t, []string{
			"mod2/states/0000000010-0000000001.kv",
		}, []testStoreConfig{
			{"mod1", 10},
			{"mod2", 1},
			{"mod3", 5},
		})
		ctx := withTestRequest("mod3", 10)

		storeMap, err := p.setupSubrequestStores(ctx, confMap)

		require.NoError(t, err)
		assert.Len(t, storeMap, 3)

		fullKV := storeMap["mod1"].(*store.FullKV)
		assert.Equal(t, 10, int(fullKV.ModuleInitialBlock()))
		val, _ := storeMap["mod2"].(*store.FullKV).GetLast("k")
		assert.Equal(t, []byte("v"), val)
		partialKV := storeMap["mod3"].(*store.PartialKV)
		assert.Equal(t, 10, int(partialKV.InitialBlock()))
	})

	t.Run("fail with multiple output modules", func(t *testing.T) {
		ctx := withTestRequest("mod1,mod2", 10)

		_, err := p.setupSubrequestStores(ctx, nil)

		assert.Equal(t, "invalid number of backprocess leaf store: 2", err.Error())
	})
}

func testConfigMap(t *testing.T, files []string, configs []testStoreConfig) store.ConfigMap {
	t.Helper()
	confMap := make(store.ConfigMap)
	objStore := dstore.NewMockStore(nil)
	bytes := []byte{1, 1, 107, 1, 118} // {"k": "v"}
	for _, file := range files {
		objStore.SetFile(file, bytes)
	}
	for _, conf := range configs {
		newStore, err := store.NewConfig(conf.name, conf.initBlock, conf.name, pbsubstreams.Module_KindStore_UPDATE_POLICY_SET, "string", objStore)
		require.NoError(t, err)
		confMap[newStore.Name()] = newStore
	}
	return confMap
}

type testStoreConfig struct {
	name      string
	initBlock uint64
}

func withTestRequest(outputModule string, startBlock uint64) context.Context {
	ctx, _ := reqctx.WithRequest(context.Background(), &pbsubstreams.Request{
		OutputModules: strings.Split(outputModule, ","),
		StartBlockNum: int64(startBlock),
	}, false)
	return ctx
}

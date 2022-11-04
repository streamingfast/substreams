package integration

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"testing"

	"github.com/streamingfast/substreams/wasm"

	"github.com/streamingfast/bstream"
	"github.com/streamingfast/dstore"
	"github.com/streamingfast/substreams/orchestrator/work"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	pbsubstreamstest "github.com/streamingfast/substreams/pb/sf/substreams/v1/test"
	"github.com/streamingfast/substreams/pipeline"
	"github.com/streamingfast/substreams/pipeline/execout/cachev1"
	"github.com/streamingfast/substreams/reqctx"
	"github.com/streamingfast/substreams/service/config"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"google.golang.org/grpc/metadata"
)

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

type TestStoreDelta struct {
	Operation string      `json:"op"`
	OldValue  interface{} `json:"old"`
	NewValue  interface{} `json:"new"`
}

type TestStoreOutput struct {
	StoreName string            `json:"name"`
	Deltas    []*TestStoreDelta `json:"deltas"`
}
type TestMapOutput struct {
	ModuleName string                      `json:"name"`
	Result     *pbsubstreamstest.MapResult `json:"result"`
}
type AssertMapOutput struct {
	ModuleName string `json:"name"`
	Result     bool   `json:"result"`
}

func processRequest(
	t *testing.T,
	ctx context.Context,
	request *pbsubstreams.Request,
	workerFactory work.WorkerFactory,
	newGenerator NewTestBlockGenerator,
	responseCollector *responseCollector,
	isSubRequest bool,
	blockProcessedCallBack blockProcessedCallBack,
	testTempDir string,
	subrequestsSplitSize uint64,
	parallelSubrequests uint64,
) error {
	t.Helper()

	var opts []pipeline.Option

	reqDetails, err := pipeline.BuildRequestDetails(request, isSubRequest, func() (uint64, error) {
		return request.StopBlockNum, nil
		//return 0, fmt.Errorf("no live feed")
	})
	require.NoError(t, err)
	ctx = reqctx.WithRequest(ctx, reqDetails)

	baseStoreStore, err := dstore.NewStore(filepath.Join(testTempDir, "test.store"), "", "none", true)
	require.NoError(t, err)

	runtimeConfig := config.NewRuntimeConfig(
		10,
		10,
		subrequestsSplitSize,
		parallelSubrequests,
		baseStoreStore,
		workerFactory,
	)

	cachingEngine, err := cachev1.NewEngine(runtimeConfig, "sf.substreams.v1.test.Block", zap.NewNop())
	require.NoError(t, err)

	moduleTree, err := pipeline.NewModuleTree(request, "sf.substreams.v1.test.Block")
	require.NoError(t, err)

	storeConfigs, err := pipeline.InitializeStoreConfigs(moduleTree, runtimeConfig.BaseObjectStore)
	require.NoError(t, err)

	stores := pipeline.NewStores(storeConfigs, runtimeConfig.StoreSnapshotsSaveInterval, reqDetails.RequestStartBlockNum, request.StopBlockNum, isSubRequest)

	pipe := pipeline.New(
		ctx,
		moduleTree,
		stores,
		wasm.NewRuntime(nil),
		cachingEngine,
		runtimeConfig,
		responseCollector.Collect,
		opts...,
	)

	require.NoError(t, pipe.Init(ctx))

	tr := &TestRunner{
		t:                      t,
		baseStoreStore:         baseStoreStore,
		blockProcessedCallBack: blockProcessedCallBack,
		request:                request,
		blockGeneratorFactory:  newGenerator,
		pipe:                   pipe,
	}

	err = pipe.Launch(ctx, tr, &nooptrailable{})

	if closer, ok := cachingEngine.(io.Closer); ok {
		closer.Close()
	}

	return err
}

func funcName(t *testing.T, err error) {
	require.Nil(t, err)
}

type nooptrailable struct {
}

func (n nooptrailable) SetTrailer(md metadata.MD) {
}

type TestRunner struct {
	t *testing.T

	baseStoreStore         dstore.Store
	blockProcessedCallBack blockProcessedCallBack
	request                *pbsubstreams.Request
	blockGeneratorFactory  NewTestBlockGenerator
	pipe                   *pipeline.Pipeline
}

func (r *TestRunner) Run(context.Context) error {
	generator := r.blockGeneratorFactory(uint64(r.request.StartBlockNum), r.request.StopBlockNum)

	for _, generatedBlock := range generator.Generate() {
		blk := generatedBlock.block
		err := r.pipe.ProcessBlock(blk, generatedBlock.obj)

		if err != nil && !errors.Is(err, io.EOF) {
			return fmt.Errorf("process block: %w", err)
		}
		if errors.Is(err, io.EOF) {
			return err
		}

		if r.blockProcessedCallBack != nil {
			r.blockProcessedCallBack(r.pipe, blk, r.pipe.GetStoreMap(), r.baseStoreStore)
		}
	}
	return nil
}

package integration

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"testing"
	"time"

	"github.com/streamingfast/bstream"
	"github.com/streamingfast/dstore"
	"github.com/streamingfast/substreams/manifest"
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
	"google.golang.org/protobuf/proto"
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
	moduleGraph *manifest.ModuleGraph,
	workerFactory work.WorkerFactory,
	newGenerator NewTestBlockGenerator,
	responseCollector *responseCollector, isSubRequest bool,
	blockProcessedCallBack blockProcessedCallBack,
	testTempDir string,
	subrequestsSplitSize uint64,
) error {
	t.Helper()

	var opts []pipeline.Option

	ctx, err := reqctx.WithRequest(ctx, request, isSubRequest)
	require.Nil(t, err)

	baseStoreStore, err := dstore.NewStore(filepath.Join(testTempDir, "test.store"), "", "none", true)
	require.NoError(t, err)

	cachingEngine, err := cachev1.NewEngine(config.RuntimeConfig{StoreSnapshotsSaveInterval: 10, BaseObjectStore: baseStoreStore}, zap.NewNop())
	require.NoError(t, err)
	storeBoundary := pipeline.NewStoreBoundary(10, request.StopBlockNum)

	runtimeConfig := config.NewRuntimeConfig(
		10,
		10,
		subrequestsSplitSize,
		1,
		baseStoreStore,
		workerFactory,
	)
	pipe := pipeline.New(
		ctx,
		moduleGraph,
		"sf.substreams.v1.test.Block",
		nil,
		cachingEngine,
		runtimeConfig,
		storeBoundary,
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

	for _, b := range generator.Generate() {
		o := &Obj{
			cursor: bstream.EmptyCursor,
			step:   bstream.StepType(b.Step),
		}

		payload, err := proto.Marshal(b)
		require.NoError(r.t, err)

		bb := &bstream.Block{
			Id:             b.Id,
			Number:         b.Number,
			PreviousId:     "",
			Timestamp:      time.Time{},
			LibNum:         0,
			PayloadKind:    0,
			PayloadVersion: 0,
		}
		_, err = bstream.MemoryBlockPayloadSetter(bb, payload)
		require.NoError(r.t, err)

		err = r.pipe.ProcessBlock(bb, o)

		if err != nil && !errors.Is(err, io.EOF) {
			return fmt.Errorf("process block: %w", err)
		}
		if errors.Is(err, io.EOF) {
			return err
		}

		if r.blockProcessedCallBack != nil {
			r.blockProcessedCallBack(r.pipe, bb, r.pipe.StoreMap, r.baseStoreStore)
		}
	}
	return nil
}

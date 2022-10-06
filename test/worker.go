package pipeline

import (
	"context"
	"github.com/streamingfast/bstream"
	"github.com/streamingfast/dstore"
	"github.com/streamingfast/substreams"
	"github.com/streamingfast/substreams/block"
	"github.com/streamingfast/substreams/manifest"
	"github.com/streamingfast/substreams/orchestrator"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/pipeline"
	"github.com/streamingfast/substreams/pipeline/execout/cachev1"
	"github.com/streamingfast/substreams/store"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
	"io"
	"testing"
	"time"
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

type responseCollector struct {
	responses []*pbsubstreams.Response
}

func NewResponseCollector() *responseCollector {
	return &responseCollector{
		responses: []*pbsubstreams.Response{},
	}
}

func (c *responseCollector) Collect(resp *pbsubstreams.Response) error {
	c.responses = append(c.responses, resp)
	return nil
}

type TestWorker struct {
	t                 *testing.T
	moduleGraph       *manifest.ModuleGraph
	responseCollector *responseCollector
}

func (w *TestWorker) Run(ctx context.Context, job *orchestrator.Job, requestModules *pbsubstreams.Modules, respFunc substreams.ResponseFunc) ([]*block.Range, error) {
	w.t.Helper()
	req := job.CreateRequest(requestModules)
	blockGenerator := LinearBlockGenerator{
		startBlock:         uint64(req.StartBlockNum),
		inclusiveStopBlock: req.StopBlockNum,
	}

	_ = processRequest(w.t, req, w.moduleGraph, blockGenerator, nil, w.responseCollector, true)
	return block.Ranges{
		&block.Range{
			StartBlock:        uint64(req.StartBlockNum),
			ExclusiveEndBlock: req.StopBlockNum,
		},
	}, nil
}

func processRequest(t *testing.T, request *pbsubstreams.Request, moduleGraph *manifest.ModuleGraph, generator TestBlockGenerator, workerPool *orchestrator.WorkerPool, responseCollector *responseCollector, isSubRequest bool) (out []*pbsubstreams.Response) {
	t.Helper()

	ctx := context.Background()

	var opts []pipeline.Option

	req := pipeline.NewRequestContext(ctx, request, isSubRequest)

	baseStoreStore, err := dstore.NewStore("file:///tmp/test.store", "", "none", true)
	require.NoError(t, err)

	cachingEngine, err := cachev1.NewEngine(ctx, 10, baseStoreStore, zap.NewNop())
	require.NoError(t, err)

	storeGenerator := pipeline.NewStoreFactory(baseStoreStore, 10)
	storeBoundary := pipeline.NewStoreBoundary(10)
	storeMap := store.NewMap()

	pipe := pipeline.New(
		req,
		moduleGraph,
		"sf.substreams.v1.test.Block",
		nil,
		10,
		cachingEngine,
		storeMap,
		storeGenerator,
		storeBoundary,
		responseCollector.Collect,
		opts...,
	)

	err = pipe.Init(workerPool)
	require.NoError(t, err)

	for _, b := range generator.Generate() {
		o := &Obj{
			cursor: bstream.EmptyCursor,
			step:   bstream.StepType(b.Step),
		}

		payload, err := proto.Marshal(b)
		require.NoError(t, err)

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
		require.NoError(t, err)
		err = pipe.ProcessBlock(bb, o)
		if err != nil {
			require.Equal(t, io.EOF, err)
		}
	}

	return
}

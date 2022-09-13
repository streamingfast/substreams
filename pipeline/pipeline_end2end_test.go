package pipeline

import (
	"context"
	"path"
	"runtime"
	"strconv"
	"testing"
	"time"

	"github.com/streamingfast/bstream"
	"github.com/streamingfast/dstore"
	"github.com/streamingfast/substreams"
	"github.com/streamingfast/substreams/block"
	"github.com/streamingfast/substreams/manifest"
	"github.com/streamingfast/substreams/orchestrator"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"google.golang.org/protobuf/proto"
)

type TestBlockGenerator interface {
	Generate() []*pbsubstreams.TestBlock
}

type LinearBlockGenerator struct {
	startBlock         uint64
	inclusiveStopBlock uint64
}

func (g LinearBlockGenerator) Generate() []*pbsubstreams.TestBlock {
	var blocks []*pbsubstreams.TestBlock
	for i := g.startBlock; i <= g.inclusiveStopBlock; i++ {
		blocks = append(blocks, &pbsubstreams.TestBlock{
			Id:     "block-" + strconv.FormatUint(i, 10),
			Number: i,
			Step:   int32(pbsubstreams.ForkStep_STEP_NEW),
		})
	}
	return blocks
}

type TestWorker struct {
	t           *testing.T
	moduleGraph *manifest.ModuleGraph
}

func (w *TestWorker) Run(ctx context.Context, job *orchestrator.Job, requestModules *pbsubstreams.Modules, respFunc substreams.ResponseFunc) ([]*block.Range, error) {
	w.t.Helper()
	req := job.CreateRequest(requestModules)
	blockGenerator := LinearBlockGenerator{
		startBlock:         uint64(req.StartBlockNum),
		inclusiveStopBlock: req.StopBlockNum + 1,
	}

	_ := processRequest(w.t, req, w.moduleGraph, blockGenerator, nil)
	//todo: cumulate responses

	return block.Ranges{
		&block.Range{
			StartBlock:        uint64(req.StartBlockNum),
			ExclusiveEndBlock: req.StopBlockNum,
		},
	}, nil
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

func processRequest(t *testing.T, request *pbsubstreams.Request, moduleGraph *manifest.ModuleGraph, generator TestBlockGenerator, workerPool *orchestrator.WorkerPool) (out []*pbsubstreams.Response) {
	t.Helper()

	ctx := context.Background()
	tracer := otel.GetTracerProvider().Tracer("end2end-test")
	baseStoreStore, err := dstore.NewStore("file:///tmp/", "", "none", true)
	require.NoError(t, err)

	pipe := New(
		ctx,
		tracer,
		request,
		moduleGraph,
		"sf.substreams.v1.TestBlock",
		baseStoreStore,
		10,
		nil,
		10,
		func(resp *pbsubstreams.Response) error {
			out = append(out, resp)
			return nil
		},
	)

	if err := pipe.Init(workerPool); err != nil {
		require.NoError(t, err)
	}

	for _, b := range generator.Generate() {
		o := &Obj{
			cursor: nil,
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
		require.NoError(t, err)
	}

	return
}

func SimplepipelineTest(t *testing.T) {
	_, filename, _, _ := runtime.Caller(1)
	manifestDir := path.Dir(filename)

	manifestReader := manifest.NewReader(path.Join(manifestDir, "manifest.yaml"))
	pkg, err := manifestReader.Read()
	require.NoError(t, err)

	_, err = manifest.NewModuleGraph(pkg.Modules.Modules)
	require.NoError(t, err)

	//request := &pbsubstreams.Request{
	//	StartBlockNum:                  10,
	//	StartCursor:                    "",
	//	StopBlockNum:                   11,
	//	ForkSteps:                      nil,
	//	IrreversibilityCondition:       "",
	//	Modules:                        nil,
	//	OutputModules:                  nil,
	//	InitialStoreSnapshotForModules: nil,
	//}
	//
	//workerPool := orchestrator.NewWorkerPool(1, func(tracer ttrace.Tracer) orchestrator.Worker {
	//	return &TestWorker{
	//		t:           t,
	//		moduleGraph: moduleGraph,
	//	}
	//})
	//
	//blockGenerator := LinearBlockGenerator{
	//	startBlock:         uint64(request.StartBlockNum),
	//	inclusiveStopBlock: request.StopBlockNum + 1,
	//}
	//processRequest(t, request, moduleGraph, blockGenerator, workerPool)
}

package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/streamingfast/bstream"
	"github.com/streamingfast/dstore"
	"github.com/streamingfast/logging"
	"github.com/streamingfast/substreams"
	"github.com/streamingfast/substreams/block"
	"github.com/streamingfast/substreams/manifest"
	"github.com/streamingfast/substreams/orchestrator"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	pbsubstreamstest "github.com/streamingfast/substreams/pb/sf/substreams/v1/test"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	ttrace "go.opentelemetry.io/otel/trace"
	"google.golang.org/protobuf/proto"
)

type TestBlockGenerator interface {
	Generate() []*pbsubstreamstest.Block
}

type LinearBlockGenerator struct {
	startBlock         uint64
	inclusiveStopBlock uint64
}

func (g LinearBlockGenerator) Generate() []*pbsubstreamstest.Block {
	var blocks []*pbsubstreamstest.Block
	for i := g.startBlock; i <= g.inclusiveStopBlock; i++ {
		blocks = append(blocks, &pbsubstreamstest.Block{
			Id:     "block-" + strconv.FormatUint(i, 10),
			Number: i,
			Step:   int32(bstream.StepIrreversible),
		})
	}
	return blocks
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

func processRequest(t *testing.T, request *pbsubstreams.Request, moduleGraph *manifest.ModuleGraph, generator TestBlockGenerator, workerPool *orchestrator.WorkerPool, responseCollector *responseCollector, isSubRequest bool) (out []*pbsubstreams.Response) {
	t.Helper()

	ctx := context.Background()
	tracer := otel.GetTracerProvider().Tracer("end2end-test")
	baseStoreStore, err := dstore.NewStore("file:///tmp/test.store", "", "none", true)
	require.NoError(t, err)

	pipe := New(
		ctx,
		tracer,
		request,
		moduleGraph,
		"sf.substreams.v1.test.Block",
		baseStoreStore,
		10,
		10,
		nil,
		10,
		responseCollector.Collect,
	)

	pipe.isSubrequest = isSubRequest

	if err := pipe.Init(workerPool); err != nil {
		require.NoError(t, err)
	}

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

func processManifest(t *testing.T, manifestPath string) (*pbsubstreams.Package, *manifest.ModuleGraph) {
	t.Helper()

	manifestReader := manifest.NewReader(manifestPath)
	pkg, err := manifestReader.Read()
	require.NoError(t, err)

	moduleGraph, err := manifest.NewModuleGraph(pkg.Modules.Modules)
	require.NoError(t, err)

	return pkg, moduleGraph
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

func runTest(t *testing.T, startBlock int64, exclusiveEndBlock uint64, moduleNames []string) (moduleOutputs []string) {
	_, _ = logging.ApplicationLogger("test", "test")

	err := os.RemoveAll("/tmp/test.store")
	require.NoError(t, err)

	//todo: compile substreams

	pkg, moduleGraph := processManifest(t, "./test_data/simple_substreams/substreams.yaml")

	request := &pbsubstreams.Request{
		StartBlockNum: startBlock,
		StopBlockNum:  exclusiveEndBlock,
		Modules:       pkg.Modules,
		OutputModules: moduleNames,
	}

	responseCollector := NewResponseCollector()
	workerPool := orchestrator.NewWorkerPool(1, func(tracer ttrace.Tracer) orchestrator.Worker {
		return &TestWorker{
			t:                 t,
			moduleGraph:       moduleGraph,
			responseCollector: NewResponseCollector(),
		}
	})

	blockGenerator := LinearBlockGenerator{
		startBlock:         uint64(request.StartBlockNum),
		inclusiveStopBlock: request.StopBlockNum,
	}

	processRequest(t, request, moduleGraph, blockGenerator, workerPool, responseCollector, false)

	for _, response := range responseCollector.responses {
		switch r := response.Message.(type) {
		case *pbsubstreams.Response_Progress:
			_ = r.Progress
		case *pbsubstreams.Response_SnapshotData:
			_ = r.SnapshotData
		case *pbsubstreams.Response_SnapshotComplete:
			_ = r.SnapshotComplete
		case *pbsubstreams.Response_Data:
			for _, output := range r.Data.Outputs {
				for _, log := range output.Logs {
					fmt.Println("LOG: ", log)
				}
				if out := output.GetMapOutput(); out != nil {
					if output.Name == "map_test" {
						r := &pbsubstreamstest.MapResult{}
						err = proto.Unmarshal(out.Value, r)
						require.NoError(t, err)

						out := &TestMapOutput{
							ModuleName: output.Name,
							Result:     r,
						}
						jsonData, err := json.Marshal(out)
						require.NoError(t, err)
						moduleOutputs = append(moduleOutputs, string(jsonData))
					}
				}
				if out := output.GetStoreDeltas(); out != nil {
					testOutput := &TestStoreOutput{
						StoreName: output.Name,
					}
					for _, delta := range out.Deltas {

						if output.Name == "store_map_result" {
							o := &pbsubstreamstest.MapResult{}
							err = proto.Unmarshal(delta.OldValue, o)
							require.NoError(t, err)

							n := &pbsubstreamstest.MapResult{}
							err = proto.Unmarshal(delta.NewValue, n)
							require.NoError(t, err)

							testOutput.Deltas = append(testOutput.Deltas, &TestStoreDelta{
								Operation: delta.Operation.String(),
								OldValue:  o,
								NewValue:  n,
							})
						} else {
							testOutput.Deltas = append(testOutput.Deltas, &TestStoreDelta{
								Operation: delta.Operation.String(),
								OldValue:  string(delta.OldValue),
								NewValue:  string(delta.NewValue),
							})
						}
					}
					jsonData, err := json.Marshal(testOutput)
					require.NoError(t, err)
					moduleOutputs = append(moduleOutputs, string(jsonData))
				}
			}
		}
	}
	return
}

func Test_SimpleMapModule(t *testing.T) {
	moduleOutputs := runTest(t, 1, 3, []string{"map_test"})
	require.Equal(t, []string{
		`{"name":"map_test","result":{"block_number":1,"block_hash":"block-1"}}`,
		`{"name":"map_test","result":{"block_number":2,"block_hash":"block-2"}}`,
	}, moduleOutputs)
}

func Test_store_add_int64(t *testing.T) {
	moduleOutputs := runTest(t, 1, 4, []string{"store_add_int64"})
	require.Equal(t, []string{
		`{"name":"store_add_int64","deltas":[{"op":"CREATE","old":"","new":"1"}]}`,
		`{"name":"store_add_int64","deltas":[{"op":"UPDATE","old":"1","new":"2"}]}`,
		`{"name":"store_add_int64","deltas":[{"op":"UPDATE","old":"2","new":"3"}]}`,
	}, moduleOutputs)
}

func Test_store_map_result(t *testing.T) {
	moduleOutputs := runTest(t, 1, 3, []string{"store_map_result"})
	require.Equal(t, []string{
		`{"name":"store_map_result","deltas":[{"op":"CREATE","old":{},"new":{"block_number":1,"block_hash":"block-1"}}]}`,
		`{"name":"store_map_result","deltas":[{"op":"CREATE","old":{},"new":{"block_number":2,"block_hash":"block-2"}}]}`,
	}, moduleOutputs)
}

func Test_MultipleModule(t *testing.T) {
	moduleOutputs := runTest(t, 1, 3, []string{"store_map_result"})
	require.Equal(t, []string{
		`{"name":"store_map_result","deltas":[{"op":"CREATE","old":{},"new":{"block_number":1,"block_hash":"block-1"}}]}`,
		`{"name":"store_map_result","deltas":[{"op":"CREATE","old":{},"new":{"block_number":2,"block_hash":"block-2"}}]}`,
	}, moduleOutputs)
}

func Test_SimpleEmptyObjectMarshall(t *testing.T) {
	b := &pbsubstreamstest.Block{}
	data, err := proto.Marshal(b)
	require.NoError(t, err)
	require.Empty(t, data)
}

package pipeline

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/streamingfast/substreams/reqctx"
	"github.com/streamingfast/substreams/work"
	"go.opentelemetry.io/otel"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/streamingfast/bstream"
	"github.com/streamingfast/dstore"
	tracing "github.com/streamingfast/sf-tracing"
	"github.com/streamingfast/substreams"
	"github.com/streamingfast/substreams/block"
	"github.com/streamingfast/substreams/manifest"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	pbsubstreamstest "github.com/streamingfast/substreams/pb/sf/substreams/v1/test"
	"github.com/streamingfast/substreams/pipeline"
	"github.com/streamingfast/substreams/pipeline/execout/cachev1"
	"github.com/streamingfast/substreams/service/config"
	"github.com/streamingfast/substreams/store"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
)

type blockProcessedCallBack func(p *pipeline.Pipeline, b *bstream.Block, stores store.Map, baseStore dstore.Store)

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
			Step:   int32(bstream.StepNewIrreversible),
		})
	}
	return blocks
}

type TestWorker struct {
	t                      *testing.T
	moduleGraph            *manifest.ModuleGraph
	responseCollector      *responseCollector
	newBlockGenerator      NewTestBlockGenerator
	blockProcessedCallBack blockProcessedCallBack
	testTempDir            string
}

func (w *TestWorker) Run(ctx context.Context, request *pbsubstreams.Request, _ substreams.ResponseFunc) (brange []*block.Range, err error) {
	w.t.Helper()

	ctx, span := reqctx.WithSpan(ctx, "running_job_test")
	defer span.EndWithErr(&err)

	logger := reqctx.Logger(ctx)

	logger.Info("worker running job",
		zap.Strings("output_modules", request.OutputModules),
		zap.Int64("start_block_num", request.StartBlockNum),
		zap.Uint64("stop_block_num", request.StopBlockNum),
	)
	if _, err := processRequest(w.t, ctx, request, w.moduleGraph, nil, w.newBlockGenerator, w.responseCollector, true, w.blockProcessedCallBack, w.testTempDir); err != nil {
		return nil, fmt.Errorf("processing sub request: %w", err)
	}

	return block.Ranges{
		&block.Range{
			StartBlock:        uint64(request.StartBlockNum),
			ExclusiveEndBlock: request.StopBlockNum,
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

func newResponseCollector() *responseCollector {
	return &responseCollector{
		responses: []*pbsubstreams.Response{},
	}
}

func (c *responseCollector) Collect(resp *pbsubstreams.Response) error {
	c.responses = append(c.responses, resp)
	return nil
}

type NewTestBlockGenerator func(startBlock uint64, inclusiveStopBlock uint64) TestBlockGenerator

func processRequest(
	t *testing.T,
	ctx context.Context,
	request *pbsubstreams.Request,
	moduleGraph *manifest.ModuleGraph,
	workerFactory work.WorkerFactory, newGenerator NewTestBlockGenerator, responseCollector *responseCollector, isSubRequest bool, blockProcessedCallBack blockProcessedCallBack, testTempDir string,
) (out []*pbsubstreams.Response, err error) {
	t.Helper()

	var opts []pipeline.Option

	ctx, err = reqctx.WithRequest(ctx, request, isSubRequest)
	require.Nil(t, err)

	baseStoreStore, err := dstore.NewStore(filepath.Join(testTempDir, "test.store"), "", "none", true)
	require.NoError(t, err)

	cachingEngine, err := cachev1.NewEngine(config.RuntimeConfig{StoreSnapshotsSaveInterval: 10, BaseObjectStore: baseStoreStore}, zap.NewNop())
	require.NoError(t, err)
	storeBoundary := pipeline.NewStoreBoundary(10)

	runtimeConfig := config.NewRuntimeConfig(
		10,
		10,
		10,
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

	generator := newGenerator(uint64(request.StartBlockNum), request.StopBlockNum)

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
		if !errors.Is(err, io.EOF) && err != nil {
			return nil, fmt.Errorf("process block: %w", err)
		}
		if blockProcessedCallBack != nil && err == nil {
			blockProcessedCallBack(pipe, bb, pipe.StoreMap, baseStoreStore)
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
type AssertMapOutput struct {
	ModuleName string `json:"name"`
	Result     bool   `json:"result"`
}

func runTest(t *testing.T, startBlock int64, exclusiveEndBlock uint64, moduleNames []string, newBlockGenerator NewTestBlockGenerator, blockProcessedCallBack blockProcessedCallBack) (moduleOutputs []string, err error) {
	if os.Getenv("SUBSTREAMS_INTEGRATION_TESTS") == "" {
		t.Skip("Environment variable SUBSTREAMS_INTEGRATION_TESTS must be set for now to run integration tests")
	}
	ctx := context.Background()
	ctx = reqctx.WithLogger(ctx, zlog)

	testTempDir := os.TempDir()
	if os.Getenv("SUBSTREAMS_INTEGRATION_NO_CLEANUP") == "" {
		defer os.RemoveAll(testTempDir)
	}

	tracingEnabled := os.Getenv("SF_TRACING") != ""
	if tracingEnabled {
		fmt.Println("Running test with tracing enabled: ", os.Getenv("SF_TRACING"))
		err = tracing.SetupOpenTelemetry("substreams")
		require.NoError(t, err)
		ctx = reqctx.WithTracer(ctx, otel.GetTracerProvider().Tracer("service.test"))
		spanCtx, span := reqctx.WithSpan(ctx, "substreams_request_test")
		defer func() {
			span.EndWithErr(nil)
			fmt.Println("Test complete waiting 20s for tracing to be sent")
			time.Sleep(20 * time.Second)
		}()
		ctx = spanCtx
	}

	//todo: compile substreams
	pkg, moduleGraph := processManifest(t, "./testdata/substreams-test-v0.1.0.spkg")

	request := &pbsubstreams.Request{
		StartBlockNum: startBlock,
		StopBlockNum:  exclusiveEndBlock,
		Modules:       pkg.Modules,
		OutputModules: moduleNames,
	}

	responseCollector := newResponseCollector()

	workerFactory := func(_ *zap.Logger) work.Worker {
		// TODO(abourget): this isn't used anymore, but we still need a way to mock this E2E test.
		// Unsure about the best way to do this. Will need to look into it.
		return &TestWorker{
			t:                      t,
			moduleGraph:            moduleGraph,
			responseCollector:      newResponseCollector(),
			newBlockGenerator:      newBlockGenerator,
			blockProcessedCallBack: blockProcessedCallBack,
			testTempDir:            testTempDir,
		}
	}

	if _, err = processRequest(t, ctx, request, moduleGraph, workerFactory, newBlockGenerator, responseCollector, false, blockProcessedCallBack, testTempDir); err != nil {
		return nil, fmt.Errorf("running test: %w", err)
	}

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
					if output.Name == "test_map" {
						r := &pbsubstreamstest.MapResult{}
						err := proto.Unmarshal(out.Value, r)
						require.NoError(t, err)

						out := &TestMapOutput{
							ModuleName: output.Name,
							Result:     r,
						}
						jsonData, err := json.Marshal(out)
						require.NoError(t, err)
						moduleOutputs = append(moduleOutputs, string(jsonData))
						continue
					}

					if strings.HasPrefix(output.Name, "assert") {
						assertOut := &AssertMapOutput{
							ModuleName: output.Name,
							Result:     len(out.Value) > 0,
						}

						jsonData, err := json.Marshal(assertOut)
						require.NoError(t, err)
						moduleOutputs = append(moduleOutputs, string(jsonData))
					}

				}
				if out := output.GetStoreDeltas(); out != nil {
					testOutput := &TestStoreOutput{
						StoreName: output.Name,
					}
					for _, delta := range out.Deltas {

						if output.Name == "test_store_proto" {
							o := &pbsubstreamstest.MapResult{}
							err := proto.Unmarshal(delta.OldValue, o)
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
	newBlockGenerator := func(startBlock uint64, inclusiveStopBlock uint64) TestBlockGenerator {
		return &LinearBlockGenerator{
			startBlock:         startBlock,
			inclusiveStopBlock: inclusiveStopBlock,
		}
	}
	moduleOutputs, err := runTest(t, 10, 12, []string{"test_map"}, newBlockGenerator, nil)
	require.NoError(t, err)
	require.Equal(t, []string{
		`{"name":"test_map","result":{"block_number":10,"block_hash":"block-10"}}`,
		`{"name":"test_map","result":{"block_number":11,"block_hash":"block-11"}}`,
	}, moduleOutputs)
}

func Test_test_store_proto(t *testing.T) {
	newBlockGenerator := func(startBlock uint64, inclusiveStopBlock uint64) TestBlockGenerator {
		return &LinearBlockGenerator{
			startBlock:         startBlock,
			inclusiveStopBlock: inclusiveStopBlock,
		}
	}
	moduleOutputs, err := runTest(t, 10, 12, []string{"test_store_proto"}, newBlockGenerator, nil)
	require.NoError(t, err)

	require.Equal(t, []string{
		`{"name":"test_store_proto","deltas":[{"op":"CREATE","old":{},"new":{"block_number":10,"block_hash":"block-10"}}]}`,
		`{"name":"test_store_proto","deltas":[{"op":"CREATE","old":{},"new":{"block_number":11,"block_hash":"block-11"}}]}`,
	}, moduleOutputs)
}

func Test_MultipleModule(t *testing.T) {
	newBlockGenerator := func(startBlock uint64, inclusiveStopBlock uint64) TestBlockGenerator {
		return &LinearBlockGenerator{
			startBlock:         startBlock,
			inclusiveStopBlock: inclusiveStopBlock,
		}
	}
	moduleOutputs, err := runTest(t, 10, 12, []string{"test_map", "test_store_add_int64", "test_store_proto"}, newBlockGenerator, nil)
	require.NoError(t, err)

	require.Equal(t, []string{
		`{"name":"test_map","result":{"block_number":10,"block_hash":"block-10"}}`,
		`{"name":"test_store_add_int64","deltas":[{"op":"UPDATE","old":"9","new":"10"}]}`,
		`{"name":"test_store_proto","deltas":[{"op":"CREATE","old":{},"new":{"block_number":10,"block_hash":"block-10"}}]}`,
		`{"name":"test_map","result":{"block_number":11,"block_hash":"block-11"}}`,
		`{"name":"test_store_add_int64","deltas":[{"op":"UPDATE","old":"10","new":"11"}]}`,
		`{"name":"test_store_proto","deltas":[{"op":"CREATE","old":{},"new":{"block_number":11,"block_hash":"block-11"}}]}`,
	}, moduleOutputs)
}

func Test_MultipleModule_Batch(t *testing.T) {
	newBlockGenerator := func(startBlock uint64, inclusiveStopBlock uint64) TestBlockGenerator {
		return &LinearBlockGenerator{
			startBlock:         startBlock,
			inclusiveStopBlock: inclusiveStopBlock,
		}
	}

	//todo: Need to validate the storage file

	_, err := runTest(t, 1000, 1021, []string{"test_store_add_bigint", "assert_test_store_add_bigint"}, newBlockGenerator, nil)
	require.NoError(t, err)
}

func Test_MultipleModule_Batch_2(t *testing.T) {
	newBlockGenerator := func(startBlock uint64, inclusiveStopBlock uint64) TestBlockGenerator {
		return &LinearBlockGenerator{
			startBlock:         startBlock,
			inclusiveStopBlock: inclusiveStopBlock,
		}
	}

	moduleOutputs, err := runTest(t, 110, 112, []string{"test_map", "test_store_proto"}, newBlockGenerator, nil)
	require.NoError(t, err)

	require.Equal(t, []string{
		`{"name":"test_map","result":{"block_number":110,"block_hash":"block-110"}}`,
		`{"name":"test_store_proto","deltas":[{"op":"CREATE","old":{},"new":{"block_number":110,"block_hash":"block-110"}}]}`,
		`{"name":"test_map","result":{"block_number":111,"block_hash":"block-111"}}`,
		`{"name":"test_store_proto","deltas":[{"op":"CREATE","old":{},"new":{"block_number":111,"block_hash":"block-111"}}]}`,
	}, moduleOutputs)
}

func Test_MultipleModule_Batch_Output_Written(t *testing.T) {
	newBlockGenerator := func(startBlock uint64, inclusiveStopBlock uint64) TestBlockGenerator {
		return &LinearBlockGenerator{
			startBlock:         startBlock,
			inclusiveStopBlock: inclusiveStopBlock,
		}
	}

	outputFilesLen := 0
	moduleOutputs, err := runTest(t, 110, 112, []string{"test_map", "test_store_proto"},
		newBlockGenerator,
		func(p *pipeline.Pipeline, b *bstream.Block, stores store.Map, baseStore dstore.Store) {
			err := baseStore.Walk(context.Background(), "", func(filename string) (err error) {
				if strings.Contains(filename, "output") {
					outputFilesLen++
				}
				return nil
			})
			require.NoError(t, err)
		},
	)
	require.NoError(t, err)

	require.NotZero(t, moduleOutputs)
	require.NotZero(t, outputFilesLen)
}

//func Test_test_store_add_bigint(t *testing.T) {
//	newBlockGenerator := func(startBlock uint64, inclusiveStopBlock uint64) TestBlockGenerator {
//		return &LinearBlockGenerator{
//			startBlock:         startBlock,
//			inclusiveStopBlock: inclusiveStopBlock,
//		}
//	}
//
//	_, err := runTest(t, 1, 1001, []string{"test_store_add_bigint", "assert_test_store_add_bigint"}, newBlockGenerator, nil)
//	require.NoError(t, err)
//
//}
func Test_test_store_delete_prefix(t *testing.T) {
	newBlockGenerator := func(startBlock uint64, inclusiveStopBlock uint64) TestBlockGenerator {
		return &LinearBlockGenerator{
			startBlock:         startBlock,
			inclusiveStopBlock: inclusiveStopBlock,
		}
	}
	_, err := runTest(t, 30, 41, []string{"test_store_delete_prefix", "assert_test_store_delete_prefix"}, newBlockGenerator, func(p *pipeline.Pipeline, b *bstream.Block, stores store.Map, baseStore dstore.Store) {
		if b.Number == 40 {
			s, storeFound := stores.Get("test_store_delete_prefix")
			require.True(t, storeFound)
			require.Equal(t, uint64(1), s.Length())
		}
	})
	require.NoError(t, err)
}

//
//// -------------------- StoreAddI64 -------------------- //
//func Test_test_store_add_i64(t *testing.T) {
//	newBlockGenerator := func(startBlock uint64, inclusiveStopBlock uint64) TestBlockGenerator {
//		return &LinearBlockGenerator{
//			startBlock:         startBlock,
//			inclusiveStopBlock: inclusiveStopBlock,
//		}
//	}
//	_, err := runTest(t, 1, 2, []string{"setup_test_store_add_i64", "assert_test_store_add_i64"}, newBlockGenerator, nil)
//	require.NoError(t, err)
//}
//
//func Test_test_store_add_i64_deltas(t *testing.T) {
//	newBlockGenerator := func(startBlock uint64, inclusiveStopBlock uint64) TestBlockGenerator {
//		return &LinearBlockGenerator{
//			startBlock:         startBlock,
//			inclusiveStopBlock: inclusiveStopBlock,
//		}
//	}
//	_, err := runTest(t, 1, 2, []string{"setup_test_store_add_i64", "assert_test_store_add_i64_deltas"}, newBlockGenerator, nil)
//	require.NoError(t, err)
//}
//
//// -------------------- StoreSetI64/StoreGetI64 -------------------- //
//func Test_test_store_set_i64(t *testing.T) {
//	newBlockGenerator := func(startBlock uint64, inclusiveStopBlock uint64) TestBlockGenerator {
//		return &LinearBlockGenerator{
//			startBlock:         startBlock,
//			inclusiveStopBlock: inclusiveStopBlock,
//		}
//	}
//	_, err := runTest(t, 20, 31, []string{"setup_test_store_set_i64", "assert_test_store_set_i64"}, newBlockGenerator, nil)
//	require.NoError(t, err)
//}

func Test_assert_all_test(t *testing.T) {
	newBlockGenerator := func(startBlock uint64, inclusiveStopBlock uint64) TestBlockGenerator {
		return &LinearBlockGenerator{
			startBlock:         startBlock,
			inclusiveStopBlock: inclusiveStopBlock,
		}
	}
	_, err := runTest(t, 20, 31, []string{"assert_all_test"}, newBlockGenerator, nil)
	require.NoError(t, err)
}

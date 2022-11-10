package integration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/streamingfast/substreams/pipeline/cache"

	"github.com/streamingfast/bstream"
	"github.com/streamingfast/dstore"
	tracing "github.com/streamingfast/sf-tracing"
	"github.com/streamingfast/substreams/manifest"
	"github.com/streamingfast/substreams/orchestrator/outputmodules"
	"github.com/streamingfast/substreams/orchestrator/work"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	pbsubstreamstest "github.com/streamingfast/substreams/pb/sf/substreams/v1/test"
	"github.com/streamingfast/substreams/pipeline"
	"github.com/streamingfast/substreams/reqctx"
	"github.com/streamingfast/substreams/service/config"
	"github.com/streamingfast/substreams/wasm"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.uber.org/zap"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"
)

type testRun struct {
	Cursor                 *bstream.Cursor
	StartBlock             int64
	ExclusiveEndBlock      uint64
	ModuleNames            []string
	SubrequestsSplitSize   uint64
	ParallelSubrequests    uint64
	NewBlockGenerator      BlockGeneratorFactory
	BlockProcessedCallback blockProcessedCallBack
	ProductionMode         bool

	Responses []*pbsubstreams.Response
}

func newTestRun(startBlock int64, exclusiveEndBlock uint64, moduleNames ...string) *testRun {
	return &testRun{StartBlock: startBlock, ExclusiveEndBlock: exclusiveEndBlock, ModuleNames: moduleNames, ProductionMode: true}
}

func (f *testRun) Run(t *testing.T) error {
	ctx := context.Background()
	ctx = reqctx.WithLogger(ctx, zlog)

	testTempDir := t.TempDir()
	fmt.Println("Running test in temp dir: ", testTempDir)

	ctx = withTestTracing(t, ctx)

	//todo: compile substreams
	pkg := manifest.TestReadManifest(t, "./testdata/substreams-test-v0.1.0.spkg")

	opaqueCursor := ""
	if f.Cursor != nil {
		opaqueCursor = f.Cursor.ToOpaque()
	}
	request := &pbsubstreams.Request{
		StartBlockNum:  f.StartBlock,
		StopBlockNum:   f.ExclusiveEndBlock,
		StartCursor:    opaqueCursor,
		Modules:        pkg.Modules,
		OutputModules:  f.ModuleNames,
		ProductionMode: f.ProductionMode,
	}

	if f.SubrequestsSplitSize == 0 {
		f.SubrequestsSplitSize = 10
	}
	if f.ParallelSubrequests == 0 {
		f.ParallelSubrequests = 1
	}

	// TODO(abourget): why are there two response collectors?
	responseCollector := newResponseCollector()

	newBlockGenerator := func(startBlock uint64, inclusiveStopBlock uint64) TestBlockGenerator {
		return &LinearBlockGenerator{
			startBlock:         startBlock,
			inclusiveStopBlock: inclusiveStopBlock,
		}
	}
	if f.NewBlockGenerator != nil {
		newBlockGenerator = f.NewBlockGenerator
	}

	workerFactory := func(_ *zap.Logger) work.Worker {
		w := &TestWorker{
			t:                      t,
			responseCollector:      newResponseCollector(),
			newBlockGenerator:      newBlockGenerator,
			blockProcessedCallBack: f.BlockProcessedCallback,
			testTempDir:            testTempDir,
		}
		return w
	}

	if err := processRequest(t, ctx, request, workerFactory, newBlockGenerator, responseCollector, false, f.BlockProcessedCallback, testTempDir, f.SubrequestsSplitSize, f.ParallelSubrequests); err != nil {
		return fmt.Errorf("running test: %w", err)
	}

	f.Responses = responseCollector.responses

	return nil
}

func (f *testRun) Logs() (out []string) {
	for _, response := range f.Responses {
		switch r := response.Message.(type) {
		case *pbsubstreams.Response_Data:
			for _, output := range r.Data.Outputs {
				for _, log := range output.DebugLogs {
					out = append(out, log)
				}
			}
		}
	}
	return
}

func (f *testRun) MapOutput(modName string) string {
	var moduleOutputs []string
	for _, response := range f.Responses {
		switch r := response.Message.(type) {
		case *pbsubstreams.Response_Data:
			for _, output := range r.Data.Outputs {
				if output.Name != modName {
					continue
				}
				mapout := output.GetMapOutput()
				if mapout == nil {
					continue
				}

				res := &pbsubstreamstest.MapResult{}
				err := proto.Unmarshal(mapout.Value, res)
				if err != nil {
					panic("marshaling proto: " + err.Error())
				}

				out := &TestMapOutput{
					ModuleName: output.Name,
					Result:     res,
				}
				jsonData, err := json.Marshal(out)
				if err != nil {
					panic("marshaling json: " + err.Error())
				}
				moduleOutputs = append(moduleOutputs, fmt.Sprintf("%d: %s: %s", r.Data.Clock.Number, output.Name, string(jsonData)))
			}
		}
	}
	return "\n" + strings.Join(moduleOutputs, "\n")
}

func (f *testRun) ModuleOutputs(t *testing.T) (moduleOutputs []string) {
	// TODO(abourget): get rid of this method, rather have more spotted
	// methods to return what we're interested in, and compare that.
	// We can then run multiple spotted assertions.
	for _, response := range f.Responses {
		switch r := response.Message.(type) {
		case *pbsubstreams.Response_Progress:
			_ = r.Progress
		case *pbsubstreams.Response_SnapshotData:
			_ = r.SnapshotData
		case *pbsubstreams.Response_SnapshotComplete:
			_ = r.SnapshotComplete
		case *pbsubstreams.Response_Data:
			for _, output := range r.Data.Outputs {
				for _, log := range output.DebugLogs {
					fmt.Println("LOG: ", log)
				}
				if out := output.GetMapOutput(); out != nil {
					if output.Name == "test_map" {
						// TODO(abourget): get rid of those side effects, have the caller do the job.. and
						// test where appropriate. (even if we have a `assertCommonThings()` method that does this.
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
						// TODO(abourget): get rid of those assertion side effects..
						// have the caller assert the right things
						assertOut := &AssertMapOutput{
							ModuleName: output.Name,
							Result:     len(out.Value) > 0,
						}

						jsonData, err := json.Marshal(assertOut)
						require.NoError(t, err)
						moduleOutputs = append(moduleOutputs, string(jsonData))
					}
				}
				if out := output.GetDebugStoreDeltas(); out != nil {
					testOutput := &TestStoreOutput{
						StoreName: output.Name,
					}
					for _, delta := range out.Deltas {

						if output.Name == "test_store_proto" {
							// TODO(abourget): same here, get rid of that hard-coded test assertion within the
							// module outputs function call.
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
	return moduleOutputs
}

func runTest(
	t *testing.T,
	cursor *bstream.Cursor,
	startBlock int64,
	exclusiveEndBlock uint64,
	moduleNames []string,
	subrequestsSplitSize uint64,
	parallelSubrequests uint64,
	newBlockGenerator BlockGeneratorFactory,
	blockProcessedCallBack blockProcessedCallBack,
) (moduleOutputs []string, err error) {
	run := newTestRun(startBlock, exclusiveEndBlock, moduleNames...)
	run.Cursor = cursor
	run.SubrequestsSplitSize = subrequestsSplitSize
	run.ParallelSubrequests = parallelSubrequests
	run.NewBlockGenerator = newBlockGenerator
	run.BlockProcessedCallback = blockProcessedCallBack
	if err := run.Run(t); err != nil {
		return nil, err
	}

	return run.ModuleOutputs(t), nil
}

func withTestTracing(t *testing.T, ctx context.Context) context.Context {
	t.Helper()
	tracingEnabled := os.Getenv("SF_TRACING") != ""
	if tracingEnabled {
		fmt.Println("Running test with tracing enabled: ", os.Getenv("SF_TRACING"))
		require.NoError(t, tracing.SetupOpenTelemetry("substreams"))
		ctx = reqctx.WithTracer(ctx, otel.GetTracerProvider().Tracer("service.test"))
		spanCtx, span := reqctx.WithSpan(ctx, "substreams_request_test")
		defer func() {
			span.End()
			fmt.Println("Test complete waiting 20s for tracing to be sent")
			time.Sleep(20 * time.Second)
		}()
		ctx = spanCtx
	}
	return ctx
}

func processRequest(
	t *testing.T,
	ctx context.Context,
	request *pbsubstreams.Request,
	workerFactory work.WorkerFactory,
	newGenerator BlockGeneratorFactory,
	responseCollector *responseCollector,
	isSubRequest bool,
	blockProcessedCallBack blockProcessedCallBack,
	testTempDir string,
	subrequestsSplitSize uint64,
	parallelSubrequests uint64,
) error {
	t.Helper()

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

	const blockType = "sf.substreams.v1.test.Block"

	cachingEngine, err := cache.NewEngine(runtimeConfig, blockType, zap.NewNop())
	require.NoError(t, err)

	require.NoError(t, outputmodules.ValidateRequest(request, blockType))

	outputGraph, err := outputmodules.NewOutputModuleGraph(request)
	require.NoError(t, err)

	storeConfigs, err := pipeline.InitializeStoreConfigs(outputGraph, runtimeConfig.BaseObjectStore)
	require.NoError(t, err)

	stores := pipeline.NewStores(storeConfigs, runtimeConfig.StoreSnapshotsSaveInterval, reqDetails.RequestStartBlockNum, request.StopBlockNum, isSubRequest)

	pipe := pipeline.New(
		ctx,
		outputGraph,
		stores,
		wasm.NewRuntime(nil),
		cachingEngine,
		runtimeConfig,
		responseCollector.Collect,
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

type nooptrailable struct{}

func (n nooptrailable) SetTrailer(md metadata.MD) {}

type TestRunner struct {
	t *testing.T

	baseStoreStore         dstore.Store
	blockProcessedCallBack blockProcessedCallBack
	request                *pbsubstreams.Request
	blockGeneratorFactory  BlockGeneratorFactory
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

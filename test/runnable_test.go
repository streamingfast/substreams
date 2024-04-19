package integration

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.opentelemetry.io/otel/attribute"

	"github.com/streamingfast/bstream"
	"github.com/streamingfast/dstore"
	tracing "github.com/streamingfast/sf-tracing"
	"github.com/streamingfast/shutter"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.uber.org/zap"

	"github.com/streamingfast/substreams/manifest"
	"github.com/streamingfast/substreams/orchestrator/work"
	pbssinternal "github.com/streamingfast/substreams/pb/sf/substreams/intern/v2"
	pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/pipeline"
	"github.com/streamingfast/substreams/reqctx"
	"github.com/streamingfast/substreams/service"
	"github.com/streamingfast/substreams/service/config"
)

type testPreWork func(t *testing.T, run *testRun, workerFactory work.WorkerFactory)

type testRun struct {
	Package                *pbsubstreams.Package
	Cursor                 *bstream.Cursor
	StartBlock             int64
	ExclusiveEndBlock      uint64
	ModuleName             string
	ParallelSubrequests    uint64
	NewBlockGenerator      BlockGeneratorFactory
	BlockProcessedCallback blockProcessedCallBack
	LinearHandoffBlockNum  uint64 // defaults to the request's StopBlock, so no linear handoff, only backprocessing
	ProductionMode         bool
	// PreWork can be done to perform tier2 work in advance, to simulate when
	// pre-existing data is available in different conditions
	PreWork testPreWork
	Context context.Context // custom top-level context, defaults to context.Background()

	Params map[string]string

	Responses []*pbsubstreamsrpc.Response
	TempDir   string
}

func newTestRun(t *testing.T, startBlock int64, linearHandoffBlock, exclusiveEndBlock uint64, moduleName string) *testRun {
	pkg := manifest.TestReadManifest(t, "./testdata/substreams-test-v0.1.0.spkg")

	return &testRun{Package: pkg, StartBlock: startBlock, ExclusiveEndBlock: exclusiveEndBlock, ModuleName: moduleName, LinearHandoffBlockNum: linearHandoffBlock}
}

func (f *testRun) Run(t *testing.T, testName string) error {
	ctx := context.Background()
	if f.Context != nil {
		ctx = f.Context
	}

	ctx = reqctx.WithLogger(ctx, zlog)

	testTempDir := t.TempDir()
	f.TempDir = testTempDir
	os.Setenv("TEST_TEMP_DIR", f.TempDir)

	ctx, endFunc := withTestTracing(t, ctx, testName)
	defer endFunc()
	if f.Context == nil {
		f.Context = ctx
	}

	opaqueCursor := ""
	if f.Cursor != nil {
		opaqueCursor = f.Cursor.ToOpaque()
	}
	request := &pbsubstreamsrpc.Request{
		StartBlockNum:  f.StartBlock,
		StopBlockNum:   f.ExclusiveEndBlock,
		StartCursor:    opaqueCursor,
		Modules:        f.Package.Modules,
		OutputModule:   f.ModuleName,
		ProductionMode: f.ProductionMode,
	}

	if f.Params != nil {
		for k, v := range f.Params {
			var found bool
			for _, mod := range f.Package.Modules.Modules {
				if k == mod.Name {
					assert.NotZero(t, len(mod.Inputs))
					p := mod.Inputs[0].GetParams()
					assert.NotNil(t, p)
					p.Value = v
					found = true
				}
			}
			assert.True(t, found)
		}
	}

	if f.ParallelSubrequests == 0 {
		f.ParallelSubrequests = 1
	}

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
		return &TestWorker{
			t:                      t,
			responseCollector:      newResponseCollector(),
			newBlockGenerator:      newBlockGenerator,
			blockProcessedCallBack: f.BlockProcessedCallback,
			testTempDir:            testTempDir,
			id:                     workerID.Inc(),
		}
	}

	if f.PreWork != nil {
		f.PreWork(t, f, workerFactory)
	}

	if err := processRequest(t, ctx, request, workerFactory, newBlockGenerator, responseCollector, false, f.BlockProcessedCallback, testTempDir, f.ParallelSubrequests, f.LinearHandoffBlockNum); err != nil {
		return fmt.Errorf("running test: %w", err)
	}

	f.Responses = responseCollector.responses

	return nil
}

func (f *testRun) Logs() (out []string) {
	for _, response := range f.Responses {
		switch r := response.Message.(type) {
		case *pbsubstreamsrpc.Response_BlockScopedData:
			for _, output := range r.BlockScopedData.AllModuleOutputs() {
				if debugInfo := output.DebugInfo(); debugInfo != nil {
					out = append(out, debugInfo.GetLogs()...)
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
		case *pbsubstreamsrpc.Response_BlockScopedData:
			for _, output := range r.BlockScopedData.AllModuleOutputs() {
				if output.Name() != modName {
					continue
				}
				if !output.IsMap() {
					continue
				}
				mapout := output.MapOutput.GetMapOutput()
				if mapout == nil {
					continue
				}

				// TODO(abourget): use our library to decode those protobufs on the fly, and
				// allow us to test with that as JSON.
				// That MapResult right now is pretty useless.. it doesn't really
				// honor what is
				//res := &pbsubstreamstest.MapResult{}
				//err := proto.Unmarshal(mapout.Value, res)
				//if err != nil {
				//	panic("marshaling proto: " + err.Error())
				//}
				res := hex.EncodeToString(mapout.Value)
				//jsonData, err := json.Marshal(res)
				//if err != nil {
				//	panic("marshaling json: " + err.Error())
				//}
				moduleOutputs = append(moduleOutputs, fmt.Sprintf("%d: %s: %s", r.BlockScopedData.Clock.Number, output.Name(), res))
			}
		}
	}
	return "\n" + strings.Join(moduleOutputs, "\n")
}

func withTestTracing(t *testing.T, ctx context.Context, testName string) (context.Context, func()) {
	t.Helper()
	tracingEnabled := os.Getenv("SF_TRACING") != ""
	endFunc := func() {}
	if tracingEnabled {
		fmt.Println("Running test with tracing enabled: ", os.Getenv("SF_TRACING"))
		require.NoError(t, tracing.SetupOpenTelemetry(ctx, "substreams"))
		ctx = reqctx.WithTracer(ctx, otel.GetTracerProvider().Tracer("service.test"))
		spanCtx, span := reqctx.WithSpan(ctx, testName)
		endFunc = func() {
			span.End()
			fmt.Println("Test complete waiting 20s for tracing to be sent")
			time.Sleep(20 * time.Second)
		}
		ctx = spanCtx
		_, newSpan := reqctx.Tracer(ctx).Start(ctx, "something_start")
		newSpan.SetAttributes(attribute.Int64("block_num", 1))
		time.Sleep(2 * time.Second)
		newSpan.AddEvent("something_append")
		newSpan.End()

	}
	return ctx, endFunc
}

func processInternalRequest(
	t *testing.T,
	ctx context.Context,
	request *pbssinternal.ProcessRangeRequest,
	workerFactory work.WorkerFactory,
	newGenerator BlockGeneratorFactory,
	responseCollector *responseCollector,
	blockProcessedCallBack blockProcessedCallBack,
	testTempDir string,
) error {
	t.Helper()

	baseStoreStore, err := dstore.NewStore(filepath.Join(testTempDir, "test.store"), "zst", "zstd", true)
	require.NoError(t, err)

	taggedStore, err := baseStoreStore.SubStore("tag")
	require.NoError(t, err)
	tr := &TestRunner{
		t:                      t,
		baseStoreStore:         taggedStore,
		blockProcessedCallBack: blockProcessedCallBack,
		blockGeneratorFactory:  newGenerator,
	}

	runtimeConfig := config.RuntimeConfig{
		StateBundleSize:            10,
		DefaultParallelSubrequests: 10,
		BaseObjectStore:            baseStoreStore,
		DefaultCacheTag:            "tag",
		WorkerFactory:              workerFactory,
	}
	svc := service.TestNewServiceTier2(runtimeConfig, tr.StreamFactory)

	return svc.TestProcessRange(ctx, request, responseCollector.Collect)
}

func processRequest(
	t *testing.T,
	ctx context.Context,
	request *pbsubstreamsrpc.Request,
	workerFactory work.WorkerFactory,
	newGenerator BlockGeneratorFactory,
	responseCollector *responseCollector,
	isSubRequest bool,
	blockProcessedCallBack blockProcessedCallBack,
	testTempDir string,
	parallelSubrequests uint64,
	linearHandoffBlockNum uint64,
) error {
	t.Helper()

	baseStoreStore, err := dstore.NewStore(filepath.Join(testTempDir, "test.store"), "zst", "zstd", true)
	require.NoError(t, err)

	taggedStore, err := baseStoreStore.SubStore("tag")
	require.NoError(t, err)

	tr := &TestRunner{
		t:                      t,
		baseStoreStore:         taggedStore,
		blockProcessedCallBack: blockProcessedCallBack,
		blockGeneratorFactory:  newGenerator,
	}

	runtimeConfig := config.RuntimeConfig{
		StateBundleSize:            10,
		DefaultParallelSubrequests: parallelSubrequests,
		BaseObjectStore:            baseStoreStore,
		DefaultCacheTag:            "tag",
		WorkerFactory:              workerFactory,
		MaxJobsAhead:               10,
	}

	svc := service.TestNewService(runtimeConfig, linearHandoffBlockNum, tr.StreamFactory)
	return svc.TestBlocks(ctx, isSubRequest, request, responseCollector.Collect)
}

type TestRunner struct {
	t *testing.T
	*shutter.Shutter

	baseStoreStore         dstore.Store
	blockProcessedCallBack blockProcessedCallBack
	blockGeneratorFactory  BlockGeneratorFactory

	pipe      *pipeline.Pipeline
	generator TestBlockGenerator
}

func (r *TestRunner) StreamFactory(_ context.Context, h bstream.Handler, startBlockNum int64, stopBlockNum uint64, _ string, _ bool, _ bool, _ *zap.Logger) (service.Streamable, error) {
	r.pipe = h.(*pipeline.Pipeline)
	r.Shutter = shutter.New()
	r.generator = r.blockGeneratorFactory(uint64(startBlockNum), stopBlockNum)
	return r, nil
}

func (r *TestRunner) Run(context.Context) error {
	for _, generatedBlock := range r.generator.Generate() {
		blk := generatedBlock.block
		err := r.pipe.ProcessBlock(blk, generatedBlock.obj)

		if err != nil && !errors.Is(err, io.EOF) {
			return fmt.Errorf("process block: %w", err)
		}
		if errors.Is(err, io.EOF) {
			return err
		}

		if r.blockProcessedCallBack != nil {
			r.blockProcessedCallBack(&execContext{
				block:     blk,
				stores:    r.pipe.GetStoreMap(),
				baseStore: r.baseStoreStore,
			})
		}
	}
	return nil
}

type Obj struct {
	cursor             *bstream.Cursor
	step               bstream.StepType
	reorgJunctionBlock bstream.BlockRef
}

func (o *Obj) Cursor() *bstream.Cursor {
	return o.cursor
}

func (o *Obj) Step() bstream.StepType {
	return o.step
}

func (o *Obj) FinalBlockHeight() uint64 {
	return o.cursor.LIB.Num()
}

func (o *Obj) ReorgJunctionBlock() bstream.BlockRef {
	return o.reorgJunctionBlock
}

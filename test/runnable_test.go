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

	"google.golang.org/grpc/metadata"

	"github.com/streamingfast/shutter"

	"github.com/streamingfast/substreams/service"

	"github.com/streamingfast/bstream"
	"github.com/streamingfast/dstore"
	tracing "github.com/streamingfast/sf-tracing"
	"github.com/streamingfast/substreams/manifest"
	"github.com/streamingfast/substreams/orchestrator/work"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	pbsubstreamstest "github.com/streamingfast/substreams/pb/sf/substreams/v1/test"
	"github.com/streamingfast/substreams/pipeline"
	"github.com/streamingfast/substreams/reqctx"
	"github.com/streamingfast/substreams/service/config"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.uber.org/zap"
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
	LinearHandoffBlockNum  uint64 // defaults to the request's StopBlock, so no linear handoff, only backprocessing
	ProductionMode         bool
	Context                context.Context // custom top-level context, defaults to context.Background()

	Responses []*pbsubstreams.Response
	TempDir   string
}

func newTestRun(startBlock int64, linearHandoffBlock, exclusiveEndBlock uint64, moduleNames ...string) *testRun {
	return &testRun{StartBlock: startBlock, ExclusiveEndBlock: exclusiveEndBlock, ModuleNames: moduleNames, LinearHandoffBlockNum: linearHandoffBlock}
}

func (f *testRun) Run(t *testing.T) error {
	ctx := context.Background()
	if f.Context != nil {
		ctx = f.Context
	}
	ctx = reqctx.WithLogger(ctx, zlog)

	testTempDir := t.TempDir()
	fmt.Println("Running test in temp dir: ", testTempDir)
	f.TempDir = testTempDir

	ctx = withTestTracing(t, ctx)

	//todo: compile substreams
	//pkg := manifest.TestReadManifest(t, "./testdata/substreams-test-v0.1.0.spkg")
	pkg := manifest.TestReadManifest(t, "./testdata/simple_substreams/substreams.yaml")

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

	if err := processRequest(t, ctx, request, workerFactory, newBlockGenerator, responseCollector, false, f.BlockProcessedCallback, testTempDir, f.SubrequestsSplitSize, f.ParallelSubrequests, f.LinearHandoffBlockNum); err != nil {
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
				moduleOutputs = append(moduleOutputs, fmt.Sprintf("%d: %s: %s", r.Data.Clock.Number, output.Name, res))
			}
		}
	}
	return "\n" + strings.Join(moduleOutputs, "\n")
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
	linearHandoffBlockNum uint64,
) error {
	t.Helper()

	baseStoreStore, err := dstore.NewStore(filepath.Join(testTempDir, "test.store"), "", "none", true)
	require.NoError(t, err)

	tr := &TestRunner{
		t:                      t,
		baseStoreStore:         baseStoreStore,
		blockProcessedCallBack: blockProcessedCallBack,
		blockGeneratorFactory:  newGenerator,
	}
	runtimeConfig := config.NewRuntimeConfig(
		10,
		10,
		subrequestsSplitSize,
		parallelSubrequests,
		baseStoreStore,
		workerFactory,
	)
	svc := service.TestNewService(runtimeConfig, linearHandoffBlockNum, tr.StreamFactory)

	if isSubRequest {
		ctx = metadata.NewIncomingContext(ctx, metadata.MD{"substreams-partial-mode": []string{"true"}})
	}
	return svc.TestBlocks(ctx, request, responseCollector.Collect)
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

func (r *TestRunner) StreamFactory(h bstream.Handler, startBlockNum int64, stopBlockNum uint64, cursor string) (service.Streamable, error) {
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

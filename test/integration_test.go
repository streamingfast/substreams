package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/streamingfast/bstream"
	"github.com/streamingfast/dstore"
	tracing "github.com/streamingfast/sf-tracing"
	"github.com/streamingfast/substreams/manifest"
	"github.com/streamingfast/substreams/orchestrator/work"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	pbsubstreamstest "github.com/streamingfast/substreams/pb/sf/substreams/v1/test"
	"github.com/streamingfast/substreams/pipeline"
	"github.com/streamingfast/substreams/reqctx"
	"github.com/streamingfast/substreams/store"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
)

func runTest(
	t *testing.T,
	cursor *bstream.Cursor,
	startBlock int64,
	exclusiveEndBlock uint64,
	moduleNames []string,
	subrequestsSplitSize uint64,
	parallelSubrequests uint64, newBlockGenerator NewTestBlockGenerator, blockProcessedCallBack blockProcessedCallBack,
) (moduleOutputs []string, err error) {
	ctx := context.Background()
	ctx = reqctx.WithLogger(ctx, zlog)

	testTempDir := t.TempDir()
	fmt.Println("Running test in temp dir: ", testTempDir)

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

	//todo: compile substreams
	pkg := manifest.TestReadManifest(t, "./testdata/substreams-test-v0.1.0.spkg")

	opaqueCursor := ""
	if cursor != nil {
		opaqueCursor = cursor.ToOpaque()
	}
	request := &pbsubstreams.Request{
		StartBlockNum: startBlock,
		StopBlockNum:  exclusiveEndBlock,
		StartCursor:   opaqueCursor,
		Modules:       pkg.Modules,
		OutputModules: moduleNames,
	}

	responseCollector := newResponseCollector()

	workerFactory := func(_ *zap.Logger) work.Worker {
		w := &TestWorker{
			t:                      t,
			responseCollector:      newResponseCollector(),
			newBlockGenerator:      newBlockGenerator,
			blockProcessedCallBack: blockProcessedCallBack,
			testTempDir:            testTempDir,
		}
		return w
	}

	if err = processRequest(t, ctx, request, workerFactory, newBlockGenerator, responseCollector, false, blockProcessedCallBack, testTempDir, subrequestsSplitSize, parallelSubrequests); err != nil {
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
	moduleOutputs, err := runTest(t, nil, 10, 12, []string{"test_map"}, 10, 1, newBlockGenerator, nil)
	require.NoError(t, err)
	require.Equal(t, []string{
		`{"name":"test_map","result":{"block_number":10,"block_hash":"block-10"}}`,
		`{"name":"test_map","result":{"block_number":11,"block_hash":"block-11"}}`,
	}, moduleOutputs)

	runtime.GC()

}

//todo:
// 1. add test and new block generator (string and parse easier) that generates
//		different sequence of blocks and test with a store add the value in the store
// 	- 10a, 11a, 12b, 12a, 13a
//   new10a, new11a, new12b, undo12b, new12a, new13a (with some irreversible steps maybe...)
// 2. also expected field validation for the cursor and the step type

func Test_AddBigIntWithCursorGeneratorStepNew(t *testing.T) { // todo: change test name
	t.Skipf("todo(colin): fix integration tests")
	forkDbGenerator := &ForkBlockGenerator{
		initialLIB: bstream.NewBlockRef("0a", 0),
		forkBlockRefs: []*ForkBlockRef{
			{blockRef: bstream.NewBlockRef("1a", 1), previousID: "0a", libBlockRef: bstream.NewBlockRef("0a", 0)},
			{blockRef: bstream.NewBlockRef("2a", 2), previousID: "1a", libBlockRef: bstream.NewBlockRef("0a", 0)},
			{blockRef: bstream.NewBlockRef("3a", 3), previousID: "2a", libBlockRef: bstream.NewBlockRef("0a", 0)},
			{blockRef: bstream.NewBlockRef("2b", 2), previousID: "1a", libBlockRef: bstream.NewBlockRef("0a", 0)},
			{blockRef: bstream.NewBlockRef("4a", 4), previousID: "3a", libBlockRef: bstream.NewBlockRef("0a", 0)},
			{blockRef: bstream.NewBlockRef("3b", 3), previousID: "2b", libBlockRef: bstream.NewBlockRef("0a", 0)},
			{blockRef: bstream.NewBlockRef("4b", 4), previousID: "3b", libBlockRef: bstream.NewBlockRef("0a", 0)},
			{blockRef: bstream.NewBlockRef("5b", 5), previousID: "4b", libBlockRef: bstream.NewBlockRef("0a", 0)},
			{blockRef: bstream.NewBlockRef("5a", 5), previousID: "4a", libBlockRef: bstream.NewBlockRef("0a", 0)},
			{blockRef: bstream.NewBlockRef("6a", 6), previousID: "5a", libBlockRef: bstream.NewBlockRef("4a", 4)},
		},
	}

	newBlockGenerator := func(startBlock uint64, inclusiveStopBlock uint64) TestBlockGenerator {
		return forkDbGenerator
	}
	_, err := runTest(t, nil, 1, 7, []string{"test_store_add_bigint", "assert_test_store_add_bigint"}, 10, 1, newBlockGenerator, func(p *pipeline.Pipeline, b *bstream.Block, stores store.Map, baseStore dstore.Store) {
		if b.Number == 6 {
			s, found := stores.Get("test_store_add_bigint")
			require.True(t, found)
			bytes, found := s.GetLast("a.key.pos")
			require.True(t, found)
			bi := &big.Int{}
			_, success := bi.SetString(string(bytes), 10)
			require.True(t, success)
			require.Equal(t, "6", bi.String())

			bytes, found = s.GetLast("a.key.neg")
			require.True(t, found)
			_, success = bi.SetString(string(bytes), 10)
			require.True(t, success)
			require.Equal(t, "-6", bi.String())
		}
	})
	require.NoError(t, err)
}

func Test_test_store_proto(t *testing.T) {
	newBlockGenerator := func(startBlock uint64, inclusiveStopBlock uint64) TestBlockGenerator {
		return &LinearBlockGenerator{
			startBlock:         startBlock,
			inclusiveStopBlock: inclusiveStopBlock,
		}
	}
	moduleOutputs, err := runTest(t, nil, 10, 12, []string{"test_store_proto"}, 10, 1, newBlockGenerator, nil)
	require.NoError(t, err)

	require.Equal(t, []string{
		`{"name":"test_store_proto","deltas":[{"op":"CREATE","old":{},"new":{"block_number":10,"block_hash":"block-10"}}]}`,
		`{"name":"test_store_proto","deltas":[{"op":"CREATE","old":{},"new":{"block_number":11,"block_hash":"block-11"}}]}`,
	}, moduleOutputs)
}

func Test_MultipleModule(t *testing.T) {
	t.Skipf("todo(colin): fix integration tests")
	newBlockGenerator := func(startBlock uint64, inclusiveStopBlock uint64) TestBlockGenerator {
		return &LinearBlockGenerator{
			startBlock:         startBlock,
			inclusiveStopBlock: inclusiveStopBlock,
		}
	}
	moduleOutputs, err := runTest(t, nil, 10, 12, []string{"test_map", "test_store_add_int64", "test_store_proto"}, 10, 1, newBlockGenerator, nil)
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
	t.Skipf("todo(colin): fix integration tests")
	newBlockGenerator := func(startBlock uint64, inclusiveStopBlock uint64) TestBlockGenerator {
		return &LinearBlockGenerator{
			startBlock:         startBlock,
			inclusiveStopBlock: inclusiveStopBlock,
		}
	}

	//todo: Need to validate the storage file

	_, err := runTest(t, nil, 1000, 1021, []string{"test_store_add_bigint", "assert_test_store_add_bigint"}, 10, 1, newBlockGenerator, nil)
	require.NoError(t, err)
}

func Test_MultipleModule_Batch_2(t *testing.T) {
	newBlockGenerator := func(startBlock uint64, inclusiveStopBlock uint64) TestBlockGenerator {
		return &LinearBlockGenerator{
			startBlock:         startBlock,
			inclusiveStopBlock: inclusiveStopBlock,
		}
	}

	moduleOutputs, err := runTest(t, nil, 110, 112, []string{"test_map", "test_store_proto"}, 10, 1, newBlockGenerator, nil)
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
	moduleOutputs, err := runTest(t, nil, 110, 112, []string{"test_map", "test_store_proto"}, 10, 1,
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

//	func Test_test_store_add_bigint(t *testing.T) {
//		newBlockGenerator := func(startBlock uint64, inclusiveStopBlock uint64) TestBlockGenerator {
//			return &LinearBlockGenerator{
//				startBlock:         startBlock,
//				inclusiveStopBlock: inclusiveStopBlock,
//			}
//		}
//
//		_, err := runTest(t, 1, 1001, []string{"test_store_add_bigint", "assert_test_store_add_bigint"}, newBlockGenerator, nil)
//		require.NoError(t, err)
//
// }
func Test_test_store_delete_prefix(t *testing.T) {
	newBlockGenerator := func(startBlock uint64, inclusiveStopBlock uint64) TestBlockGenerator {
		return &LinearBlockGenerator{
			startBlock:         startBlock,
			inclusiveStopBlock: inclusiveStopBlock,
		}
	}
	_, err := runTest(
		t,
		nil,
		30,
		41,
		[]string{"test_store_delete_prefix", "assert_test_store_delete_prefix"},
		10,
		1, newBlockGenerator, func(p *pipeline.Pipeline, b *bstream.Block, stores store.Map, baseStore dstore.Store) {
			if b.Number == 40 {
				s, storeFound := stores.Get("test_store_delete_prefix")
				require.True(t, storeFound)
				require.Equal(t, uint64(1), s.Length())
			}
		},
	)
	require.NoError(t, err)
}

// -------------------- StoreAddI64 -------------------- //
func Test_test_store_add_i64(t *testing.T) {
	newBlockGenerator := func(startBlock uint64, inclusiveStopBlock uint64) TestBlockGenerator {
		return &LinearBlockGenerator{
			startBlock:         startBlock,
			inclusiveStopBlock: inclusiveStopBlock,
		}
	}
	_, err := runTest(t, nil, 1, 2, []string{"setup_test_store_add_i64", "assert_test_store_add_i64"}, 10, 1, newBlockGenerator, nil)
	require.NoError(t, err)
}

func Test_test_store_add_i64_deltas(t *testing.T) {
	newBlockGenerator := func(startBlock uint64, inclusiveStopBlock uint64) TestBlockGenerator {
		return &LinearBlockGenerator{
			startBlock:         startBlock,
			inclusiveStopBlock: inclusiveStopBlock,
		}
	}
	_, err := runTest(t, nil, 1, 2, []string{"setup_test_store_add_i64", "assert_test_store_add_i64_deltas"}, 10, 1, newBlockGenerator, nil)
	require.NoError(t, err)
}

// -------------------- StoreSetI64/StoreGetI64 -------------------- //
func Test_test_store_set_i64(t *testing.T) {
	newBlockGenerator := func(startBlock uint64, inclusiveStopBlock uint64) TestBlockGenerator {
		return &LinearBlockGenerator{
			startBlock:         startBlock,
			inclusiveStopBlock: inclusiveStopBlock,
		}
	}
	_, err := runTest(t, nil, 20, 31, []string{"setup_test_store_set_i64", "assert_test_store_set_i64"}, 10, 1, newBlockGenerator, nil)
	require.NoError(t, err)
}

func Test_test_store_root_depend(t *testing.T) {
	newBlockGenerator := func(startBlock uint64, inclusiveStopBlock uint64) TestBlockGenerator {
		return &LinearBlockGenerator{
			startBlock:         startBlock,
			inclusiveStopBlock: inclusiveStopBlock,
		}
	}

	_, err := runTest(t, nil, 10, 11, []string{"store_depends_on_depend"}, 10, 10, newBlockGenerator, nil)
	require.NoError(t, err)
}

func Test_test_store_string_get(t *testing.T) {
	t.Skipf("todo(colin): fix integration tests")
	newBlockGenerator := func(startBlock uint64, inclusiveStopBlock uint64) TestBlockGenerator {
		return &LinearBlockGenerator{
			startBlock:         startBlock,
			inclusiveStopBlock: inclusiveStopBlock,
		}
	}

	_, err := runTest(t, nil, 10, 11, []string{"setup_test_store_get_set_string", "assert_test_store_get_set_string"}, 10, 1, newBlockGenerator, nil)
	require.NoError(t, err)
}

func Test_test_store_get_array_string(t *testing.T) {
	t.Skipf("todo(colin): fix integration tests")
	newBlockGenerator := func(startBlock uint64, inclusiveStopBlock uint64) TestBlockGenerator {
		return &LinearBlockGenerator{
			startBlock:         startBlock,
			inclusiveStopBlock: inclusiveStopBlock,
		}
	}

	_, err := runTest(t, nil, 1, 2, []string{"setup_test_store_get_array_string", "assert_test_store_get_array_string"}, 10, 1, newBlockGenerator, nil)
	require.NoError(t, err)
}

//func Test_test_store_get_array_proto(t *testing.T) {
//	newBlockGenerator := func(startBlock uint64, inclusiveStopBlock uint64) TestBlockGenerator {
//		return &LinearBlockGenerator{
//			startBlock:         startBlock,
//			inclusiveStopBlock: inclusiveStopBlock,
//		}
//	}
//
//	_, err := runTest(t, nil, 1, 5, []string{"setup_test_store_get_array_proto", "assert_test_store_get_array_proto"}, 10, 1, newBlockGenerator, nil)
//	require.NoError(t, err)
//}

func Test_assert_all_test(t *testing.T) {
	newBlockGenerator := func(startBlock uint64, inclusiveStopBlock uint64) TestBlockGenerator {
		return &LinearBlockGenerator{
			startBlock:         startBlock,
			inclusiveStopBlock: inclusiveStopBlock,
		}
	}
	_, err := runTest(t, nil, 20, 31, []string{"assert_all_test"}, 10, 1, newBlockGenerator, nil)
	require.NoError(t, err)
}

func Test_assert_all_string(t *testing.T) {
	newBlockGenerator := func(startBlock uint64, inclusiveStopBlock uint64) TestBlockGenerator {
		return &LinearBlockGenerator{
			startBlock:         startBlock,
			inclusiveStopBlock: inclusiveStopBlock,
		}
	}
	_, err := runTest(t, nil, 20, 31, []string{"assert_all_test_string"}, 10, 1, newBlockGenerator, nil)
	require.NoError(t, err)
}

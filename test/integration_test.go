package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/streamingfast/bstream"
	"github.com/streamingfast/dstore"
	tracing "github.com/streamingfast/sf-tracing"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	pbsubstreamstest "github.com/streamingfast/substreams/pb/sf/substreams/v1/test"
	"github.com/streamingfast/substreams/pipeline"
	"github.com/streamingfast/substreams/reqctx"
	"github.com/streamingfast/substreams/store"
	"github.com/streamingfast/substreams/work"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
)

func runTest(t *testing.T, startBlock int64, exclusiveEndBlock uint64, moduleNames []string, newBlockGenerator NewTestBlockGenerator, blockProcessedCallBack blockProcessedCallBack) (moduleOutputs []string, err error) {
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

	workerFactory := func(_ *zap.Logger) work.JobRunner {
		// TODO(abourget): this isn't used anymore, but we still need a way to mock this E2E test.
		// Unsure about the best way to do this. Will need to look into it.
		w := &TestWorker{
			t:                      t,
			moduleGraph:            moduleGraph,
			responseCollector:      newResponseCollector(),
			newBlockGenerator:      newBlockGenerator,
			blockProcessedCallBack: blockProcessedCallBack,
			testTempDir:            testTempDir,
		}
		return w.Run
	}

	if err = processRequest(t, ctx, request, moduleGraph, workerFactory, newBlockGenerator, responseCollector, false, blockProcessedCallBack, testTempDir); err != nil {
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

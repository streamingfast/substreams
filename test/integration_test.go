package pipeline

import (
	"encoding/json"
	"fmt"
	"github.com/streamingfast/substreams/manifest"
	"github.com/streamingfast/substreams/orchestrator"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"os"
	"testing"
)

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
	ModuleName string                  `json:"name"`
	Result     *pbsubstreams.MapResult `json:"result"`
}

func runTest(t *testing.T, spkgPath string, startBlock int64, exclusiveEndBlock uint64, moduleNames []string) (moduleOutputs []string) {
	//_, _ = logging.ApplicationLogger("test", "test")

	err := os.RemoveAll("/tmp/test.store")
	require.NoError(t, err)

	pkg, moduleGraph := processManifest(t, spkgPath)

	request := &pbsubstreams.Request{
		StartBlockNum: startBlock,
		StopBlockNum:  exclusiveEndBlock,
		Modules:       pkg.Modules,
		OutputModules: moduleNames,
	}

	responseCollector := NewResponseCollector()
	workerPool := orchestrator.NewWorkerPool(1, func() orchestrator.Worker {
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
						r := &pbsubstreams.MapResult{}
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
							o := &pbsubstreams.MapResult{}
							err = proto.Unmarshal(delta.OldValue, o)
							require.NoError(t, err)

							n := &pbsubstreams.MapResult{}
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
	moduleOutputs := runTest(
		t,
		"./testdata/substreams-test-v0.1.0.spkg",
		10,
		12,
		[]string{"map_test"},
	)
	require.Equal(t, []string{
		`{"name":"map_test","result":{"block_number":10,"block_hash":"block-10"}}`,
		`{"name":"map_test","result":{"block_number":11,"block_hash":"block-11"}}`,
	}, moduleOutputs)
}

func Test_store_add_int64(t *testing.T) {
	moduleOutputs := runTest(t, "./testdata/substreams-test-v0.1.0.spkg", 10, 13, []string{"store_add_int64"})
	require.Equal(t, []string{
		`{"name":"store_add_int64","deltas":[{"op":"CREATE","old":"","new":"1"}]}`,
		`{"name":"store_add_int64","deltas":[{"op":"UPDATE","old":"1","new":"2"}]}`,
		`{"name":"store_add_int64","deltas":[{"op":"UPDATE","old":"2","new":"3"}]}`,
	}, moduleOutputs)
}

func Test_store_map_result(t *testing.T) {
	moduleOutputs := runTest(t, "./testdata/substreams-test-v0.1.0.spkg", 10, 12, []string{"store_map_result"})
	require.Equal(t, []string{
		`{"name":"store_map_result","deltas":[{"op":"CREATE","old":{},"new":{"block_number":10,"block_hash":"block-10"}}]}`,
		`{"name":"store_map_result","deltas":[{"op":"CREATE","old":{},"new":{"block_number":11,"block_hash":"block-11"}}]}`,
	}, moduleOutputs)
}

func Test_MultipleModule(t *testing.T) {
	moduleOutputs := runTest(t, "./testdata/substreams-test-v0.1.0.spkg", 10, 12, []string{"map_test", "store_add_int64", "store_map_result"})
	require.Equal(t, []string{
		`{"name":"map_test","result":{"block_number":10,"block_hash":"block-10"}}`,
		`{"name":"store_add_int64","deltas":[{"op":"CREATE","old":"","new":"1"}]}`,
		`{"name":"store_map_result","deltas":[{"op":"CREATE","old":{},"new":{"block_number":10,"block_hash":"block-10"}}]}`,
		`{"name":"map_test","result":{"block_number":11,"block_hash":"block-11"}}`,
		`{"name":"store_add_int64","deltas":[{"op":"UPDATE","old":"1","new":"2"}]}`,
		`{"name":"store_map_result","deltas":[{"op":"CREATE","old":{},"new":{"block_number":11,"block_hash":"block-11"}}]}`,
	}, moduleOutputs)
}

func Test_MultipleModule_Batch(t *testing.T) {
	moduleOutputs := runTest(t, "./testdata/substreams-test-v0.1.0.spkg", 110, 112, []string{"map_test", "store_add_int64", "store_map_result"})
	require.Equal(t, []string{
		`{"name":"map_test","result":{"block_number":110,"block_hash":"block-110"}}`,
		`{"name":"store_add_int64","deltas":[{"op":"UPDATE","old":"90","new":"91"}]}`,
		`{"name":"store_map_result","deltas":[{"op":"CREATE","old":{},"new":{"block_number":110,"block_hash":"block-110"}}]}`,
		`{"name":"map_test","result":{"block_number":111,"block_hash":"block-111"}}`,
		`{"name":"store_add_int64","deltas":[{"op":"UPDATE","old":"91","new":"92"}]}`,
		`{"name":"store_map_result","deltas":[{"op":"CREATE","old":{},"new":{"block_number":111,"block_hash":"block-111"}}]}`,
	}, moduleOutputs)
}

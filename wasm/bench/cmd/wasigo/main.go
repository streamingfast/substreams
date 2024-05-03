package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/streamingfast/substreams/wasm/bench/substreams_wasi_go/pb"

	"go.uber.org/zap"

	"github.com/streamingfast/dstore"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/storage/store"
	"github.com/streamingfast/substreams/wasm"
	_ "github.com/streamingfast/substreams/wasm/wasi"
)

func main() {
	ctx := context.Background()
	wasmRuntime := wasm.NewRegistryWithRuntime("wasi", nil, 0)
	code, err := os.ReadFile("/Users/colindickson/code/dfuse/substreams/wasm/bench/substreams_wasi_go/main.wasm")
	blockReader, err := os.Open("/Users/colindickson/code/dfuse/substreams/wasm/bench/cmd/wasigo/testdata/block.binpb")
	if err != nil {
		panic(err)
	}

	defer blockReader.Close()

	module, err := wasmRuntime.NewModule(ctx, code, "go/wasi")
	if err != nil {
		panic(fmt.Errorf("creating new module: %w", err))
	}
	instance, err := module.NewInstance(ctx)

	start := time.Now()

	for i := 0; i < 1; i++ {
		args := args(
			wasm.NewParamsInput("{key.1: 'value.1'}"),
			blockInputFile("/Users/colindickson/code/dfuse/substreams/wasm/bench/cmd/wasigo/testdata/block.binpb"),
			wasm.NewStoreReaderInput("store.reader.1", createStore(ctx, "store.reader.1")),
			wasm.NewStoreReaderInput("store.reader.2", createStore(ctx, "store.reader.2")),
			wasm.NewStoreWriterOutput("out", createStore(ctx, "out"), 1, "string"),
		)

		call := wasm.NewCall(nil, "mapBlock", "mapBlock", args)
		_, err = module.ExecuteNewCall(ctx, call, instance, args)
		if err != nil {
			panic(fmt.Errorf("executing call: %w", err))
		}

		data := call.Output()
		output := &pb.MapBlockOutput{}
		err = output.UnmarshalVT(data)
		if err != nil {
			panic(fmt.Errorf("unmarshalling output: %w", err))
		}
		jdata, err := json.Marshal(output)
		if err != nil {
			panic(err)
		}
		fmt.Println("output::", string(jdata))

		fmt.Println("-------------------------------- call logs --------------------------------")
		for _, log := range call.Logs {
			fmt.Print(log)
		}
		fmt.Println("----------------------------------------------------------------")

	}
	fmt.Println("total duration", time.Since(start))

}

func createStore(ctx context.Context, name string) *store.FullKV {
	ds, err := dstore.NewStore("file:///tmp/"+name, "kv", "", true)
	if err != nil {
		panic(err)
	}
	storeConfig, err := store.NewConfig(name, 0, "hash.1", pbsubstreams.Module_KindStore_UPDATE_POLICY_UNSET, "string", ds, "trace.id.1")
	if err != nil {
		panic(err)
	}
	fullStore := storeConfig.NewFullKV(zap.NewNop())
	//err = fullStore.Load(ctx, store.NewCompleteFileInfo("map_block", 0, 0))
	//if err != nil {
	//	panic(err)
	//}
	fullStore.Set(0, "key_123", "value_123")
	fullStore.Reset()

	return fullStore
}

func args(ins ...wasm.Argument) []wasm.Argument {
	return ins
}

func blockInputFile(filename string) wasm.Argument {
	content, err := os.ReadFile(filename)
	if err != nil {
		panic(fmt.Errorf("reading input file: %w", err))
	}

	input := wasm.NewSourceInput("sf.ethereum.type.v2.Block")
	input.SetValue(content)

	return input
}

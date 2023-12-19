package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/streamingfast/substreams/wasm"
	_ "github.com/streamingfast/substreams/wasm/wasi"
)

func main() {
	start := time.Now()
	ctx := context.Background()
	wasmRuntime := wasm.NewRegistryWithRuntime("wasi", nil, 0)
	code, err := os.ReadFile("/Users/cbillett/devel/sf/substreams/wasm/bench/substreams_tiny_go/main.wasm")
	blockReader, err := os.Open("/Users/cbillett/devel/sf/substreams/wasm/bench/cmd/barebones/testdata/block.binpb")
	if err != nil {
		panic(err)
	}
	defer blockReader.Close()

	module, err := wasmRuntime.NewModule(ctx, code, "go/wasi")
	if err != nil {
		panic(fmt.Errorf("creating new module: %w", err))
	}
	instance, err := module.NewInstance(ctx)

	args := args(blockInputFile("/Users/cbillett/devel/sf/substreams/wasm/bench/cmd/barebones/testdata/block.binpb"))
	call := wasm.NewCall(nil, "", "", args)
	_, err = module.ExecuteNewCall(ctx, call, instance, args)
	if err != nil {
		panic(fmt.Errorf("executing call: %w", err))
	}
	fmt.Println("call output", string(call.Output()))
	fmt.Println("duration", time.Since(start))

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

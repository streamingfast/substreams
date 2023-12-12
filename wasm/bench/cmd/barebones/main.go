package main

import (
	"context"
	"fmt"
	"github.com/tetratelabs/wazero/api"
	"log"
	"os"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
	"github.com/tetratelabs/wazero/sys"
)

// main writes an input file to stdout, just like `cat`.
//
// This is a basic introduction to the WebAssembly System Interface (WASI).
// See https://github.com/WebAssembly/WASI
func main() {
	// Choose the context to use for function calls.
	ctx := context.Background()

	// Create a new WebAssembly Runtime.
	runtime := wazero.NewRuntime(ctx)
	defer runtime.Close(ctx) // This closes everything this Runtime created.

	reader, err := os.Open("/Users/colindickson/code/dfuse/substreams/wasm/bench/cmd/barebones/testdata/block.binpb")
	if err != nil {
		panic(err)
	}
	defer reader.Close()

	// Combine the above into our baseline config, overriding defaults.
	config := wazero.NewModuleConfig().
		// By default, I/O streams are discarded and there's no file system.
		WithStdout(os.Stdout).WithStderr(os.Stderr).WithStdin(reader)
	config = config.WithStartFunctions("popo")

	// Instantiate WASI, which implements system I/O such as console output.
	wasi_snapshot_preview1.MustInstantiate(ctx, runtime)

	build := runtime.NewHostModuleBuilder("env")
	build.NewFunctionBuilder().
		WithGoModuleFunction(api.GoModuleFunc(func(ctx context.Context, mod api.Module, stack []uint64) {
			fmt.Println("hello world")
		}), nil, nil).
		WithName("bar").
		Export("bar")
	patateModule, err := build.Compile(ctx)
	if err != nil {
		panic(err)
	}
	apiMod, err := runtime.InstantiateModule(ctx, patateModule, config.WithName("env"))
	if err != nil {
		panic(err)
	}
	_ = apiMod

	catWasm := readCode("/Users/colindickson/code/dfuse/substreams/wasm/bench/substreams_ts/index.wasm")

	// InstantiateModule runs the "_start" function, WASI's "main".
	// * Set the program name (arg[0]) to "wasi"; arg[1] should be "/test.txt".
	if _, err := runtime.InstantiateWithConfig(ctx, catWasm, config); err != nil {
		// Note: Most compilers do not exit the module after running "_start",
		// unless there was an error. This allows you to call exported functions.
		if exitErr, ok := err.(*sys.ExitError); ok && exitErr.ExitCode() != 0 {
			fmt.Fprintf(os.Stderr, "exit_code: %d\n", exitErr.ExitCode())
		} else if !ok {
			log.Panicln(err)
		}
	}
}

func readCode(filename string) []byte {
	content, err := os.ReadFile(filename)
	if err != nil {
		panic(err)
	}

	return content
}
